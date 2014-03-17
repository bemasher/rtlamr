// RTLAMR - An rtl-sdr receiver for smart meters operating in the 900MHz ISM band.
// Copyright (C) 2014 Douglas Hall
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bufio"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/bemasher/rtltcp"

	"github.com/bemasher/rtlamr/bch"
	"github.com/bemasher/rtlamr/preamble"
)

const (
	BlockSize = 1 << 12

	SampleRate   = 2.4e6
	DataRate     = 32.768e3
	SymbolLength = SampleRate / DataRate

	PreambleSymbols = 42
	PreambleLength  = PreambleSymbols * SymbolLength

	PacketSymbols = 192
	PacketLength  = PacketSymbols * SymbolLength

	PreambleDFTSize = 8192

	CenterFreq    = 920299072
	RestrictLocal = false

	Preamble     = 0x1F2A60
	PreambleBits = "111110010101001100000"

	GenPoly = 0x16F63

	TimeFormat = "2006-01-02T15:04:05.000"
)

var SymLen = IntRound(SymbolLength)
var config Config

type Config struct {
	serverAddr     string
	logFilename    string
	sampleFilename string
	format         string

	ServerAddr *net.TCPAddr
	CenterFreq uint
	TimeLimit  time.Duration
	MeterID    uint

	Log     *log.Logger
	LogFile *os.File

	GobUnsafe bool
	Encoder   Encoder

	SampleFile *os.File
}

func (c *Config) Parse() (err error) {
	flag.StringVar(&c.serverAddr, "server", "127.0.0.1:1234", "address or hostname of rtl_tcp instance")
	flag.StringVar(&c.logFilename, "logfile", "/dev/stdout", "log statement dump file")
	flag.StringVar(&c.sampleFilename, "samplefile", os.DevNull, "received message signal dump file, offset and message length are displayed to log when enabled")
	flag.UintVar(&c.CenterFreq, "centerfreq", 920299072, "center frequency to receive on")
	flag.DurationVar(&c.TimeLimit, "duration", 0, "time to run for, 0 for infinite")
	flag.UintVar(&c.MeterID, "filterid", 0, "display only messages matching given id")
	flag.StringVar(&c.format, "format", "plain", "format to write log messages in: plain, json, xml or gob")
	flag.BoolVar(&c.GobUnsafe, "gobunsafe", false, "allow gob output to stdout")

	flag.Parse()

	// Parse and resolve rtl_tcp server address.
	c.ServerAddr, err = net.ResolveTCPAddr("tcp", c.serverAddr)
	if err != nil {
		return
	}

	// Open or create the log file.
	if c.logFilename == "/dev/stdout" {
		c.LogFile = os.Stdout
	} else {
		c.LogFile, err = os.Create(c.logFilename)
	}

	// Create a new logger with the log file as output.
	c.Log = log.New(c.LogFile, "", log.Lshortfile)
	if err != nil {
		return
	}

	// Create the sample file.
	c.SampleFile, err = os.Create(c.sampleFilename)
	if err != nil {
		return
	}

	// Create encoder for specified logging format.
	switch strings.ToLower(c.format) {
	case "plain":
		break
	case "json":
		c.Encoder = json.NewEncoder(c.LogFile)
	case "xml":
		c.Encoder = xml.NewEncoder(c.LogFile)
	case "gob":
		c.Encoder = gob.NewEncoder(c.LogFile)

		// Don't let the user output gob to stdout unless they really want to.
		if !c.GobUnsafe && c.logFilename == "/dev/stdout" {
			fmt.Println("Gob encoded messages are not stdout safe, specify logfile or use gobunsafe flag.")
			os.Exit(1)
		}
	default:
		// We didn't get a valid encoder, exit and say so.
		log.Fatal("Invalid log format:", c.format)
	}

	return
}

func (c Config) Close() {
	c.LogFile.Close()
	c.SampleFile.Close()
}

// JSON, XML and GOB all implement this interface so we can simplify log
// output formatting.
type Encoder interface {
	Encode(interface{}) error
}

type Receiver struct {
	rtltcp.SDR
	sdrBuf *bufio.Reader

	pd  preamble.PreambleDetector
	bch bch.BCH
}

func NewReceiver(blockSize int) (rcvr Receiver) {
	// Plan the preamble detector.
	rcvr.pd = preamble.NewPreambleDetector(PreambleDFTSize, SymbolLength, PreambleBits)

	// Create a new BCH for error detection.
	rcvr.bch = bch.NewBCH(GenPoly)
	config.Log.Printf("BCH: %+v\n", rcvr.bch)

	// Connect to rtl_tcp server.
	if err := rcvr.Connect(config.ServerAddr); err != nil {
		config.Log.Fatal(err)
	}

	// Buffer the connection to rtl_tcp. This simplifies reading blocks and
	// decoding data but will likely go away in a future release.
	rcvr.sdrBuf = bufio.NewReaderSize(rcvr.SDR, IntRound(PacketLength+BlockSize)<<1)

	// Tell the user how many gain settings were reported by rtl_tcp.
	config.Log.Println("GainCount:", rcvr.SDR.Info.GainCount)

	// Set some parameters for listening.
	rcvr.SetSampleRate(SampleRate)
	rcvr.SetCenterFreq(uint32(config.CenterFreq))
	rcvr.SetGainMode(true)

	return
}

// Clean up rtl_tcp connection and destroy preamble detection plan.
func (rcvr *Receiver) Close() {
	rcvr.SDR.Close()
	rcvr.pd.Close()
}

func (rcvr *Receiver) Run() {
	// Setup signal channel for interruption.
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint)

	// Allocate sample and demodulated signal buffers.
	block := make([]byte, BlockSize<<1)
	amBuf := make([]float64, IntRound(PacketLength+BlockSize))

	// Setup time limit channel
	tLimit := make(<-chan time.Time, 1)
	if config.TimeLimit != 0 {
		tLimit = time.After(config.TimeLimit)
	}

	start := time.Now()
	for {
		// Exit on interrupt or time limit, otherwise receive.
		select {
		case <-sigint:
			return
		case <-tLimit:
			fmt.Println("Time Limit Reached:", time.Since(start))
			return
		default:
			// Read new sample block.
			_, err := rcvr.sdrBuf.Read(block)
			if err != nil {
				config.Log.Fatal("Error reading samples:", err)
			}

			// Peek at a packet's worth of data plus the blocksize.
			raw, err := rcvr.sdrBuf.Peek(IntRound((PacketLength + BlockSize)) << 1)
			if err != nil {
				log.Fatal("Error peeking at buffer:", err)
			}

			// AM Demodulate
			for i := 0; i < BlockSize<<1; i++ {
				amBuf[i] = Mag(raw[i<<1], raw[(i<<1)+1])
			}

			// Detect preamble in first half of demod buffer.
			rcvr.pd.Execute(amBuf)
			align := rcvr.pd.ArgMax()

			// Bad framing, catch message on next block.
			if align > BlockSize {
				continue
			}

			// Filter signal and bit slice enough data to catch the preamble.
			filtered := MatchedFilter(amBuf[align:], PreambleSymbols>>1)
			bits := BitSlice(filtered)

			// If the preamble matches.
			if bits == PreambleBits {
				for i := BlockSize << 1; i < len(amBuf); i++ {
					amBuf[i] = Mag(raw[i<<1], raw[(i<<1)+1])
				}

				// Filter, slice and parse the rest of the buffered samples.
				filtered := MatchedFilter(amBuf[align:], PacketSymbols>>1)
				bits := BitSlice(filtered)

				// If the checksum fails, bail.
				if rcvr.bch.Encode(bits[16:]) != 0 {
					continue
				}

				// Parse SCM
				scm, err := ParseSCM(bits)
				if err != nil {
					config.Log.Fatal("Error parsing SCM:", err)
				}

				// If filtering by ID and ID doesn't match, bail.
				if config.MeterID != 0 && uint(scm.ID) != config.MeterID {
					continue
				}

				// Get current file offset.
				offset, err := config.SampleFile.Seek(0, os.SEEK_CUR)
				if err != nil {
					config.Log.Fatal("Error getting sample file offset:", err)
				}

				// Dump message to file.
				_, err = config.SampleFile.Write(raw)
				if err != nil {
					config.Log.Fatal("Error dumping samples:", err)
				}

				msg := Message{time.Now(), offset, len(raw), scm}

				// Write or encode message to log file.
				if config.Encoder == nil {
					// A nil encoder is just plain-text output.
					fmt.Fprintf(config.LogFile, "%+v", msg)
				} else {
					err = config.Encoder.Encode(msg)
					if err != nil {
						log.Fatal("Error encoding message:", err)
					}

					// The XML encoder doesn't write new lines after each
					// element, add them.
					if strings.ToLower(config.format) == "xml" {
						fmt.Fprintln(config.LogFile)
					}
				}
			}
		}
	}
}

// Shift sample from unsigned and normalize.
func Mag(i, q byte) float64 {
	j := (127.5 - float64(i)) / 127
	k := (127.5 - float64(q)) / 127
	return math.Hypot(j, k)
}

// Matched filter implemented as integrate and dump. Output array is equal to
// the number of manchester coded symbols per packet.
func MatchedFilter(input []float64, bits int) (output []float64) {
	output = make([]float64, bits)

	fIdx := 0
	for idx := 0.0; fIdx < bits; idx += SymbolLength * 2 {
		lower := IntRound(idx)
		upper := IntRound(idx + SymbolLength)

		for i := 0; i < upper-lower; i++ {
			output[fIdx] += input[lower+i] - input[upper+i]
		}
		fIdx++
	}
	return
}

func BitSlice(input []float64) (output string) {
	for _, v := range input {
		if v > 0.0 {
			output += "1"
		} else {
			output += "0"
		}
	}
	return
}

func ParseUint(raw string) uint64 {
	tmp, _ := strconv.ParseUint(raw, 2, 64)
	return tmp
}

// Message for logging.
type Message struct {
	Time   time.Time
	Offset int64
	Length int
	SCM    SCM
}

func (msg Message) String() string {
	// If we aren't dumping samples, omit offset and packet length.
	if config.sampleFilename == os.DevNull {
		return fmt.Sprintf("{Time:%s SCM:%+v}\n",
			msg.Time.Format(TimeFormat), msg.SCM,
		)
	}

	return fmt.Sprintf("{Time:%s Offset:%d Length:%d SCM:%+v}\n",
		msg.Time.Format(TimeFormat), msg.Offset, msg.Length, msg.SCM,
	)
}

// Standard Consumption Message
type SCM struct {
	ID          uint32
	Type        uint8
	Tamper      Tamper
	Consumption uint32
	Checksum    uint16
}

func (scm SCM) String() string {
	return fmt.Sprintf("{ID:%8d Type:%2d Tamper:%+v Consumption:%8d Checksum:0x%04X}",
		scm.ID, scm.Type, scm.Tamper, scm.Consumption, scm.Checksum,
	)
}

type Tamper struct {
	Phy uint8
	Enc uint8
}

func (t Tamper) String() string {
	return fmt.Sprintf("{Phy:%d Enc:%d}", t.Phy, t.Enc)
}

// Given a string of bits, parse the message.
func ParseSCM(data string) (scm SCM, err error) {
	if len(data) != 96 {
		return scm, errors.New("invalid input length")
	}

	scm.ID = uint32(ParseUint(data[21:23] + data[56:80]))
	scm.Type = uint8(ParseUint(data[26:30]))
	scm.Tamper.Phy = uint8(ParseUint(data[24:26]))
	scm.Tamper.Enc = uint8(ParseUint(data[30:32]))
	scm.Consumption = uint32(ParseUint(data[32:56]))
	scm.Checksum = uint16(ParseUint(data[80:96]))

	return scm, nil
}

func IntRound(i float64) int {
	return int(math.Floor(i + 0.5))
}

func init() {
	err := config.Parse()
	if err != nil {
		config.Log.Fatal("Error parsing flags:", err)
	}
}

func main() {
	log.Println("Server:", config.ServerAddr)
	log.Println("BlockSize:", BlockSize)
	log.Println("SampleRate:", SampleRate)
	log.Println("DataRate:", DataRate)
	log.Println("SymbolLength:", SymbolLength)
	log.Println("PacketSymbols:", PacketSymbols)
	log.Println("PacketLength:", PacketLength)
	log.Println("CenterFreq:", config.CenterFreq)
	log.Println("TimeLimit:", config.TimeLimit)

	log.Println("Format:", config.format)
	log.Println("LogFile:", config.logFilename)
	log.Println("SampleFile:", config.sampleFilename)

	if config.MeterID != 0 {
		log.Println("FilterID:", config.MeterID)
	}

	rcvr := NewReceiver(BlockSize)
	defer rcvr.Close()
	defer config.Close()

	config.Log.Println("Running...")
	rcvr.Run()
}

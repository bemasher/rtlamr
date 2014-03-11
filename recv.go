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
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/bemasher/rtltcp"

	"github.com/bemasher/rtlamr/bch"
	"github.com/bemasher/rtlamr/preamble"
)

const (
	BlockSize = 1 << 14

	SampleRate   = 2.4e6
	DataRate     = 32.768e3
	SymbolLength = SampleRate / DataRate

	PacketSymbols = 192
	PacketLength  = PacketSymbols * SymbolLength

	PreambleDFTSize = 20480

	CenterFreq    = 920299072
	RestrictLocal = false

	Preamble     = 0x1F2A60
	PreambleBits = "111110010101001100000"

	GenPoly    = 0x16F63
	MsgLen     = 10
	ErrorCount = 1

	TimeFormat = "2006-01-02T15:04:05.000"
)

var SymLen = IntRound(SymbolLength)
var config Config

type Config struct {
	serverAddr     string
	logFilename    string
	sampleFilename string

	ServerAddr *net.TCPAddr
	CenterFreq uint
	TimeLimit  time.Duration
	LogFile    *os.File
	SampleFile *os.File
}

func (c Config) String() string {
	return fmt.Sprintf("{ServerAddr:%s Freq:%d TimeLimit:%s LogFile:%s SampleFile:%s}",
		c.ServerAddr,
		c.CenterFreq,
		c.TimeLimit,
		c.LogFile.Name(),
		c.SampleFile.Name(),
	)
}

func (c *Config) Parse() (err error) {
	flag.StringVar(&c.serverAddr, "server", "127.0.0.1:1234", "address or hostname of rtl_tcp instance")
	flag.StringVar(&c.logFilename, "logfile", "/dev/stdout", "log statement dump file")
	flag.StringVar(&c.sampleFilename, "samplefile", os.DevNull, "received message signal dump file, offset and message length are displayed to log when enabled")
	flag.UintVar(&c.CenterFreq, "centerfreq", 920299072, "center frequency to receive on")
	flag.DurationVar(&c.TimeLimit, "duration", 0, "time to run for, 0 for infinite")

	flag.Parse()

	c.ServerAddr, err = net.ResolveTCPAddr("tcp", c.serverAddr)
	if err != nil {
		return
	}

	if c.logFilename == "/dev/stdout" {
		c.LogFile = os.Stdout
	} else {
		c.LogFile, err = os.Create(c.logFilename)
	}
	if err != nil {
		return
	}

	log.SetOutput(c.LogFile)
	log.SetFlags(log.Lshortfile)

	c.SampleFile, err = os.Create(c.sampleFilename)
	if err != nil {
		return
	}

	return
}

func (c Config) Close() {
	c.LogFile.Close()
	c.SampleFile.Close()
}

type Receiver struct {
	rtltcp.SDR

	pd  preamble.PreambleDetector
	bch bch.BCH
}

func NewReceiver(blockSize int) (rcvr Receiver) {
	rcvr.pd = preamble.NewPreambleDetector(PreambleDFTSize, SymbolLength, PreambleBits)

	rcvr.bch = bch.NewBCH(GenPoly, MsgLen, ErrorCount)
	log.Printf("BCH: %+v\n", rcvr.bch)

	if err := rcvr.Connect(config.ServerAddr); err != nil {
		log.Fatal(err)
	}

	log.Println("GainCount:", rcvr.SDR.Info.GainCount)

	rcvr.SetSampleRate(SampleRate)
	rcvr.SetCenterFreq(uint32(config.CenterFreq))
	rcvr.SetOffsetTuning(true)
	rcvr.SetGainMode(true)

	return
}

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
	raw := make([]byte, BlockSize<<2)
	amBuf := make([]float64, BlockSize<<1)

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
			// Rotate sample and raw buffer.
			copy(raw[:BlockSize<<1], raw[BlockSize<<1:])
			copy(amBuf[:BlockSize], amBuf[BlockSize:])

			// Read new sample block.
			_, err := io.ReadFull(rcvr, block)
			if err != nil {
				log.Fatal("Error reading samples:", err)
			}

			// Store the block to dump the message if necessary
			copy(raw[BlockSize<<1:], block)

			// AM Demodulate
			for i := 0; i < BlockSize; i++ {
				amBuf[BlockSize+i] = Mag(block[i<<1], block[(i<<1)+1])
			}

			// Detect preamble in first half of demod buffer.
			rcvr.pd.Execute(amBuf)
			align := rcvr.pd.ArgMax()

			// Bad framing, catch message on next block.
			if align > BlockSize {
				continue
			}

			// Filter signal and bit slice.
			filtered := MatchedFilter(amBuf[align:])
			bits := ""
			for i := range filtered {
				if filtered[i] > 0 {
					bits += "1"
				} else {
					bits += "0"
				}
			}

			// Convert bitstring to bytes for BCH.
			data := make([]byte, 10)
			for i := range data {
				idx := i<<3 + 16
				b, err := strconv.ParseUint(bits[idx:idx+8], 2, 8)
				if err != nil {
					log.Fatal("Error parsing byte:", err)
				}
				data[i] = byte(b)
			}

			// Calculate the syndrome to track which bits were corrected later
			// for logging.
			syn := rcvr.bch.Encode(data)

			// Correct errors
			checksum, corrected := rcvr.bch.Correct(data)

			// If the preamble matches and the corrected checksum is 0 we
			// probably have a message.
			if bits[:21] == PreambleBits && checksum == 0 {
				// Convert back to bitstring for parsing (should probably
				// write a method for parsing from bytes)
				bits = bits[:16]
				for i := range data {
					bits += fmt.Sprintf("%08b", data[i])
				}

				// Parse SCM
				scm, err := ParseSCM(bits)
				if err != nil {
					log.Fatal("Error parsing SCM:", err)
				}

				// Calculate message bounds.
				lower := (align - IntRound(8*SymbolLength)) << 1
				if lower < 0 {
					lower = 0
				}
				upper := (align + IntRound(PacketLength+8*SymbolLength)) << 1

				// Dump message to file.
				_, err = config.SampleFile.Write(raw[lower:upper])
				if err != nil {
					log.Fatal("Error dumping samples:", err)
				}

				fmt.Fprintf(config.LogFile, "%s %+v ", time.Now().Format(TimeFormat), scm)

				if config.sampleFilename != os.DevNull {
					offset, err := config.SampleFile.Seek(0, os.SEEK_CUR)
					if err != nil {
						log.Fatal("Error getting sample file offset:", err)
					}

					fmt.Printf("%d %d ", offset, upper-lower)
				}

				// If we corrected any errors, print their positions.
				if corrected {
					fmt.Fprintf(config.LogFile, "%d\n", rcvr.bch.Syndromes[syn])
				} else {
					fmt.Fprintln(config.LogFile)
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
func MatchedFilter(input []float64) (output []float64) {
	output = make([]float64, IntRound(PacketSymbols/2))
	fidx := 0
	for idx := 0.0; fidx < 96; idx += SymbolLength * 2 {
		lower := IntRound(idx)
		upper := IntRound(idx + SymbolLength)
		for i := 0; i < upper-lower; i++ {
			output[fidx] += input[lower+i] - input[upper+i]
		}
		fidx++
	}
	return
}

func ParseUint(raw string) uint64 {
	tmp, _ := strconv.ParseUint(raw, 2, 64)
	return tmp
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
		log.Fatal("Error parsing flags:", err)
	}
}

func main() {
	log.Println("Config:", config)
	log.Println("BlockSize:", BlockSize)
	log.Println("SampleRate:", SampleRate)
	log.Println("DataRate:", DataRate)
	log.Println("SymbolLength:", SymbolLength)
	log.Println("PacketSymbols:", PacketSymbols)
	log.Println("PacketLength:", PacketLength)
	log.Println("CenterFreq:", CenterFreq)

	rcvr := NewReceiver(BlockSize)
	defer rcvr.Close()
	defer config.Close()

	log.Println("Running...")
	rcvr.Run()
}

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
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/bemasher/rtlamr/bch"
	"github.com/bemasher/rtlamr/preamble"
	"github.com/bemasher/rtltcp"
)

const (
	CenterFreq = 920299072
	DataRate   = 32768

	PreambleSymbols = 42
	Preamble        = 0x1F2A60
	PreambleBits    = "111110010101001100000"

	PacketSymbols = 192

	GenPoly = 0x16F63

	RestrictLocal = false
	TimeFormat    = "2006-01-02T15:04:05.000"
)

var (
	config Config
	rcvr   Receiver
)

type Receiver struct {
	rtltcp.SDR

	pd  preamble.PreambleDetector
	bch bch.BCH
	lut MagLUT
}

func (rcvr *Receiver) Init() {
	// Plan the preamble detector.
	rcvr.pd = preamble.NewPreambleDetector(uint(config.BlockSize<<1), config.SymbolLength, PreambleBits)

	// Create a new BCH for error detection.
	rcvr.bch = bch.NewBCH(GenPoly)
	if !config.Quiet {
		config.Log.Printf("BCH: %+v\n", rcvr.bch)
	}

	rcvr.lut = NewMagLUT()

	// Connect to rtl_tcp server.
	if err := rcvr.Connect(nil); err != nil {
		config.Log.Fatal(err)
	}

	// Tell the user how many gain settings were reported by rtl_tcp.
	if !config.Quiet {
		config.Log.Println("GainCount:", rcvr.SDR.Info.GainCount)
	}

	// Set some parameters for listening.
	rcvr.SetSampleRate(uint32(config.SampleRate))
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
	signal.Notify(sigint, os.Kill, os.Interrupt)

	// Allocate sample and demodulated signal buffers.
	raw := make([]byte, (config.PacketLength+config.BlockSize)<<1)
	amBuf := make([]float64, config.PacketLength+config.BlockSize)

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
			copy(raw, raw[config.BlockSize<<1:])
			copy(amBuf, amBuf[config.BlockSize:])

			// Read new sample block.
			_, err := rcvr.Read(raw[config.PacketLength<<1:])
			if err != nil {
				config.Log.Fatal("Error reading samples: ", err)
			}

			// AM Demodulate
			block := amBuf[config.PacketLength:]
			rawBlock := raw[config.PacketLength<<1:]
			for idx := range block {
				block[idx] = math.Sqrt(rcvr.lut[rawBlock[idx<<1]] + rcvr.lut[rawBlock[(idx<<1)+1]])
			}

			// Detect preamble in first half of demod buffer.
			rcvr.pd.Execute(amBuf)
			align := rcvr.pd.ArgMax()

			// Bad framing, catch message on next block.
			if uint(align) > config.BlockSize {
				continue
			}

			// Filter signal and bit slice enough data to catch the preamble.
			filtered := MatchedFilter(amBuf[align:], PreambleSymbols>>1)
			bits := BitSlice(filtered)

			// If the preamble matches.
			if bits == PreambleBits {
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
					config.Log.Fatal("Error parsing SCM: ", err)
				}

				// If filtering by ID and ID doesn't match, bail.
				if config.MeterID != 0 && uint(scm.ID) != config.MeterID {
					continue
				}

				// If filtering by type and type doesn't match, bail.
				if config.MeterType != 0 && uint(scm.Type) != config.MeterType {
					continue
				}

				// Get current file offset.
				offset, err := config.SampleFile.Seek(0, os.SEEK_CUR)
				if err != nil {
					config.Log.Fatal("Error getting sample file offset: ", err)
				}

				// Dump message to file.
				_, err = config.SampleFile.Write(raw)
				if err != nil {
					config.Log.Fatal("Error dumping samples: ", err)
				}

				msg := Message{time.Now(), offset, len(raw), scm}

				// Write or encode message to log file.
				if config.Encoder == nil {
					// A nil encoder is just plain-text output.
					fmt.Fprintf(config.LogFile, "%+v", msg)
				} else {
					err = config.Encoder.Encode(msg)
					if err != nil {
						log.Fatal("Error encoding message: ", err)
					}

					// The XML encoder doesn't write new lines after each
					// element, add them.
					if strings.ToLower(config.format) == "xml" {
						fmt.Fprintln(config.LogFile)
					}
				}

				if config.Single {
					return
				}
			}
		}
	}
}

// A lookup table for calculating magnitude of an interleaved unsigned byte
// stream.
type MagLUT []float64

// Shifts sample by 127.4 (most common DC offset value of rtl-sdr dongles) and
// stores square.
func NewMagLUT() (lut MagLUT) {
	lut = make([]float64, 0x100)
	for idx := range lut {
		lut[idx] = 127.4 - float64(idx)
		lut[idx] *= lut[idx]
	}
	return
}

// Matched filter implemented as integrate and dump. Output array is equal to
// the number of manchester coded symbols per packet.
func MatchedFilter(input []float64, bits int) (output []float64) {
	output = make([]float64, bits)

	fIdx := 0
	for idx := 0; fIdx < bits; idx += config.SymbolLength * 2 {
		offset := idx + config.SymbolLength

		for i := 0; i < config.SymbolLength; i++ {
			output[fIdx] += input[idx+i] - input[offset+i]
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

func (msg Message) Record() (record []string) {
	record = append(record, msg.Time.Format(time.RFC3339Nano))
	record = append(record, strconv.FormatInt(int64(msg.Offset), 10))
	record = append(record, strconv.FormatInt(int64(msg.Length), 10))
	record = append(record, msg.SCM.Record()...)
	return record
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

func (scm SCM) Record() (record []string) {
	record = append(record, strconv.FormatInt(int64(scm.ID), 10))
	record = append(record, strconv.FormatInt(int64(scm.Type), 10))
	record = append(record, scm.Tamper.Record()...)
	record = append(record, strconv.FormatInt(int64(scm.Consumption), 10))
	record = append(record, fmt.Sprintf("0x%04X", scm.Checksum))
	return
}

type Tamper struct {
	Phy uint8
	Enc uint8
}

func (t Tamper) String() string {
	return fmt.Sprintf("{Phy:%d Enc:%d}", t.Phy, t.Enc)
}

func (tamper Tamper) Record() (record []string) {
	record = append(record, strconv.FormatInt(int64(tamper.Phy), 10))
	record = append(record, strconv.FormatInt(int64(tamper.Enc), 10))
	return
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

func init() {
	// Register rtltcp specific flags.
	rcvr.RegisterFlags()

	// Parse configuration
	err := config.Parse()
	if err != nil {
		log.Fatal("Error parsing flags: ", err)
	}

	// Connect receiver and set some defaults.
	rcvr.Init()

	// Handle rtltcp specific flags.
	rcvr.HandleFlags()
}

func main() {
	if !config.Quiet {
		config.Log.Println("Server:", config.ServerAddr)
		config.Log.Println("BlockSize:", config.BlockSize)
		config.Log.Println("SampleRate:", config.SampleRate)
		config.Log.Println("DataRate:", DataRate)
		config.Log.Println("SymbolLength:", config.SymbolLength)
		config.Log.Println("PreambleSymbols:", PreambleSymbols)
		config.Log.Println("PreambleLength:", config.PreambleLength)
		config.Log.Println("PacketSymbols:", PacketSymbols)
		config.Log.Println("PacketLength:", config.PacketLength)
		config.Log.Println("CenterFreq:", rcvr.Flags.CenterFreq)
		config.Log.Println("TimeLimit:", config.TimeLimit)

		config.Log.Println("Format:", config.format)
		config.Log.Println("LogFile:", config.logFilename)
		config.Log.Println("SampleFile:", config.sampleFilename)

		if config.MeterID != 0 {
			config.Log.Println("FilterID:", config.MeterID)
		}
	}

	defer rcvr.Close()
	defer config.Close()

	if !config.Quiet {
		config.Log.Println("Running...")
	}

	rcvr.Run()
}

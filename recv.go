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
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/bemasher/rtlamr/csv"
	"github.com/bemasher/rtltcp"
)

const (
	CenterFreq = 920299072
	TimeFormat = "2006-01-02T15:04:05.000"
)

var rcvr Receiver

type Receiver struct {
	rtltcp.SDR
	d Decoder
	p Parser
}

func (rcvr *Receiver) NewReceiver() {
	switch strings.ToLower(*msgType) {
	case "scm":
		rcvr.d = NewDecoder(NewSCMPacketConfig(*symbolLength))
		rcvr.p = NewSCMParser()
	case "idm":
		rcvr.d = NewDecoder(NewIDMPacketConfig(*symbolLength))
		rcvr.p = NewIDMParser()
	default:
		log.Fatalf("Invalid message type: %q\n", *msgType)
	}

	if !*quiet {
		rcvr.d.cfg.Log()
		log.Println("CRC:", rcvr.p)
	}

	// Connect to rtl_tcp server.
	if err := rcvr.Connect(nil); err != nil {
		log.Fatal(err)
	}

	rcvr.HandleFlags()

	// Tell the user how many gain settings were reported by rtl_tcp.
	if !*quiet {
		log.Println("GainCount:", rcvr.SDR.Info.GainCount)
	}

	// Set some parameters for listening.
	rcvr.SetSampleRate(uint32(rcvr.d.cfg.SampleRate))
	rcvr.SetGainMode(true)

	return
}

func (rcvr *Receiver) Run() {
	// Setup signal channel for interruption.
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Kill, os.Interrupt)

	// Setup time limit channel
	tLimit := make(<-chan time.Time, 1)
	if *timeLimit != 0 {
		tLimit = time.After(*timeLimit)
	}

	block := make([]byte, rcvr.d.cfg.BlockSize2)

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
			_, err := rcvr.Read(block)
			if err != nil {
				log.Fatal("Error reading samples: ", err)
			}

			seen := make(map[string]bool)
			for _, pkt := range rcvr.d.Decode(block) {
				data := NewDataFromBytes(pkt)

				if !seen[data.Bits] {
					seen[data.Bits] = true
				} else {
					continue
				}

				scm, err := rcvr.p.Parse(NewDataFromBytes(pkt))
				if err != nil {
					// log.Println(err)
					continue
				}

				if *meterID != 0 && uint32(*meterID) != scm.MeterID() {
					continue
				}

				if *meterType != 0 && uint8(*meterType) != scm.MeterType() {
					continue
				}

				msg := NewLogMessage(scm)

				if encoder == nil {
					// A nil encoder is just plain-text output.
					fmt.Fprintln(logFile, msg)
				} else {
					err = encoder.Encode(msg)
					if err != nil {
						log.Fatal("Error encoding message: ", err)
					}

					// The XML encoder doesn't write new lines after each
					// element, add them.
					if _, ok := encoder.(*xml.Encoder); ok {
						fmt.Fprintln(logFile)
					}
				}

				if *single {
					return
				}
			}

			if *sampleFilename != os.DevNull {
				_, err = sampleFile.Write(rcvr.d.iq)
				if err != nil {
					log.Fatal("Error writing raw samples to file:", err)
				}
			}
		}
	}
}

type Data struct {
	Bits  string
	Bytes []byte
}

func NewDataFromBytes(data []byte) (d Data) {
	d.Bytes = data
	for _, b := range data {
		d.Bits += fmt.Sprintf("%08b", b)
	}
	return
}

func NewDataFromBits(data string) (d Data) {
	d.Bits = data
	for idx := 0; idx < len(data); idx += 8 {
		b, _ := strconv.ParseUint(d.Bits[idx:idx+8], 2, 8)
		d.Bytes[idx>>3] = uint8(b)
	}
	return
}

type Parser interface {
	Parse(Data) (Message, error)
}

type Message interface {
	MsgType() string
	MeterID() uint32
	MeterType() uint8
	csv.Recorder
}

type LogMessage struct {
	Time   time.Time
	Offset int64
	Length int
	Message
}

func NewLogMessage(msg Message) (logMsg LogMessage) {
	logMsg.Time = time.Now()
	logMsg.Offset, _ = sampleFile.Seek(0, os.SEEK_CUR)
	logMsg.Length = rcvr.d.cfg.BufferLength << 1
	logMsg.Message = msg

	return
}

func (msg LogMessage) String() string {
	if *sampleFilename == os.DevNull {
		return fmt.Sprintf("{Time:%s %s:%s}", msg.Time.Format(TimeFormat), msg.MsgType(), msg.Message)
	}

	return fmt.Sprintf("{Time:%s Offset:%d Length:%d %s:%s}",
		msg.Time.Format(TimeFormat), msg.Offset, msg.Length, msg.MsgType(), msg.Message,
	)
}

func (msg LogMessage) Record() (r []string) {
	r = append(r, msg.Time.Format(time.RFC3339Nano))
	r = append(r, strconv.FormatInt(msg.Offset, 10))
	r = append(r, strconv.FormatInt(int64(msg.Length), 10))
	r = append(r, msg.Message.Record()...)
	return r
}

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
}

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to this file")

func main() {
	rcvr.RegisterFlags()
	RegisterFlags()

	flag.Parse()
	HandleFlags()

	rcvr.NewReceiver()

	defer logFile.Close()
	defer sampleFile.Close()
	defer rcvr.Close()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	rcvr.Run()
}

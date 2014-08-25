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
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	_ "net/http/pprof"

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

func (rcvr *Receiver) NewReceiver(cfg PacketConfig) {
	rcvr.d = NewDecoder(cfg)
	rcvr.p = NewSCMParser()

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
	rcvr.SetCenterFreq(CenterFreq)
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

			pktFound := false
			for _, pkt := range rcvr.d.Decode(block) {
				scm, err := rcvr.p.Parse(NewDataFromBytes(pkt))
				if err != nil {
					continue
				}

				if *meterID != 0 && uint32(*meterID) != scm.ID() {
					continue
				}

				if *meterType != 0 && uint8(*meterType) != scm.Type() {
					continue
				}

				msg := NewLogMessage(scm)
				pktFound = true

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
					if strings.ToLower(*format) == "xml" {
						fmt.Fprintln(logFile)
					}
				}

				if *single {
					return
				}
			}

			if pktFound {
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
	Name() string
	ID() uint32
	Type() uint8
}

type LogMessage struct {
	Time   time.Time
	Offset int64
	Length int
	Body   Message
}

func NewLogMessage(body Message) (msg LogMessage) {
	msg.Time = time.Now()
	msg.Offset, _ = sampleFile.Seek(0, os.SEEK_CUR)
	msg.Length = rcvr.d.cfg.BufferLength << 1
	msg.Body = body

	return
}

func (msg LogMessage) String() string {
	if *sampleFilename == os.DevNull {
		return fmt.Sprintf("{Time:%s %s:%s}", msg.Time.Format(TimeFormat), msg.Body.Name(), msg.Body)
	}

	return fmt.Sprintf("{Time:%s Offset:%d Length:%d %s:%s}",
		msg.Time.Format(TimeFormat), msg.Offset, msg.Length, msg.Body.Name(), msg.Body,
	)
}

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
}

func main() {
	rcvr.RegisterFlags()
	RegisterFlags()
	flag.Parse()
	HandleFlags()

	rcvr.NewReceiver(NewSCMPacketConfig(*symbolLength))

	defer logFile.Close()
	defer sampleFile.Close()
	defer rcvr.Close()

	go http.ListenAndServe("0.0.0.0:6060", nil)

	rcvr.Run()
}

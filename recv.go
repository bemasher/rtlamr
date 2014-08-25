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

	_ "net/http/pprof"

	"github.com/bemasher/rtltcp"
)

const (
	CenterFreq = 920299072
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

	fmt.Println(rcvr.d.cfg)

	rcvr.RegisterFlags()
	flag.Parse()

	// Connect to rtl_tcp server.
	if err := rcvr.Connect(nil); err != nil {
		log.Fatal(err)
	}

	rcvr.HandleFlags()

	// Tell the user how many gain settings were reported by rtl_tcp.
	log.Println("GainCount:", rcvr.SDR.Info.GainCount)

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

	block := make([]byte, rcvr.d.cfg.BlockSize2)

	for {
		// Exit on interrupt or time limit, otherwise receive.
		select {
		case <-sigint:
			return
		default:
			// Read new sample block.
			_, err := rcvr.Read(block)
			if err != nil {
				log.Fatal("Error reading samples: ", err)
			}

			for _, pkt := range rcvr.d.Decode(block) {
				scm, err := rcvr.p.Parse(NewDataFromBytes(pkt))
				if err != nil {
					continue
				}
				fmt.Println(scm)
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
	Parse(Data) (interface{}, error)
}

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
	log.SetOutput(os.Stdout)
}

func main() {
	rcvr.NewReceiver(NewSCMPacketConfig(73))
	defer rcvr.Close()

	go http.ListenAndServe("0.0.0.0:6060", nil)

	rcvr.Run()
}

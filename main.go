// RTLAMR - An rtl-sdr receiver for smart meters operating in the 900MHz ISM band.
// Copyright (C) 2015 Douglas Hall
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
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/bemasher/rtlamr/parse"
	"github.com/bemasher/rtltcp"

	_ "github.com/bemasher/rtlamr/idm"
	_ "github.com/bemasher/rtlamr/r900"
	_ "github.com/bemasher/rtlamr/r900bcd"
	_ "github.com/bemasher/rtlamr/scm"
	_ "github.com/bemasher/rtlamr/scmplus"
)

var rcvr Receiver

type Receiver struct {
	rtltcp.SDR
	p  parse.Parser
	fc parse.FilterChain
}

func (rcvr *Receiver) NewReceiver() {
	var err error
	if rcvr.p, err = parse.NewParser(strings.ToLower(*msgType), *symbolLength, *decimation); err != nil {
		log.Fatal(err)
	}

	// Connect to rtl_tcp server.
	if err := rcvr.Connect(nil); err != nil {
		log.Fatal(err)
	}

	rcvr.HandleFlags()

	cfg := rcvr.p.Cfg()

	gainFlagSet := false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "centerfreq":
			cfg.CenterFreq = uint32(rcvr.Flags.CenterFreq)
		case "samplerate":
			cfg.SampleRate = int(rcvr.Flags.SampleRate)
		case "gainbyindex", "tunergainmode", "tunergain", "agcmode":
			gainFlagSet = true
		case "unique":
			rcvr.fc.Add(NewUniqueFilter())
		case "filterid":
			rcvr.fc.Add(meterID)
		case "filtertype":
			rcvr.fc.Add(meterType)
		}
	})

	rcvr.SetCenterFreq(cfg.CenterFreq)
	rcvr.SetSampleRate(uint32(cfg.SampleRate))

	if !gainFlagSet {
		rcvr.SetGainMode(true)
	}

	rcvr.p.Log()

	// Tell the user how many gain settings were reported by rtl_tcp.
	log.Println("GainCount:", rcvr.SDR.Info.GainCount)

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

	in, out := io.Pipe()

	go func() {
		tcpBlock := make([]byte, 16384)
		for {
			n, err := rcvr.Read(tcpBlock)
			if err != nil {
				return
			}
			out.Write(tcpBlock[:n])
		}
	}()

	sampleBuf := new(bytes.Buffer)
	start := time.Now()

	blockCh := make(chan []byte, 128)
	blockPool := sync.Pool{
		New: func() interface{} {
			return make([]byte, rcvr.p.Cfg().BlockSize2)
		},
	}

	go func() {
		for {
			block := blockPool.Get().([]byte)

			// Read new sample block.
			_, err := io.ReadFull(in, block)
			if err != nil {
				log.Println("Error reading samples: ", err)
				continue
			}
			blockCh <- block
		}
	}()

	for {
		// Exit on interrupt or time limit, otherwise receive.
		select {
		case <-sigint:
			return
		case <-tLimit:
			log.Println("Time Limit Reached:", time.Since(start))
			return
		case block := <-blockCh:
			// If dumping samples, discard the oldest block from the buffer if
			// it's full and write the new block to it.
			if *sampleFilename != os.DevNull {
				if sampleBuf.Len() > rcvr.p.Cfg().BufferLength<<1 {
					io.CopyN(ioutil.Discard, sampleBuf, int64(len(block)))
				}
				sampleBuf.Write(block)
			}

			pktFound := false
			indices := rcvr.p.Dec().Decode(block)

			for _, pkt := range rcvr.p.Parse(indices) {
				if !rcvr.fc.Match(pkt) {
					continue
				}

				var msg parse.LogMessage
				msg.Time = time.Now()
				msg.Offset, _ = sampleFile.Seek(0, os.SEEK_CUR)
				msg.Length = sampleBuf.Len()
				msg.Message = pkt

				err := encoder.Encode(msg)
				if err != nil {
					log.Fatal("Error encoding message: ", err)
				}

				// The XML encoder doesn't write new lines after each element, print them.
				if _, ok := encoder.(*xml.Encoder); ok {
					fmt.Println()
				}

				pktFound = true
				if *single {
					if len(meterID.UintMap) == 0 {
						break
					} else {
						delete(meterID.UintMap, uint(pkt.MeterID()))
					}
				}
			}

			if pktFound {
				if *sampleFilename != os.DevNull {
					_, err := sampleFile.Write(sampleBuf.Bytes())
					if err != nil {
						log.Fatal("Error writing raw samples to file:", err)
					}
				}
				if *single && len(meterID.UintMap) == 0 {
					return
				}
			}
		}
	}
}

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
}

var (
	buildDate  string // date -u '+%Y-%m-%d'
	commitHash string // git rev-parse HEAD
)

func main() {
	rcvr.RegisterFlags()
	RegisterFlags()
	EnvOverride()

	flag.Parse()
	if *version {
		if buildDate == "" || commitHash == "" {
			fmt.Println("Built from source.")
			fmt.Println("Build Date: Unknown")
			fmt.Println("Commit:     Unknown")
		} else {
			fmt.Println("Build Date:", buildDate)
			fmt.Println("Commit:    ", commitHash)
		}
		os.Exit(0)
	}

	HandleFlags()

	rcvr.NewReceiver()

	defer sampleFile.Close()
	defer rcvr.Close()

	rcvr.Run()
}

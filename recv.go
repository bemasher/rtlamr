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
	"math/rand"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/bemasher/rtlamr/decode"
	"github.com/bemasher/rtlamr/idm"
	"github.com/bemasher/rtlamr/parse"
	"github.com/bemasher/rtlamr/scm"
	"github.com/bemasher/rtltcp"
)

const (
	CenterFreq = 920200788
)

var rcvr Receiver

type Receiver struct {
	rtltcp.SDR
	d decode.Decoder
	p parse.Parser

	chIdx    int
	channels []uint32

	last    time.Time
	pIdx    int
	pattern map[int]map[int]int

	centers     []int
	centerOrder [50][]int
	centerIdx   [50]int
}

func (rcvr *Receiver) NewReceiver() {
	switch strings.ToLower(*msgType) {
	case "scm":
		rcvr.d = decode.NewDecoder(scm.NewPacketConfig(*symbolLength), *fastMag)
		rcvr.p = scm.NewParser()
	case "idm":
		rcvr.d = decode.NewDecoder(idm.NewPacketConfig(*symbolLength), *fastMag)
		rcvr.p = idm.NewParser()
	default:
		log.Fatalf("Invalid message type: %q\n", *msgType)
	}

	rcvr.channels = []uint32{
		909586111, 909782679, 909979247, 910175815, 911224178, 911420746, // 0
		911617314, 911813882, 912010451, 912207019, 912403587, 912600155, // 6
		912796723, 912993291, 913189859, 913386427, 913582995, 913779563, // 12
		913976132, 915024495, 915221063, 915417631, 915614199, 915810767, // 18
		916007335, 916203903, 916400471, 916597040, 916793608, 916990176, // 24
		917186744, 917383312, 917579880, 917776448, 918824811, 919021379, // 30
		919217947, 919414516, 919611084, 919807652, 920004220, 920200788, // 36
		920397356, 920593924, 920790492, 920987060, 921183628, 921380197, // 42
		921576765, 921773333, // 44
	}

	rcvr.chIdx = 42
	rcvr.pattern = make(map[int]map[int]int)

	rcvr.centers = []int{0, 9, 13, 23, 28, 39, 44}
	for idx := range rcvr.centerIdx {
		rcvr.centerIdx[idx] = 0
		rcvr.centerOrder[idx] = rand.Perm(len(rcvr.centers))
	}

	if !*quiet {
		rcvr.d.Cfg.Log()
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

	rcvr.SetSampleRate(uint32(rcvr.d.Cfg.SampleRate))
	rcvr.SetCenterFreq(rcvr.channels[rcvr.chIdx])
	rcvr.SetAGCMode(false)
	rcvr.SetGainMode(false)
	rcvr.SetGainByIndex(5)

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

	block := make([]byte, rcvr.d.Cfg.BlockSize2)

	isOffset := false
	var interval time.Duration

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

			if isOffset {
				interval = 30 * time.Second
			} else {
				interval = 45 * time.Second
			}

			if !rcvr.last.IsZero() && time.Since(rcvr.last) > interval {
				isOffset = true

				// Missing on a retrace is a penalty to the channel retraced.
				for ch := range rcvr.pattern[rcvr.pIdx] {
					if ch == rcvr.chIdx {
						rcvr.pattern[rcvr.pIdx][ch]--
					} else {
						rcvr.pattern[rcvr.pIdx][ch]++
					}
				}

				log.Println("Missed: ", rcvr.pIdx, rcvr.chIdx)

				rcvr.NextChannel()
			}

			pktFound := false
			for _, pkt := range rcvr.d.Decode(block) {
				rawMsg, err := rcvr.p.Parse(parse.NewDataFromBytes(pkt))
				if err != nil {
					// log.Println(err)
					continue
				}

				if len(meterID) > 0 && !meterID[uint(rawMsg.MeterID())] {
					continue
				}

				if len(meterType) > 0 && !meterType[uint(rawMsg.MeterType())] {
					continue
				}

				channel := rcvr.d.Periodogram.Execute(rcvr.d.Re, rcvr.d.Im)

				var msg parse.Logger
				msg = parse.HopMessage{
					time.Now(),
					rawMsg.MeterID(),
					rawMsg.MeterType(),
					rcvr.chIdx,
					channel,
				}

				if encoder == nil {
					// A nil encoder is just plain-text output.
					if *sampleFilename == os.DevNull {
						fmt.Fprintln(logFile, msg.StringNoOffset())
					} else {
						fmt.Fprintln(logFile, msg)
					}
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

				pktFound = true

				if _, exists := rcvr.pattern[rcvr.pIdx]; !exists {
					rcvr.pattern[rcvr.pIdx] = make(map[int]int)
				}

				// If this isn't a retrace, add it to the pattern.
				if _, exists := rcvr.pattern[rcvr.pIdx][rcvr.chIdx+channel]; !exists {
					rcvr.pattern[rcvr.pIdx][rcvr.chIdx+channel]++
				} else {
					// Reward retrace, penalize others.
					for ch := range rcvr.pattern[rcvr.pIdx] {
						if ch == rcvr.chIdx+channel {
							rcvr.pattern[rcvr.pIdx][ch]++
						} else {
							rcvr.pattern[rcvr.pIdx][ch]--
						}
					}
				}

				log.Printf("%#v\n", rcvr.pattern)

				isOffset = false
				rcvr.NextChannel()

				if *single {
					break
				}
			}

			if pktFound {
				if *sampleFilename != os.DevNull {
					_, err = sampleFile.Write(rcvr.d.IQ)
					if err != nil {
						log.Fatal("Error writing raw samples to file:", err)
					}
				}

				if *single {
					return
				}
			}
		}
	}
}

func (rcvr *Receiver) NextChannel() {
	defer rcvr.d.Reset()

	rcvr.last = time.Now()
	rcvr.centerIdx[rcvr.pIdx] = (rcvr.centerIdx[rcvr.pIdx] + 1) % len(rcvr.centers)
	rcvr.pIdx = (rcvr.pIdx + 1) % 50

	// Prune bad channels.
	for idx := range rcvr.pattern {
		for ch, count := range rcvr.pattern[idx] {
			if count <= 0 || ch < 0 || ch >= 50 {
				delete(rcvr.pattern[idx], ch)
			}
		}
	}

	pattern := make([]int, 50)
	for idx := range pattern {
		pattern[idx] = -1
	}
	for idx, channels := range rcvr.pattern {
		max := ^int(^uint(0) >> 1)
		for ch, count := range channels {
			if max < count {
				max = count
				pattern[idx] = ch
			}
		}
	}
	encoder.Encode(struct {
		Time    time.Time
		Pattern []int
	}{time.Now(), pattern})

	rcvr.chIdx = rcvr.centers[rcvr.centerOrder[rcvr.pIdx][rcvr.centerIdx[rcvr.pIdx]]]

	max := ^int(^uint(0) >> 1)
	for ch, count := range rcvr.pattern[rcvr.pIdx] {
		if max < count {
			max = count
			rcvr.chIdx = ch
		}
	}

	if max != ^int(^uint(0)>>1) {
		log.Println("Retrace:", rcvr.pIdx, rcvr.chIdx)
	} else {
		log.Println("Guess:  ", rcvr.pIdx, rcvr.chIdx)
	}

	rcvr.SetCenterFreq(rcvr.channels[rcvr.chIdx])
}

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
	rand.Seed(time.Now().UnixNano())
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

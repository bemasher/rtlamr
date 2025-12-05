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
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/bemasher/rtlamr/protocol"
	"github.com/bemasher/rtltcp"

	_ "github.com/bemasher/rtlamr/idm"
	_ "github.com/bemasher/rtlamr/netidm"
	_ "github.com/bemasher/rtlamr/r900"
	_ "github.com/bemasher/rtlamr/r900bcd"
	_ "github.com/bemasher/rtlamr/scm"
	_ "github.com/bemasher/rtlamr/scmplus"
)

var rcvr Receiver

type Receiver struct {
	rtltcp.SDR
	d  protocol.Decoder
	fc protocol.FilterChain

	ctx    context.Context
	cancel context.CancelFunc
	wg     *sync.WaitGroup

	err error
}

func (rcvr *Receiver) NewReceiver() {
	rcvr.ctx, rcvr.cancel = context.WithCancel(context.Background())
	rcvr.wg = &sync.WaitGroup{}

	rcvr.d = protocol.NewDecoder()

	// If the msgtype "all" is given alone, register and use scm, scm+, idm and r900.
	if _, all := msgType["all"]; all && len(msgType) == 1 {
		delete(msgType, "all")
		msgType["scm"] = true
		msgType["scm+"] = true
		msgType["idm"] = true
		msgType["r900"] = true
	}

	// For each given msgType, register it with the decoder.
	for name := range msgType {
		p, err := protocol.NewParser(name, *symbolLength)
		if err != nil {
			log.Fatal(err)
		}

		rcvr.d.RegisterProtocol(p)
	}

	// Allocate the internal buffers of the decoder.
	rcvr.d.Allocate()

	// Connect to rtl_tcp server.
	if rcvr.err = rcvr.Connect(nil); rcvr.err != nil {
		log.Fatalf("%+v", errors.Wrap(rcvr.err, "rcvr.Connect"))
	}

	cfg := rcvr.d.Cfg

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
			if f.Value.String() == "true" {
				rcvr.fc.Add(NewUniqueFilter())
			}
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

	rcvr.d.Cfg = cfg
	rcvr.d.Log()

	// Tell the user how many gain settings were reported by rtl_tcp.
	log.Println("GainCount:", rcvr.SDR.Info.GainCount)
}

func (rcvr *Receiver) Close() {
	rcvr.cancel()
	rcvr.wg.Wait()
	rcvr.SDR.Close()
}

func (rcvr *Receiver) Run() {
	rcvr.wg.Add(3)

	sampleBuf := &bytes.Buffer{}

	// Allocate a channel of blocks.
	blockCh := make(chan []byte)

	// Make maps for tracking messages spanning sample blocks.
	prev := map[protocol.Digest]bool{}
	next := map[protocol.Digest]bool{}

	go func() {
		defer rcvr.wg.Done()
		<-rcvr.ctx.Done()
		// Consume any in-flight blocks.
		for range blockCh {
		}
	}()

	// Read and send sample blocks to the decoder.
	go func() {
		defer rcvr.cancel()
		defer close(blockCh)
		defer rcvr.wg.Done()

		// Make two sample blocks, one for reading, and one for the receiver to
		// decode, these are exchanged each time we read a new block.
		blockA := make([]byte, rcvr.d.Cfg.BlockSize2)
		blockB := make([]byte, rcvr.d.Cfg.BlockSize2)

		for {
			select {
			// Exit if we've been told to stop.
			case <-rcvr.ctx.Done():
				return
			default:
				rcvr.err = rcvr.SetDeadline(time.Now().Add(5 * time.Second))
				if rcvr.err != nil {
					rcvr.err = errors.Wrap(rcvr.err, "rcvr.SetDeadline")
					return
				}

				// Read new sample block.
				_, rcvr.err = io.ReadFull(rcvr, blockA)
				if rcvr.err != nil {
					rcvr.err = errors.Wrap(rcvr.err, "io.ReadFull")
					return
				}

				// Send the sample block.
				blockCh <- blockA

				// Exchange blocks for next read.
				blockA, blockB = blockB, blockA
			}
		}
	}()

	go func() {
		defer rcvr.cancel()
		defer rcvr.wg.Done()

		for {
			select {
			case <-rcvr.ctx.Done():
				return
			case block, ok := <-blockCh:
				if !ok {
					continue
				}

				// Clear next map for this sample block.
				for key := range next {
					delete(next, key)
				}

				// Discard the oldest block from the buffer if
				// it's full and write the new block to it.
				if sampleBuf.Len() > rcvr.d.Cfg.BufferLength<<1 {
					io.CopyN(io.Discard, sampleBuf, int64(len(block)))
				}
				sampleBuf.Write(block)

				pktFound := false

				// For each message returned
				for msg := range rcvr.d.Decode(block) {
					// If the filterchain rejects the message, skip it.
					if !rcvr.fc.Match(msg) {
						continue
					}

					// Make a new LogMessage
					var logMsg protocol.LogMessage
					logMsg.Time = time.Now()
					if s, ok := sampleWriter.(io.Seeker); ok {
						logMsg.Offset, _ = s.Seek(0, io.SeekCurrent)
					}
					logMsg.Length = sampleBuf.Len()
					logMsg.Type = msg.MsgType()
					logMsg.Message = msg

					// This should be unique enough to identify a message between blocks.
					msgDigest := protocol.NewDigest(msg)

					// Mark the message as seen for the next loop.
					next[msgDigest] = true

					// If the message was seen in the previous loop, skip it.
					if prev[msgDigest] {
						continue
					}

					// Encode the message
					rcvr.err = encoder.Encode(logMsg)
					rcvr.err = errors.Wrap(rcvr.err, "encoder.Encode")

					if rcvr.err != nil {
						return
					}

					pktFound = true
					if *single {
						if len(meterID.UintMap) == 0 {
							break
						} else {
							delete(meterID.UintMap, uint(msg.MeterID()))
						}
					}
				}

				if pktFound {
					_, err := sampleWriter.Write(sampleBuf.Bytes())
					if err != nil {
						log.Fatal("Error writing raw samples to file:", err)
					}
					if *single && len(meterID.UintMap) == 0 {
						rcvr.cancel()
						return
					}
				}

				// Swap next and previous digest maps.
				next, prev = prev, next
			}
		}
	}()
}

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
}

func main() {
	rcvr.RegisterFlags()
	RegisterFlags()
	EnvOverride()
	flag.Parse()
	rcvr.HandleFlags()

	if *version {
		if info, ok := debug.ReadBuildInfo(); ok {
			fmt.Printf("%+v\n", info)
		} else {
			log.Fatal("could not read build info")
		}
		os.Exit(0)
	}

	HandleFlags()

	rcvr.NewReceiver()

	defer func() {
		if c, ok := sampleWriter.(io.Closer); ok {
			c.Close()
		}
		rcvr.Close()

		if rcvr.err != nil {
			log.Fatalf("%+v\n", rcvr.err)
		}
	}()

	start := time.Now()
	rcvr.Run()

	// Setup signal channel for interruption.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	// Setup time limit channel
	timeLimitCh := make(<-chan time.Time, 1)
	if *timeLimit != 0 {
		timeLimitCh = time.After(*timeLimit)
	}

	select {
	case sig := <-sigCh:
		log.Println("Received Signal:", sig)
	case <-timeLimitCh:
		log.Println("Time Limit Reached:", time.Since(start))
	case <-rcvr.ctx.Done():
		log.Println("Receiver context cancelled.")
	}

	rcvr.Close()
}

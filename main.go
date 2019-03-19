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
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

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

	stop chan struct{}
}

func (rcvr *Receiver) NewReceiver() {
	rcvr.d = protocol.NewDecoder()

	rcvr.stop = make(chan struct{}, 1)

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
	if err := rcvr.Connect(nil); err != nil {
		log.Fatal(err)
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

	rcvr.d.Cfg = cfg
	rcvr.d.Log()

	// Tell the user how many gain settings were reported by rtl_tcp.
	log.Println("GainCount:", rcvr.SDR.Info.GainCount)

	return
}

func (rcvr *Receiver) Close() {
	rcvr.stop <- struct{}{}
	rcvr.SDR.Close()
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

	sampleBuf := new(bytes.Buffer)
	start := time.Now()

	// Allocate a channel of blocks.
	blockCh := make(chan []byte)

	// Make maps for tracking messages spanning sample blocks.
	prev := map[protocol.Digest]bool{}
	next := map[protocol.Digest]bool{}

	// Read and send sample blocks to the decoder.
	go func() {
		// Make two sample blocks, one for reading, and one for the receiver to
		// decode, these are exchanged each time we read a new block.
		blockA := make([]byte, rcvr.d.Cfg.BlockSize2)
		blockB := make([]byte, rcvr.d.Cfg.BlockSize2)

		// When exiting this goroutine, close the block channel.
		defer close(blockCh)

		for {
			select {
			// Exit if we've been told to stop.
			case <-rcvr.stop:
				return
			default:
				// Read new sample block.
				_, err := io.ReadFull(rcvr, blockA)

				// If we get an EOF, exit.
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					log.Println("encountered eof:", err)
					return
				}

				// If we get a network operation error.
				if opErr, ok := err.(*net.OpError); ok {
					// If temporary, keep reading.
					if opErr.Temporary() {
						log.Printf("operr: temporary: %+v\n", opErr)
						continue
					}

					// If it's not temporary, exit.
					log.Printf("operr: %+v\n", opErr)
					return
				}

				// Send the sample block.
				blockCh <- blockA

				// Exchange blocks for next read.
				blockA, blockB = blockB, blockA
			}
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
		case block, ok := <-blockCh:
			// If blockCh is closed, exit.
			if !ok {
				return
			}

			// Clear next map for this sample block.
			for key := range next {
				delete(next, key)
			}

			// If dumping samples, discard the oldest block from the buffer if
			// it's full and write the new block to it.
			if *sampleFilename != os.DevNull {
				if sampleBuf.Len() > rcvr.d.Cfg.BufferLength<<1 {
					io.CopyN(ioutil.Discard, sampleBuf, int64(len(block)))
				}
				sampleBuf.Write(block)
			}

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
				logMsg.Offset, _ = sampleFile.Seek(0, os.SEEK_CUR)
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
				err := encoder.Encode(logMsg)
				if err != nil {
					log.Fatal("Error encoding message: ", err)
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

			// Swap next and previous digest maps.
			next, prev = prev, next
		}
	}
}

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
}

var (
	buildTag   = "dev"     // v#.#.#
	buildDate  = "unknown" // date -u '+%Y-%m-%d'
	commitHash = "unknown" // git rev-parse HEAD
)

func main() {
	rcvr.RegisterFlags()
	RegisterFlags()
	EnvOverride()
	flag.Parse()
	rcvr.HandleFlags()

	if *version {
		fmt.Println("Build Tag: ", buildTag)
		fmt.Println("Build Date:", buildDate)
		fmt.Println("Commit:    ", commitHash)
		os.Exit(0)
	}

	HandleFlags()

	rcvr.NewReceiver()

	defer sampleFile.Close()
	defer rcvr.Close()

	rcvr.Run()
}

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
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
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

	ctx  context.Context
	canc context.CancelCauseFunc
	wg   *sync.WaitGroup
}

func (rcvr *Receiver) NewReceiver(ctx context.Context) {
	rcvr.ctx, rcvr.canc = context.WithCancelCause(ctx)

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
			slog.Error("message type", "error", err)
		}

		rcvr.d.RegisterProtocol(p)
	}

	// Allocate the internal buffers of the decoder.
	rcvr.d.Allocate()

	// Connect to rtl_tcp server.
	if err := rcvr.Connect(); err != nil {
		rcvr.canc(fmt.Errorf("rcvr.Connect: %w", err))
		return
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
	slog.Info("rtl_tcp", "GainCount", rcvr.SDR.Info.GainCount)
}

func (rcvr *Receiver) Close() {
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
		defer rcvr.canc(nil)
		defer close(blockCh)
		defer rcvr.wg.Done()

		tick := time.Tick(time.Second)

		bytesRead := 0

		for {
			block := make([]byte, rcvr.d.Cfg.BlockSize2)

			err := rcvr.SetDeadline(time.Now().Add(5 * time.Second))
			if err != nil {
				rcvr.canc(fmt.Errorf("rcvr.SetDeadline: %w", err))
				return
			}

			// Read new sample block.
			for offset := 0; offset < len(block); {
				var n int

				n, err = rcvr.Read(block[offset:])
				if err != nil {
					rcvr.canc(fmt.Errorf("rcvr.Read: %w", err))
					return
				}

				offset += n
				bytesRead += n
			}

			select {
			case <-tick:
				// Complain if received samples are less than 90% configured rate.
				if bytesRead>>1 < (rcvr.d.Cfg.SampleRate * 9 / 10) {
					slog.Warn("not keeping up with rtl_tcp", "rate", bytesRead>>1)
				}
				bytesRead = 0
			default:
			}

			select {
			// Exit if we've been told to stop.
			case <-rcvr.ctx.Done():
				return
			case blockCh <- block: // Send the sample block.
			}
		}
	}()

	go func() {
		defer rcvr.canc(nil)
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
					err := encoder.Encode(logMsg)
					if err != nil {
						rcvr.canc(fmt.Errorf("encoder.Encode: %w", err))
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
						slog.Error("error writing raw samples to file", "error", err)
						os.Exit(1)
					}
					if *single && len(meterID.UintMap) == 0 {
						rcvr.canc(errors.New("single: received messages from all meters"))
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
	_, file, _, ok := runtime.Caller(0)
	dir := ""
	if ok {
		dir = filepath.Dir(file)
		dir = filepath.Dir(dir) + string(filepath.Separator)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr,
		&slog.HandlerOptions{
			AddSource: true,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.SourceKey {
					source := a.Value.Any().(*slog.Source)
					source.File = strings.TrimPrefix(filepath.FromSlash(source.File), dir)
				}
				return a
			},
		})))
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
			slog.Error("could not read build info")
		}
		os.Exit(0)
	}

	HandleFlags()

	// Setup signal channel for interruption.
	ctx, canc := signal.NotifyContext(context.Background(), os.Interrupt)
	defer canc()

	// Make sure sampleWriter is closed.
	defer func() {
		if c, ok := sampleWriter.(io.Closer); ok {
			slog.Info("closing sampleWriter")
			c.Close()
		}
	}()

	// Set context timeout when provided.
	if *timeLimit != 0 {
		ctx, canc = context.WithTimeout(ctx, *timeLimit)
		defer canc()
	}

	rcvr.NewReceiver(ctx)
	defer rcvr.Close()

	rcvr.Run()

	select {
	case <-ctx.Done():
		err := context.Cause(ctx)
		slog.Info("main context cancelled", "cause", err)
	case <-rcvr.ctx.Done():
		err := context.Cause(rcvr.ctx)
		slog.Info("receiver context cancelled", "cause", err)
	}
}

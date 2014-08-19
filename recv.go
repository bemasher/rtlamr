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
	"log"
	"math"
	"os"
	"os/signal"

	"github.com/bemasher/rtltcp"
)

const (
	CenterFreq = 920299072
	DataRate   = 32768
)

var rcvr Receiver

type Receiver struct {
	rtltcp.SDR

	lut     MagLUT
	cfg     PacketConfig
	decoder PacketDecoder
}

func (rcvr *Receiver) NewReceiver(pktDecoder PacketDecoder) {
	rcvr.RegisterFlags()
	rcvr.Flags.FlagSet.Parse(os.Args[1:])

	rcvr.decoder = pktDecoder
	rcvr.cfg = pktDecoder.PacketConfig()

	rcvr.lut = NewMagLUT()

	// Connect to rtl_tcp server.
	if err := rcvr.Connect(nil); err != nil {
		log.Fatal(err)
	}

	rcvr.HandleFlags()

	// Tell the user how many gain settings were reported by rtl_tcp.
	log.Println("GainCount:", rcvr.SDR.Info.GainCount)

	// Set some parameters for listening.
	rcvr.SetCenterFreq(CenterFreq)
	rcvr.SetSampleRate(uint32(rcvr.cfg.SampleRate))
	rcvr.SetGainMode(true)

	return
}

// Clean up rtl_tcp connection and destroy preamble detection plan.
func (rcvr *Receiver) Close() {
	rcvr.SDR.Close()
	rcvr.decoder.Close()
}

func (rcvr *Receiver) Run() {
	// Setup signal channel for interruption.
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Kill, os.Interrupt)

	// Allocate sample and demodulated signal buffers.
	raw := make([]byte, (rcvr.cfg.PacketLength+rcvr.cfg.BlockSize)<<1)
	amBuf := make([]float64, rcvr.cfg.PacketLength+rcvr.cfg.BlockSize)

	for {
		// Exit on interrupt or time limit, otherwise receive.
		select {
		case <-sigint:
			return
		default:
			copy(raw, raw[rcvr.cfg.BlockSize<<1:])
			copy(amBuf, amBuf[rcvr.cfg.BlockSize:])

			// Read new sample block.
			_, err := rcvr.Read(raw[rcvr.cfg.PacketLength<<1:])
			if err != nil {
				log.Fatal("Error reading samples: ", err)
			}

			// AM Demodulate
			block := amBuf[rcvr.cfg.PacketLength:]
			rawBlock := raw[rcvr.cfg.PacketLength<<1:]
			for idx := range block {
				block[idx] = math.Sqrt(rcvr.lut[rawBlock[idx<<1]] + rcvr.lut[rawBlock[(idx<<1)+1]])
			}

			// Detect preamble in first half of demod buffer.
			align := rcvr.decoder.SearchPreamble(amBuf)

			// Bad framing, catch message on next block.
			if uint(align) > rcvr.cfg.BlockSize {
				continue
			}

			// Filter signal and bit slice enough data to catch the preamble.
			filtered := MatchedFilter(rcvr.cfg, amBuf[align:], int(rcvr.cfg.PreambleSymbols>>1))
			data := BitSlice(filtered)

			// If the preamble matches.
			if data.Bits == rcvr.cfg.PreambleBits {
				// Filter, slice and parse the rest of the buffered samples.
				filtered := MatchedFilter(rcvr.cfg, amBuf[align:], int(rcvr.cfg.PacketSymbols>>1))
				data := BitSlice(filtered)

				// Parse packet.
				pkt, err := rcvr.decoder.Decode(data)
				if err == nil {
					log.Printf("%+v\n", pkt)
				}
			}
		}
	}
}

type Data struct {
	Bits  string
	Bytes []byte
}

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
	log.SetOutput(os.Stdout)
}

func main() {
	rcvr.NewReceiver(NewIDMDecoder(73))
	defer rcvr.Close()

	log.Println("Server:", rcvr.Flags.ServerAddr)
	log.Println("BlockSize:", rcvr.cfg.BlockSize)
	log.Println("SampleRate:", rcvr.cfg.SampleRate)
	log.Println("DataRate:", DataRate)
	log.Println("SymbolLength:", rcvr.cfg.SymbolLength)
	log.Println("PreambleSymbols:", rcvr.cfg.PreambleSymbols)
	log.Println("PreambleLength:", rcvr.cfg.PreambleLength)
	log.Println("PacketSymbols:", rcvr.cfg.PacketSymbols)
	log.Println("PacketLength:", rcvr.cfg.PacketLength)
	log.Println("PreambleBits:", rcvr.cfg.PreambleBits)
	log.Println("Checksum:", rcvr.decoder.CRC())

	rcvr.Run()
}

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

package protocol

import (
	"log"
	"math"
	"strings"
	"sync"
)

// PacketConfig specifies packet-specific radio configuration.
type PacketConfig struct {
	Protocol string
	Preamble string

	DataRate int

	BlockSize, BlockSize2    int
	ChipLength, SymbolLength int
	SampleRate               int

	PreambleSymbols, PacketSymbols int
	PreambleLength, PacketLength   int

	BufferLength int
	CenterFreq   uint32
}

func (d Decoder) Log() {
	log.Println("CenterFreq:", d.Cfg.CenterFreq)
	log.Println("SampleRate:", d.Cfg.SampleRate)
	log.Println("DataRate:", d.Cfg.DataRate)
	log.Println("ChipLength:", d.Cfg.ChipLength)
	log.Println("PreambleSymbols:", d.Cfg.PreambleSymbols)
	log.Println("PreambleLength:", d.Cfg.PreambleLength)
	log.Println("PacketSymbols:", d.Cfg.PacketSymbols)
	log.Println("PacketLength:", d.Cfg.PacketLength)

	var preambles []string
	for preamble, _ := range d.preambleStrs {
		preambles = append(preambles, preamble)
	}

	log.Println("Protocols:", strings.Join(d.protocols, ","))
	log.Println("Preambles:", strings.Join(preambles, ","))
}

// Decoder contains buffers and radio configuration.
type Decoder struct {
	Cfg PacketConfig
	wg  *sync.WaitGroup

	Signal    []float64
	Quantized []byte

	csum  []float64
	demod Demodulator

	preambleStrs map[string]bool
	preambles    map[string][]Parser
	protocols    []string

	pkt []byte

	packed       []byte
	sIdxA, sIdxB []int
}

func NewDecoder() Decoder {
	return Decoder{
		wg:           new(sync.WaitGroup),
		preambles:    make(map[string][]Parser),
		preambleStrs: make(map[string]bool),
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Using a single decoder, register protocols to pass off decoded packets to.
func (d *Decoder) RegisterProtocol(p Parser) {
	// Protocols such as R900 require the use of internal decoder data for further processing.
	p.SetDecoder(d)

	// Take the largest value for each protocol. Some values are simply overridden
	d.Cfg.CenterFreq = p.Cfg().CenterFreq
	d.Cfg.DataRate = max(d.Cfg.DataRate, p.Cfg().DataRate)
	d.Cfg.ChipLength = max(d.Cfg.ChipLength, p.Cfg().ChipLength)
	d.Cfg.PreambleSymbols = max(d.Cfg.PreambleSymbols, p.Cfg().PreambleSymbols)
	d.Cfg.PacketSymbols = max(d.Cfg.PacketSymbols, p.Cfg().PacketSymbols)

	// Take a string of ascii 0's and 1's, convert them to numerical 0's and 1's.
	// This is used during preamble searching.
	preambleBytes := make([]byte, len(p.Cfg().Preamble))
	for idx, bit := range p.Cfg().Preamble {
		if bit == '1' {
			preambleBytes[idx] = 1
		}
	}

	// Keep track of registered preambles for logging back to the user.
	d.preambleStrs[p.Cfg().Preamble] = true

	// Associate the parser with the appropriate preamble.
	d.preambles[string(preambleBytes)] = append(d.preambles[string(preambleBytes)], p)

	// Add the protocol to the list for logging back to the user.
	d.protocols = append(d.protocols, p.Cfg().Protocol)
}

// Calculate lengths and allocate internal buffers.
func (d *Decoder) Allocate() {
	d.Cfg.SymbolLength = d.Cfg.ChipLength << 1
	d.Cfg.SampleRate = d.Cfg.DataRate * d.Cfg.ChipLength

	d.Cfg.PreambleLength = d.Cfg.PreambleSymbols * d.Cfg.SymbolLength
	d.Cfg.PacketLength = d.Cfg.PacketSymbols * d.Cfg.SymbolLength

	d.Cfg.BlockSize = NextPowerOf2(d.Cfg.PreambleLength)
	d.Cfg.BlockSize2 = d.Cfg.BlockSize << 1

	d.Cfg.BufferLength = d.Cfg.PacketLength + d.Cfg.BlockSize

	// Allocate necessary buffers.
	d.Signal = make([]float64, d.Cfg.BlockSize+d.Cfg.SymbolLength)
	d.Quantized = make([]byte, d.Cfg.BufferLength)

	d.csum = make([]float64, len(d.Signal)+1)

	// Calculate magnitude lookup table specified by -fastmag flag.
	d.demod = NewMagLUT()

	// Signal up to the final stage is 1-bit per byte. Allocate a buffer to
	// store packed version 8-bits per byte.
	d.pkt = make([]byte, (d.Cfg.PacketSymbols+7)>>3)

	d.sIdxA = make([]int, 0, d.Cfg.BlockSize)
	d.sIdxB = make([]int, 0, d.Cfg.BlockSize)

	d.packed = make([]byte, (d.Cfg.BlockSize+d.Cfg.PreambleLength+7)>>3)

	return
}

// Decode accepts a sample block and returns a channel of messages.
func (d Decoder) Decode(input []byte) chan Message {
	// Shift buffers to append new block.
	copy(d.Signal, d.Signal[d.Cfg.BlockSize:])
	copy(d.Quantized, d.Quantized[d.Cfg.BlockSize:])

	// Compute the magnitude of the new block.
	d.demod.Execute(input, d.Signal[d.Cfg.SymbolLength:])

	// Perform matched filter on new block.
	d.Filter(d.Signal, d.Quantized[d.Cfg.PacketLength:])

	msgCh := make(chan Message)

	// For each preamble.
	for preamble, parsers := range d.preambles {
		// Get a list of packets with valid preambles.
		pkts := d.Slice(d.Search([]byte(preamble)))

		// Increment the wait group for all the parsers we will run on these packets.
		d.wg.Add(len(parsers))

		// For each parser, run it on the given packets.
		for _, p := range parsers {
			go p.Parse(pkts, msgCh, d.wg)
		}
	}

	// Close the message channel when all of the parsers have finished.
	go func() {
		d.wg.Wait()
		close(msgCh)
	}()

	return msgCh
}

// A Demodulator knows how to demodulate an array of uint8 IQ samples into an
// array of float64 samples.
type Demodulator interface {
	Execute([]byte, []float64)
}

// Default Magnitude Lookup Table
type MagLUT []float64

// Pre-computes normalized squares with most common DC offset for rtl-sdr dongles.
func NewMagLUT() (lut MagLUT) {
	lut = make([]float64, 0x100)
	for idx := range lut {
		lut[idx] = (127.5 - float64(idx)) / 127.5
		lut[idx] *= lut[idx]
	}
	return
}

// Calculates complex magnitude on given IQ stream writing result to output.
func (lut MagLUT) Execute(input []byte, output []float64) {
	i := 0
	for idx := range output {
		output[idx] = lut[input[i]] + lut[input[i+1]]
		i += 2
	}
}

// Matched filter for Manchester coded signals. Output signal's sign at each
// sample determines the bit-value due to Manchester symbol odd symmetry.
func (d Decoder) Filter(input []float64, output []byte) {
	// Computing the cumulative summation over the signal simplifies
	// filtering to the difference of a pair of subtractions.
	var sum float64
	for idx, v := range input {
		sum += v
		d.csum[idx+1] = sum
	}

	// Filter result is difference of summation of lower and upper chips.
	lower := d.csum[d.Cfg.ChipLength:]
	upper := d.csum[d.Cfg.SymbolLength:]
	for idx, l := range lower[:len(output)] {
		f := (l - d.csum[idx]) - (upper[idx] - l)
		output[idx] = 1 - byte(math.Float64bits(f)>>63)
	}

	return
}

// Return a list of indices into the quantized signal at which a valid preamble
// exists.
// 1. Pack the quantized signal into bytes.
// 2. Build a list of indices by eliminating bytes that contain no bits matching
//    the first bit of the preamble.
// 3. Continue eliminating indices at which the preamble cannot exist.
// 4. Convert indices from byte-based to sample-based.
// 5. Check each of these indices for the preamble.
func (d *Decoder) Search(preamble []byte) []int {
	symLenByte := d.Cfg.SymbolLength >> 3

	// Pack the bit-wise quantized signal into bytes.
	for bIdx := range d.packed {
		var b byte
		for _, qBit := range d.Quantized[bIdx<<3 : (bIdx+1)<<3] {
			b = (b << 1) | qBit
		}
		d.packed[bIdx] = b
	}

	// For each bit in the preamble.
	for pIdx, pBit := range preamble {
		// For 0, mask is 0xFF, for 1, mask is 0x00
		pBit = (pBit ^ 1) * 0xFF
		offset := pIdx * symLenByte
		// If this is the first bit of the preamble.
		if pIdx == 0 {
			// Truncate the list of possible indices.
			d.sIdxA = d.sIdxA[:0]
			// For each packed byte.
			for qIdx, b := range d.packed[:d.Cfg.BlockSize>>3] {
				// If the byte contains any bits that match the current preamble bit.
				if b != pBit {
					// Add the index to the list.
					d.sIdxA = append(d.sIdxA, qIdx)
				}
			}
		} else {
			// From the list of possible indices, eliminate any indices at which
			// the preamble does not exist for the current preamble bit.
			d.sIdxB, d.sIdxA = searchPassByte(pBit, d.packed[offset:], d.sIdxA, d.sIdxB[:0])

			// If we've eliminated all possible indices, there is no preamble.
			if len(d.sIdxA) == 0 {
				return nil
			}
		}
	}

	symLen := d.Cfg.SymbolLength

	// Truncate index list B.
	d.sIdxB = d.sIdxB[:0]
	// For each index in list A.
	for _, qIdx := range d.sIdxA {
		// For each bit in the current byte.
		for idx := 0; idx < 8; idx++ {
			// Add the signal-based index to index list B.
			d.sIdxB = append(d.sIdxB, (qIdx<<3)+idx)
		}
	}

	// Swap index lists A and B.
	d.sIdxA, d.sIdxB = d.sIdxB, d.sIdxA

	// Check which indices the preamble actually exists at.
	for pIdx, pBit := range preamble {
		offset := pIdx * symLen
		offsetQuantized := d.Quantized[offset : offset+d.Cfg.BlockSize]

		// Search the list of possible indices for indices at which the preamble actually exists.
		d.sIdxB, d.sIdxA = searchPass(pBit, offsetQuantized, d.sIdxA, d.sIdxB[:0])

		// If at the current bit of the preamble, there are no indices left to
		// check, the preamble does not exist in the current sample block.
		if len(d.sIdxA) == 0 {
			return nil
		}
	}

	return d.sIdxA
}

func searchPassByte(pBit byte, sig []byte, a, b []int) ([]int, []int) {
	for _, qIdx := range a {
		if sig[qIdx] != pBit {
			b = append(b, qIdx)
		}
	}

	return a, b
}

func searchPass(pBit byte, sig []byte, a, b []int) ([]int, []int) {
	for _, qIdx := range a {
		if sig[qIdx] == pBit {
			b = append(b, qIdx)
		}
	}

	return a, b
}

// Given a list of indices the preamble exists at, sample the appropriate bits
// of the signal's bit-decision. Pack bits of each index into an array of bytes
// and return each packet.
func (d Decoder) Slice(indices []int) (pkts []Data) {
	// For each of the indices the preamble exists at.
	for _, qIdx := range indices {
		// Check that we're still within the first sample block. We'll catch
		// the message on the next sample block otherwise.
		if qIdx > d.Cfg.BlockSize {
			continue
		}

		// Packet is 1 bit per byte, pack to 8-bits per byte.
		for pIdx := 0; pIdx < d.Cfg.PacketSymbols; pIdx++ {
			d.pkt[pIdx>>3] <<= 1
			d.pkt[pIdx>>3] |= d.Quantized[qIdx+(pIdx*d.Cfg.SymbolLength)]
		}

		// Store the packet in the seen map and append to the packet list.
		data := NewData(d.pkt)
		data.Idx = qIdx
		pkts = append(pkts, data)
	}

	return
}

func NextPowerOf2(v int) int {
	return 1 << uint(math.Ceil(math.Log2(float64(v))))
}

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

package decode

import (
	"fmt"
	"log"
	"math"
)

// PacketConfig specifies packet-specific radio configuration.
type PacketConfig struct {
	DataRate int

	BlockSize, BlockSize2    int
	ChipLength, SymbolLength int
	SampleRate               int

	PreambleSymbols, PacketSymbols int
	PreambleLength, PacketLength   int
	Preamble                       string

	BufferLength int

	CenterFreq uint32
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
	log.Println("Preamble:", d.Cfg.Preamble)
}

// Decoder contains buffers and radio configuration.
type Decoder struct {
	Cfg PacketConfig

	Signal    []float64
	Filtered  []float64
	Quantized []byte

	csum  []float64
	demod Demodulator

	preamble []byte
	slices   [][]byte

	preambleFinder *byteFinder

	pkt []byte
}

// Create a new decoder with the given packet configuration.
func NewDecoder(cfg PacketConfig) (d Decoder) {
	d.Cfg = cfg

	d.Cfg.SymbolLength = d.Cfg.ChipLength << 1
	d.Cfg.SampleRate = d.Cfg.DataRate * d.Cfg.ChipLength

	d.Cfg.PreambleLength = d.Cfg.PreambleSymbols * d.Cfg.SymbolLength
	d.Cfg.PacketLength = d.Cfg.PacketSymbols * d.Cfg.SymbolLength

	d.Cfg.BlockSize = NextPowerOf2(d.Cfg.PreambleLength)
	d.Cfg.BlockSize2 = d.Cfg.BlockSize << 1

	d.Cfg.BufferLength = d.Cfg.PacketLength + d.Cfg.BlockSize

	// Allocate necessary buffers.
	d.Signal = make([]float64, d.Cfg.BlockSize+d.Cfg.SymbolLength)
	d.Filtered = make([]float64, d.Cfg.BlockSize)
	d.Quantized = make([]byte, d.Cfg.BufferLength)

	d.csum = make([]float64, len(d.Signal)+1)

	// Calculate magnitude lookup table specified by -fastmag flag.
	d.demod = NewMagLUT()

	// Pre-calculate a byte-slice version of the preamble for searching.
	d.preamble = make([]byte, d.Cfg.PreambleSymbols)
	for idx := range d.Cfg.Preamble {
		if d.Cfg.Preamble[idx] == '1' {
			d.preamble[idx] = 1
		}
	}

	// Slice quantized sample buffer to make searching for the preamble more
	// memory local.
	d.slices = make([][]byte, d.Cfg.SymbolLength)

	symbolsPerBlock := (d.Cfg.BlockSize + d.Cfg.PreambleLength) / d.Cfg.SymbolLength
	for symbolOffset := range d.slices {
		d.slices[symbolOffset] = make([]byte, symbolsPerBlock)
	}

	d.preambleFinder = makeByteFinder(d.preamble)

	// Signal up to the final stage is 1-bit per byte. Allocate a buffer to
	// store packed version 8-bits per byte.
	d.pkt = make([]byte, (d.Cfg.PacketSymbols+7)>>3)

	return
}

// Decode accepts a sample block and performs various DSP techniques to extract a packet.
func (d Decoder) Decode(input []byte) []int {
	// Shift buffers to append new block.
	copy(d.Signal, d.Signal[d.Cfg.BlockSize:])
	copy(d.Quantized, d.Quantized[d.Cfg.BlockSize:])

	// Compute the magnitude of the new block.
	d.demod.Execute(input, d.Signal[d.Cfg.SymbolLength:])

	// Perform matched filter on new block.
	d.Filter(d.Signal, d.Filtered)

	// Perform bit-decision on new block.
	Quantize(d.Filtered, d.Quantized[d.Cfg.PacketLength:])

	// Pack the quantized signal into slices for searching.
	d.Transpose(d.Quantized)

	// Return a list of indexes the preamble exists at.
	return d.Search()
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
func (d Decoder) Filter(input, output []float64) {
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
		output[idx] = (l - d.csum[idx]) - (upper[idx] - l)
	}

	return
}

// Bit-value is determined by the sign of each sample after filtering.
func Quantize(input []float64, output []byte) {
	// There's really not much we can optimize here as it depends pretty
	// heavily on the content of the input, which is unbiased on average.
	for idx, val := range input {
		output[idx] = 1 - byte(math.Float64bits(val)>>63)
	}

	return
}

// Transpose quantized signal into slices such that the first rank represents
// sample offsets and the second represents the value of each symbol from the
// given offset.
//
// Transforms:
// <--Sym1--><--Sym2--><--Sym3--><--Sym4--><--Sym5--><--Sym6--><--Sym7--><--Sym8-->
// <11111111><22222222><33333333><44444444><55555555><66666666><77777777><88888888>
// to:
// <12345678><12345678><12345678><12345678><12345678><12345678><12345678><12345678>
func (d *Decoder) Transpose(input []byte) {
	for symbolOffset, slice := range d.slices {
		symbolInInput := 0
		offsetInput := input[symbolOffset:]
		for symbolIdx := range slice {
			slice[symbolIdx] = offsetInput[symbolInInput]
			symbolInInput += d.Cfg.SymbolLength
		}
	}

	return
}

// For each sample offset look for the preamble. Return a list of indexes the
// preamble is found at. Indexes are absolute in the unsliced quantized
// buffer.
func (d *Decoder) Search() (indexes []int) {
	for symbolOffset, slice := range d.slices {
		lastIdx := 0
		idx := 0
		for {
			idx = d.preambleFinder.next(slice[lastIdx:])
			if idx != -1 {
				indexes = append(indexes, (lastIdx+idx)*d.Cfg.SymbolLength+symbolOffset)
				lastIdx += idx + 1
			} else {
				break
			}
		}
	}

	return
}

// Given a list of indices the preamble exists at, sample the appropriate bits
// of the signal's bit-decision. Pack bits of each index into an array of byte
// arrays and return.
func (d Decoder) Slice(indices []int) (pkts [][]byte) {
	// It is likely that a message will be successfully decoded at multiple indices,
	// only keep track of unique instances.
	seen := make(map[string]bool)

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
		pktStr := fmt.Sprintf("%02X", d.pkt)
		if !seen[pktStr] {
			seen[pktStr] = true
			pkts = append(pkts, make([]byte, len(d.pkt)))
			copy(pkts[len(pkts)-1], d.pkt)
		}
	}

	return
}

func NextPowerOf2(v int) int {
	return 1 << uint(math.Ceil(math.Log2(float64(v))))
}

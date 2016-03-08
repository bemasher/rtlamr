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

	BlockSize, BlockSize2       int
	SymbolLength, SymbolLength2 int
	SampleRate                  int

	PreambleSymbols, PacketSymbols int
	PreambleLength, PacketLength   int
	Preamble                       string

	BufferLength int

	CenterFreq uint32
}

func (cfg PacketConfig) Decimate(decimation int) PacketConfig {
	cfg.BlockSize /= decimation
	cfg.BlockSize2 /= decimation
	cfg.SymbolLength /= decimation
	cfg.SymbolLength2 /= decimation
	cfg.SampleRate /= decimation
	cfg.DataRate /= decimation

	cfg.PreambleLength /= decimation
	cfg.PacketLength /= decimation

	cfg.BufferLength /= decimation

	return cfg
}

func (d Decoder) Log() {
	if d.Decimation != 1 {
		log.Printf("BlockSize: %d|%d\n", d.Cfg.BlockSize, d.DecCfg.BlockSize)
		log.Println("CenterFreq:", d.Cfg.CenterFreq)
		log.Printf("SampleRate: %d|%d\n", d.Cfg.SampleRate, d.DecCfg.SampleRate)
		log.Printf("DataRate: %d|%d\n", d.Cfg.DataRate, d.DecCfg.DataRate)
		log.Printf("SymbolLength: %d|%d\n", d.Cfg.SymbolLength, d.DecCfg.SymbolLength)
		log.Println("PreambleSymbols:", d.Cfg.PreambleSymbols)
		log.Printf("PreambleLength: %d|%d\n", d.Cfg.PreambleLength, d.DecCfg.PreambleLength)
		log.Println("PacketSymbols:", d.Cfg.PacketSymbols)
		log.Printf("PacketLength: %d|%d\n", d.Cfg.PacketLength, d.DecCfg.PacketLength)
		log.Println("Preamble:", d.Cfg.Preamble)

		if d.Cfg.SymbolLength%d.Decimation != 0 {
			log.Println("Warning: decimated symbol length is non-integral, sensitivity may be poor")
		}

		if d.DecCfg.SymbolLength < 3 {
			log.Fatal("Error: illegal decimation factor, choose a smaller factor")
		}

		return
	}

	log.Println("CenterFreq:", d.Cfg.CenterFreq)
	log.Println("SampleRate:", d.Cfg.SampleRate)
	log.Println("DataRate:", d.Cfg.DataRate)
	log.Println("SymbolLength:", d.Cfg.SymbolLength)
	log.Println("PreambleSymbols:", d.Cfg.PreambleSymbols)
	log.Println("PreambleLength:", d.Cfg.PreambleLength)
	log.Println("PacketSymbols:", d.Cfg.PacketSymbols)
	log.Println("PacketLength:", d.Cfg.PacketLength)
	log.Println("Preamble:", d.Cfg.Preamble)
}

// Decoder contains buffers and radio configuration.
type Decoder struct {
	Cfg PacketConfig

	Decimation int
	DecCfg     PacketConfig

	IQ        []byte
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
func NewDecoder(cfg PacketConfig, decimation int) (d Decoder) {
	d.Cfg = cfg

	d.Cfg.SymbolLength2 = d.Cfg.SymbolLength << 1
	d.Cfg.SampleRate = d.Cfg.DataRate * d.Cfg.SymbolLength

	d.Cfg.PreambleLength = d.Cfg.PreambleSymbols * d.Cfg.SymbolLength2
	d.Cfg.PacketLength = d.Cfg.PacketSymbols * d.Cfg.SymbolLength2

	d.Cfg.BlockSize = NextPowerOf2(d.Cfg.PreambleLength)
	d.Cfg.BlockSize2 = d.Cfg.BlockSize << 1

	d.Cfg.BufferLength = d.Cfg.PacketLength + d.Cfg.BlockSize

	d.Decimation = decimation
	d.DecCfg = d.Cfg.Decimate(d.Decimation)

	// Allocate necessary buffers.
	d.IQ = make([]byte, d.Cfg.BufferLength<<1)
	d.Signal = make([]float64, d.DecCfg.BufferLength)
	d.Filtered = make([]float64, d.DecCfg.BufferLength)
	d.Quantized = make([]byte, d.DecCfg.BufferLength)

	d.csum = make([]float64, (d.DecCfg.PacketLength - d.DecCfg.SymbolLength2 + 1))

	// Calculate magnitude lookup table specified by -fastmag flag.
	d.demod = NewSqrtMagLUT()

	// Pre-calculate a byte-slice version of the preamble for searching.
	d.preamble = make([]byte, len(d.Cfg.Preamble))
	for idx := range d.Cfg.Preamble {
		if d.Cfg.Preamble[idx] == '1' {
			d.preamble[idx] = 1
		}
	}

	// Slice quantized sample buffer to make searching for the preamble more
	// memory local. Pre-allocate a flat buffer so memory is contiguous and
	// assign slices to the buffer.
	d.slices = make([][]byte, d.DecCfg.SymbolLength2)
	flat := make([]byte, d.DecCfg.BlockSize2-(d.DecCfg.BlockSize2%d.DecCfg.SymbolLength2))

	symbolsPerBlock := d.DecCfg.BlockSize2 / d.DecCfg.SymbolLength2
	for symbolOffset := range d.slices {
		lower := symbolOffset * symbolsPerBlock
		upper := (symbolOffset + 1) * symbolsPerBlock
		d.slices[symbolOffset] = flat[lower:upper]
	}

	d.preambleFinder = makeByteFinder(d.preamble)

	// Signal up to the final stage is 1-bit per byte. Allocate a buffer to
	// store packed version 8-bits per byte.
	d.pkt = make([]byte, (d.DecCfg.PacketSymbols+7)>>3)

	return
}

// Decode accepts a sample block and performs various DSP techniques to extract a packet.
func (d Decoder) Decode(input []byte) []int {
	// Shift buffers to append new block.
	copy(d.IQ, d.IQ[d.Cfg.BlockSize<<1:])
	copy(d.Signal, d.Signal[d.DecCfg.BlockSize:])
	copy(d.Filtered, d.Filtered[d.DecCfg.BlockSize:])
	copy(d.Quantized, d.Quantized[d.DecCfg.BlockSize:])
	copy(d.IQ[d.Cfg.PacketLength<<1:], input[:])

	iqBlock := d.IQ[d.Cfg.PacketLength<<1:]
	signalBlock := d.Signal[d.DecCfg.PacketLength:]

	// Compute the magnitude of the new block.
	d.demod.Execute(iqBlock, signalBlock)

	signalBlock = d.Signal[d.DecCfg.PacketLength-d.DecCfg.SymbolLength2:]
	filterBlock := d.Filtered[d.DecCfg.PacketLength-d.DecCfg.SymbolLength2:]

	// Perform matched filter on new block.
	d.Filter(signalBlock, filterBlock)

	// Perform bit-decision on new block.
	Quantize(filterBlock, d.Quantized[d.DecCfg.PacketLength-d.DecCfg.SymbolLength2:])

	// Pack the quantized signal into slices for searching.
	d.Pack(d.Quantized[:d.DecCfg.BlockSize2])

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
func NewSqrtMagLUT() (lut MagLUT) {
	lut = make([]float64, 0x100)
	for idx := range lut {
		lut[idx] = 127.4 - float64(idx)
		lut[idx] *= lut[idx]
	}
	return
}

// Calculates complex magnitude on given IQ stream writing result to output.
func (lut MagLUT) Execute(input []byte, output []float64) {
	decIdx := 0
	dec := (len(input) / len(output))

	for idx := 0; decIdx < len(output); idx += dec {
		output[decIdx] = math.Sqrt(lut[input[idx]] + lut[input[idx+1]])
		decIdx++
	}
}

// Matched filter for Manchester coded signals. Output signal's sign at each
// sample determines the bit-value since Manchester symbols have odd symmetry.
func (d Decoder) Filter(input, output []float64) {
	// Computing the cumulative summation over the signal simplifies
	// filtering to the difference of a pair of subtractions.
	var sum float64
	for idx, v := range input {
		sum += v
		d.csum[idx+1] = sum
	}

	// Filter result is difference of summation of lower and upper symbols.
	lower := d.csum[d.DecCfg.SymbolLength:]
	upper := d.csum[d.DecCfg.SymbolLength2:]
	n := len(input) - d.DecCfg.SymbolLength2
	for idx := 0; idx < n; idx++ {
		output[idx] = (lower[idx] - d.csum[idx]) - (upper[idx] - lower[idx])
	}

	return
}

// Bit-value is determined by the sign of each sample after filtering.
func Quantize(input []float64, output []byte) {
	for idx, val := range input {
		output[idx] = byte(math.Float64bits(val)>>63) ^ 0x01
	}

	return
}

// Packs quantized signal into slices such that the first rank represents
// sample offsets and the second represents the value of each symbol from the
// given offset.
//
// Transforms:
// <--Sym1--><--Sym2--><--Sym3--><--Sym4--><--Sym5--><--Sym6--><--Sym7--><--Sym8-->
// <12345678><12345678><12345678><12345678><12345678><12345678><12345678><12345678>
// to:
// <11111111><22222222><33333333><44444444><55555555><66666666><77777777><88888888>
func (d *Decoder) Pack(input []byte) {
	for symbolOffset, slice := range d.slices {
		for symbolIdx := range slice {
			slice[symbolIdx] = input[symbolIdx*d.DecCfg.SymbolLength2+symbolOffset]
		}
	}

	return
}

// For each sample offset look for the preamble. Return a list of indexes the
// preamble is found at. Indexes are absolute in the unsliced quantized
// buffer.
func (d *Decoder) Search() (indexes []int) {
	for symbolOffset, slice := range d.slices {
		offset := 0
		idx := 0
		for {
			idx = d.preambleFinder.next(slice[offset:])
			if idx != -1 {
				indexes = append(indexes, (offset+idx)*d.DecCfg.SymbolLength2+symbolOffset)
				offset += idx + 1
			} else {
				break
			}
		}
	}

	return
}

// Given a list of indeces the preamble exists at, sample the appropriate bits
// of the signal's bit-decision. Pack bits of each index into an array of byte
// arrays and return.
func (d Decoder) Slice(indices []int) (pkts [][]byte) {
	// We will likely find multiple instances of the message so only keep
	// track of unique instances.
	seen := make(map[string]bool)

	// For each of the indices the preamble exists at.
	for _, qIdx := range indices {
		// Check that we're still within the first sample block. We'll catch
		// the message on the next sample block otherwise.
		if qIdx > d.DecCfg.BlockSize {
			continue
		}

		// Packet is 1 bit per byte, pack to 8-bits per byte.
		for pIdx := 0; pIdx < d.DecCfg.PacketSymbols; pIdx++ {
			d.pkt[pIdx>>3] <<= 1
			d.pkt[pIdx>>3] |= d.Quantized[qIdx+(pIdx*d.DecCfg.SymbolLength2)]
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

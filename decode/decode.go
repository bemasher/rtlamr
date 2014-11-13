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

package decode

import (
	"fmt"
	"log"
	"math"

	"github.com/bemasher/rtlamr/decode/dft"
)

const (
	ChannelWidth = 196568
)

// PacketConfig specifies packet-specific radio configuration.
type PacketConfig struct {
	DataRate                    int
	BlockSize, BlockSize2       int
	SymbolLength, SymbolLength2 int
	SampleRate                  int

	PreambleSymbols, PacketSymbols int
	PreambleLength, PacketLength   int
	BufferLength                   int
	Preamble                       string
}

func (cfg PacketConfig) Log() {
	log.Println("BlockSize:", cfg.BlockSize)
	log.Println("SampleRate:", cfg.SampleRate)
	log.Println("DataRate:", cfg.DataRate)
	log.Println("SymbolLength:", cfg.SymbolLength)
	log.Println("PreambleSymbols:", cfg.PreambleSymbols)
	log.Println("PreambleLength:", cfg.PreambleLength)
	log.Println("PacketSymbols:", cfg.PacketSymbols)
	log.Println("PacketLength:", cfg.PacketLength)
	log.Println("Channels:", cfg.SampleRate/ChannelWidth)
	log.Println("ExcessBandwidth:", cfg.SampleRate%ChannelWidth)
	log.Println("Preamble:", cfg.Preamble)
}

// Decoder contains buffers and radio configuration.
type Decoder struct {
	Cfg PacketConfig

	IQ        []byte
	Signal    []float64
	Quantized []byte

	Periodogram Periodogram

	Re, Im []float64

	csum []float64
	lut  MagnitudeLUT

	preamble []byte
	slices   [][]byte

	pkt []byte
}

// Create a new decoder with the given packet configuration.
func NewDecoder(cfg PacketConfig, fastMag bool) (d Decoder) {
	d.Cfg = cfg

	// Allocate necessary buffers.
	d.IQ = make([]byte, d.Cfg.BufferLength<<1)
	d.Signal = make([]float64, d.Cfg.BufferLength)
	d.Quantized = make([]byte, d.Cfg.BufferLength)

	d.Re = make([]float64, d.Cfg.BufferLength)
	d.Im = make([]float64, d.Cfg.BufferLength)

	d.csum = make([]float64, d.Cfg.BlockSize+d.Cfg.SymbolLength2+1)

	d.Periodogram = NewPeriodogram(d.Cfg.SampleRate / ChannelWidth)

	// Calculate magnitude lookup table specified by -fastmag flag.
	if fastMag {
		d.lut = NewAlphaMaxBetaMinLUT()
	} else {
		d.lut = NewSqrtMagLUT()
	}

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
	d.slices = make([][]byte, d.Cfg.SymbolLength2)
	flat := make([]byte, d.Cfg.BlockSize2-(d.Cfg.BlockSize2%d.Cfg.SymbolLength2))

	for symbolOffset := range d.slices {
		lower := symbolOffset * (d.Cfg.BlockSize2 / d.Cfg.SymbolLength2)
		upper := (symbolOffset + 1) * (d.Cfg.BlockSize2 / d.Cfg.SymbolLength2)
		d.slices[symbolOffset] = flat[lower:upper]
	}

	// Signal up to the final stage is 1-bit per byte. Allocate a buffer to
	// store packed version 8-bits per byte.
	d.pkt = make([]byte, d.Cfg.PacketSymbols>>3)

	return
}

// Decode accepts a sample block and performs various DSP techniques to extract a packet.
func (d Decoder) Decode(input []byte) (pkts [][]byte) {
	// Shift buffers to append new block.
	copy(d.IQ, d.IQ[d.Cfg.BlockSize<<1:])
	copy(d.Signal, d.Signal[d.Cfg.BlockSize:])
	copy(d.Quantized, d.Quantized[d.Cfg.BlockSize:])
	copy(d.IQ[d.Cfg.PacketLength<<1:], input[:])

	iqBlock := d.IQ[d.Cfg.PacketLength<<1:]
	signalBlock := d.Signal[d.Cfg.PacketLength:]

	re := d.Re[d.Cfg.PacketLength:]
	im := d.Im[d.Cfg.PacketLength:]
	for idx := range signalBlock {
		re[idx] = 127.4 - float64(d.IQ[idx<<1])
		im[idx] = 127.4 - float64(d.IQ[idx<<1+1])
	}

	// Compute the magnitude of the new block.
	d.lut.Execute(iqBlock, signalBlock)

	signalBlock = d.Signal[d.Cfg.PacketLength-d.Cfg.SymbolLength2:]

	// Perform matched filter on new block.
	d.Filter(signalBlock)
	signalBlock = d.Signal[d.Cfg.PacketLength-d.Cfg.SymbolLength2:]

	// Perform bit-decision on new block.
	Quantize(signalBlock, d.Quantized[d.Cfg.PacketLength-d.Cfg.SymbolLength2:])

	// Pack the quantized signal into slices for searching.
	d.Pack(d.Quantized[:d.Cfg.BlockSize2], d.slices)

	// Get a list of indexes the preamble exists at.
	indexes := d.Search(d.slices, d.preamble)

	// We will likely find multiple instances of the message so only keep
	// track of unique instances.
	seen := make(map[string]bool)

	// For each of the indexes the preamble exists at.
	for _, qIdx := range indexes {
		// Check that we're still within the first sample block. We'll catch
		// the message on the next sample block otherwise.
		if qIdx > d.Cfg.BlockSize {
			continue
		}

		// Packet is 1 bit per byte, pack to 8-bits per byte.
		for pIdx := 0; pIdx < d.Cfg.PacketSymbols; pIdx++ {
			d.pkt[pIdx>>3] <<= 1
			d.pkt[pIdx>>3] |= d.Quantized[qIdx+(pIdx*d.Cfg.SymbolLength2)]
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

func (d Decoder) Reset() {
	for idx := range d.IQ {
		d.IQ[idx] = 0
	}
	for idx := range d.Signal {
		d.Signal[idx] = 0
		d.Quantized[idx] = 0
		d.Re[idx] = 0
		d.Im[idx] = 0
	}
}

type Periodogram struct {
	length int
	power  []float64
	re, im []float64
	dft    func(ri, ii, ro, io []float64)
}

func NewPeriodogram(n int) (p Periodogram) {
	p.length = n
	p.power = make([]float64, p.length)
	p.re = make([]float64, p.length)
	p.im = make([]float64, p.length)

	switch p.length {
	case 5:
		p.dft = dft.DFT5
	case 6:
		p.dft = dft.DFT6
	case 7:
		p.dft = dft.DFT7
	case 8:
		p.dft = dft.DFT8
	case 9:
		p.dft = dft.DFT9
	case 10:
		p.dft = dft.DFT10
	case 11:
		p.dft = dft.DFT11
	case 12:
		p.dft = dft.DFT12
	case 13:
		p.dft = dft.DFT13
	case 14:
		p.dft = dft.DFT14
	case 15:
		p.dft = dft.DFT15
	case 16:
		p.dft = dft.DFT16
	default:
		panic(fmt.Errorf("invalid transform length: %d", p.length))
	}

	return
}

func (p Periodogram) Execute(re, im []float64) int {
	for idx := range p.power {
		p.power[idx] = 0
	}

	for idx := 0; idx < len(re)-p.length; idx += p.length >> 1 {
		p.dft(re[idx:], im[idx:], p.re, p.im)
		for pIdx := range p.power {
			p.power[pIdx] += math.Sqrt(p.re[pIdx]*p.re[pIdx] + p.im[pIdx]*p.im[pIdx])
		}
	}

	max := 0.0
	argmax := 0
	for idx, val := range p.power {
		if max < val {
			max = val
			argmax = idx
		}
	}
	// return (argmax - p.length>>1) + p.length&1
	if argmax > (p.length>>1 + p.length&1) {
		return argmax - p.length
	}
	return argmax
}

// A MagnitudeLUT knows how to perform complex magnitude on a slice of IQ samples.
type MagnitudeLUT interface {
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
	for idx := range output {
		lutIdx := idx << 1
		output[idx] = math.Sqrt(lut[input[lutIdx]] + lut[input[lutIdx+1]])
	}
}

// Alpha*Max + Beta*Min Magnitude Approximation Lookup Table.
type AlphaMaxBetaMinLUT []float64

// Pre-computes absolute values with most common DC offset for rtl-sdr dongles.
func NewAlphaMaxBetaMinLUT() (lut AlphaMaxBetaMinLUT) {
	lut = make([]float64, 0x100)
	for idx := range lut {
		lut[idx] = math.Abs(127.4 - float64(idx))
	}
	return
}

// Calculates complex magnitude on given IQ stream writing result to output.
func (lut AlphaMaxBetaMinLUT) Execute(input []byte, output []float64) {
	const (
		α = 0.948059448969
		ß = 0.392699081699
	)

	for idx := range output {
		lutIdx := idx << 1
		i := lut[input[lutIdx]]
		q := lut[input[lutIdx+1]]
		if i > q {
			output[idx] = α*i + ß*q
		} else {
			output[idx] = α*q + ß*i
		}
	}
}

// Matched filter for Manchester coded signals. Output signal's sign at each
// sample determines the bit-value since Manchester symbols have odd symmetry.
func (d Decoder) Filter(input []float64) {
	// Computing the cumulative summation over the signal simplifies
	// filtering to the difference of a pair of subtractions.
	var sum float64
	for idx, v := range input {
		sum += v
		d.csum[idx+1] = sum
	}

	// Filter result is difference of summation of lower and upper symbols.
	lower := d.csum[d.Cfg.SymbolLength:]
	upper := d.csum[d.Cfg.SymbolLength2:]
	for idx := range input[:len(input)-d.Cfg.SymbolLength2] {
		input[idx] = (lower[idx] - d.csum[idx]) - (upper[idx] - lower[idx])
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
func (d Decoder) Pack(input []byte, slices [][]byte) {
	for symbolOffset, slice := range slices {
		for symbolIdx := range slice {
			slice[symbolIdx] = input[symbolIdx*d.Cfg.SymbolLength2+symbolOffset]
		}
	}

	return
}

// For each sample offset look for the preamble. Return a list of indexes the
// preamble is found at. Indexes are absolute in the unsliced quantized
// buffer.
func (d Decoder) Search(slices [][]byte, preamble []byte) (indexes []int) {
	for symbolOffset, slice := range slices {
		for symbolIdx := range slice[:len(slice)-len(preamble)] {
			var result uint8
			for bitIdx, bit := range preamble {
				result |= bit ^ slice[symbolIdx+bitIdx]
				if result != 0 {
					break
				}
			}
			if result == 0 {
				indexes = append(indexes, symbolIdx*d.Cfg.SymbolLength2+symbolOffset)
			}
		}
	}

	return
}

func NextPowerOf2(v int) int {
	return 1 << uint(math.Ceil(math.Log2(float64(v))))
}

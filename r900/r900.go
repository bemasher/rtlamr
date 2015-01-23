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

package r900

import (
	"bytes"
	"fmt"
	"math"
	"strconv"

	"github.com/bemasher/gf"
	"github.com/bemasher/rtlamr/decode"
	"github.com/bemasher/rtlamr/parse"
)

const (
	PayloadSymbols = 42
)

func NewPacketConfig(symbolLength int) (cfg decode.PacketConfig) {
	cfg.DataRate = 32768

	cfg.CenterFreq = 912380000

	cfg.SymbolLength = symbolLength
	cfg.SymbolLength2 = cfg.SymbolLength << 1

	cfg.SampleRate = cfg.DataRate * cfg.SymbolLength

	cfg.PreambleSymbols = 32
	cfg.PacketSymbols = 116

	cfg.PreambleLength = cfg.PreambleSymbols * cfg.SymbolLength2
	cfg.PacketLength = cfg.PacketSymbols * cfg.SymbolLength2

	cfg.BlockSize = decode.NextPowerOf2(cfg.PreambleLength)
	cfg.BlockSize2 = cfg.BlockSize << 1

	cfg.BufferLength = cfg.PacketLength + cfg.BlockSize

	cfg.Preamble = "00000000000000001110010101100100"

	return
}

type Parser struct {
	decode.Decoder
	field *gf.Field
	rsBuf [31]byte

	csum      []float64
	filtered  [][3]float64
	quantized []byte
}

func NewParser(symbolLength int, fastMag bool) (p Parser) {
	p.Decoder = decode.NewDecoder(NewPacketConfig(symbolLength), fastMag)

	// GF of order 32, polynomial 37, generator 2.
	p.field = gf.NewField(32, 37, 2)

	p.csum = make([]float64, p.Decoder.Cfg.BufferLength+1)
	p.filtered = make([][3]float64, p.Decoder.Cfg.BufferLength)
	p.quantized = make([]byte, p.Decoder.Cfg.BufferLength)

	return
}

func (p Parser) Dec() decode.Decoder {
	return p.Decoder
}

func (p Parser) Cfg() decode.PacketConfig {
	return p.Decoder.Cfg
}

// Perform matched filtering.
func (p Parser) Filter() {
	// This function computes the convolution of each symbol kernel with the
	// signal. The naive approach requires for each symbol to calculate the
	// summation of samples between a pair of indices.

	// 0 |--------|
	// 1   |--------|
	// 2     |--------|
	// 3       |--------|

	// To avoid redundant calculations we compute the cumulative sum of the
	// signal. This reduces each summation to the difference between the two
	// indices of the cumulative sum.

	var sum float64
	for idx, v := range p.Decoder.Signal {
		sum += v
		p.csum[idx+1] = sum
	}

	// There are six symbols, composed of three base symbols and their bitwise
	// inversions. Compute the convolution of each base symbol with the
	// signal.

	// 1100 -> 0011
	// 1010 -> 0101
	// 1001 -> 0110

	// This is basically unreadable because of a lot of algebraic
	// simplification but is necessary for efficiency.
	for idx := 0; idx < p.Decoder.Cfg.BufferLength-p.Decoder.Cfg.SymbolLength*4; idx++ {
		c0 := p.csum[idx]
		c1 := p.csum[idx+p.Decoder.Cfg.SymbolLength] * 2
		c2 := p.csum[idx+p.Decoder.Cfg.SymbolLength*2] * 2
		c3 := p.csum[idx+p.Decoder.Cfg.SymbolLength*3] * 2
		c4 := p.csum[idx+p.Decoder.Cfg.SymbolLength*4]

		p.filtered[idx][0] = c2 - c4 - c0           // 1100
		p.filtered[idx][1] = c1 - c2 + c3 - c4 - c0 // 1010
		p.filtered[idx][2] = c1 - c3 + c4 - c0      // 1001
	}
}

// Determine the symbol that exists at each sample of the signal.
func (p Parser) Quantize() {
	// 0 0011, 3 1100
	// 1 0101, 4 1010
	// 2 0110, 5 1001

	for idx, vec := range p.filtered {
		argmax := byte(0)
		max := math.Abs(vec[0])

		// If v1 is larger than v0, update max and argmax.
		if v1 := math.Abs(vec[1]); v1 > max {
			max = v1
			argmax = 1
		}

		// If v2 is larger than the greater of v1 or v0, update max and argmax.
		if v2 := math.Abs(vec[2]); v2 > max {
			max = v2
			argmax = 2
		}

		// Set the output symbol index.
		p.quantized[idx] = argmax

		// If the sign is negative, jump to the index of the inverted symbol.
		if vec[argmax] > 0 {
			p.quantized[idx] += 3
		}
	}
}

// Given a list of indices the preamble exists at, decode and parse a message.
func (p Parser) Parse(indices []int) (msgs []parse.Message) {
	p.Filter()
	p.Quantize()

	preambleLength := p.Decoder.Cfg.PreambleLength
	symbolLength := p.Decoder.Cfg.SymbolLength

	symbols := make([]byte, 21)
	zeros := make([]byte, 5)

	seen := make(map[string]bool)

	for _, preambleIdx := range indices {
		if preambleIdx > p.Decoder.Cfg.BlockSize {
			break
		}

		payloadIdx := preambleIdx + preambleLength
		var digits string
		for idx := 0; idx < PayloadSymbols*4*p.Decoder.Cfg.SymbolLength; idx += symbolLength * 4 {
			qIdx := payloadIdx + idx

			digits += strconv.Itoa(int(p.quantized[qIdx]))
		}

		var (
			bits      string
			badSymbol bool
		)
		for idx := 0; idx < len(digits); idx += 2 {
			symbol, _ := strconv.ParseInt(digits[idx:idx+2], 6, 32)
			if symbol > 31 {
				badSymbol = true
				break
			}
			symbols[idx>>1] = byte(symbol)
			bits += fmt.Sprintf("%05b", symbol)
		}

		if badSymbol || seen[bits] {
			continue
		}

		seen[bits] = true

		copy(p.rsBuf[:], symbols[:16])
		copy(p.rsBuf[26:], symbols[16:])
		syndromes := p.field.Syndrome(p.rsBuf[:], 5, 29)

		if !bytes.Equal(zeros, syndromes) {
			continue
		}

		id, _ := strconv.ParseUint(bits[:32], 2, 32)
		unkn1, _ := strconv.ParseUint(bits[32:40], 2, 8)
		unkn2, _ := strconv.ParseUint(bits[40:48], 2, 8)
		consumption, _ := strconv.ParseUint(bits[48:72], 2, 24)
		unkn3, _ := strconv.ParseUint(bits[72:74], 2, 2)
		unkn4, _ := strconv.ParseUint(bits[74:80], 2, 6)

		var r900 R900

		r900.ID = uint32(id)
		r900.Unkn1 = uint8(unkn1)
		r900.Unkn2 = uint8(unkn2)
		r900.Consumption = uint32(consumption)
		r900.Unkn3 = uint8(unkn3)
		r900.Unkn4 = uint8(unkn4)

		msgs = append(msgs, r900)
	}

	return
}

type R900 struct {
	ID          uint32 `xml:",attr"` // 32 bits
	Unkn1       uint8  `xml:",attr"` // 8 bits
	Unkn2       uint8  `xml:",attr"` // 8 bits
	Consumption uint32 `xml:",attr"` // 24 bits
	Unkn3       uint8  `xml:",attr"` // 2 bits
	Unkn4       uint8  `xml:",attr"` // 6 bits
}

func (r900 R900) MsgType() string {
	return "R900"
}

func (r900 R900) MeterID() uint32 {
	return r900.ID
}

func (r900 R900) MeterType() uint8 {
	return r900.Unkn1
}

func (r900 R900) String() string {
	return fmt.Sprintf("{ID:%10d Unkn1:0x%02X Unkn2:0x%02X Consumption:%8d Unkn3:0x%02X Unkn4:0x%02X}",
		r900.ID,
		r900.Unkn1,
		r900.Unkn2,
		r900.Consumption,
		r900.Unkn3,
		r900.Unkn4,
	)
}

func (r900 R900) Record() (r []string) {
	r = append(r, strconv.FormatUint(uint64(r900.ID), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Unkn1), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Unkn2), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Consumption), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Unkn3), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Unkn4), 10))

	return
}

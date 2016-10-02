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

	"github.com/bemasher/rtlamr/decode"
	"github.com/bemasher/rtlamr/parse"
	"github.com/bemasher/rtlamr/r900/gf"
)

const (
	PayloadSymbols = 42
)

func init() {
	parse.Register("r900", NewParser)
}

func NewPacketConfig(chipLength int) (cfg decode.PacketConfig) {
	cfg.CenterFreq = 912380000
	cfg.DataRate = 32768
	cfg.ChipLength = chipLength
	cfg.PreambleSymbols = 32
	cfg.PacketSymbols = 116
	cfg.Preamble = "00000000000000001110010101100100"

	return
}

type Parser struct {
	decode.Decoder
	field *gf.Field
	rsBuf [31]byte

	signal    []float64
	csum      []float64
	filtered  [][3]float64
	quantized []byte
}

func NewParser(chipLength, decimation int) parse.Parser {
	p := new(Parser)

	p.Decoder = decode.NewDecoder(NewPacketConfig(chipLength), decimation)

	// GF of order 32, polynomial 37, generator 2.
	p.field = gf.NewField(32, 37, 2)

	p.signal = make([]float64, p.Decoder.DecCfg.BufferLength)
	p.csum = make([]float64, p.Decoder.DecCfg.BufferLength+1)
	p.filtered = make([][3]float64, p.Decoder.DecCfg.BufferLength)
	p.quantized = make([]byte, p.Decoder.DecCfg.BufferLength)

	return p
}

func (p Parser) Dec() decode.Decoder {
	return p.Decoder
}

func (p *Parser) Cfg() *decode.PacketConfig {
	return &p.Decoder.Cfg
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
	for idx, v := range p.signal {
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

	cfg := p.Decoder.DecCfg
	for idx := 0; idx < cfg.BufferLength-cfg.ChipLength*4; idx++ {
		c0 := p.csum[idx]
		c1 := p.csum[idx+cfg.ChipLength] * 2
		c2 := p.csum[idx+cfg.ChipLength*2] * 2
		c3 := p.csum[idx+cfg.ChipLength*3] * 2
		c4 := p.csum[idx+cfg.ChipLength*4]

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
	cfg := p.Decoder.DecCfg
	copy(p.signal, p.signal[cfg.BlockSize:])
	copy(p.signal[cfg.PacketLength:], p.Decoder.Signal[cfg.SymbolLength:])

	p.Filter()
	p.Quantize()

	preambleLength := cfg.PreambleLength
	chipLength := cfg.ChipLength

	symbols := make([]byte, 21)
	zeros := make([]byte, 5)

	seen := make(map[string]bool)

	for _, preambleIdx := range indices {
		if preambleIdx > cfg.BlockSize {
			break
		}

		payloadIdx := preambleIdx + preambleLength - p.Dec().DecCfg.SymbolLength
		var digits string
		for idx := 0; idx < PayloadSymbols*4*cfg.ChipLength; idx += chipLength * 4 {
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
		nouse, _ := strconv.ParseUint(bits[40:46], 2, 6)
		backflow, _ := strconv.ParseUint(bits[46:48], 2, 2)
		consumption, _ := strconv.ParseUint(bits[48:72], 2, 24)
		unkn3, _ := strconv.ParseUint(bits[72:74], 2, 2)
		leak, _ := strconv.ParseUint(bits[74:78], 2, 4)
		leaknow, _ := strconv.ParseUint(bits[78:80], 2, 2)

		var r900 R900

		r900.ID = uint32(id)
		r900.Unkn1 = uint8(unkn1)
		r900.NoUse = uint8(nouse)
		r900.BackFlow = uint8(backflow)
		r900.Consumption = uint32(consumption)
		r900.Unkn3 = uint8(unkn3)
		r900.Leak = uint8(leak)
		r900.LeakNow = uint8(leaknow)
		copy(r900.checksum[:], symbols[16:])

		msgs = append(msgs, r900)
	}

	return
}

type R900 struct {
	ID          uint32 `xml:",attr"` // 32 bits
	Unkn1       uint8  `xml:",attr"` // 8 bits
	NoUse       uint8  `xml:",attr"` // 6 bits, day bins of no use
	BackFlow    uint8  `xml:",attr"` // 2 bits, backflow past 35d hi/lo
	Consumption uint32 `xml:",attr"` // 24 bits
	Unkn3       uint8  `xml:",attr"` // 2 bits
	Leak        uint8  `xml:",attr"` // 4 bits, day bins of leak
	LeakNow     uint8  `xml:",attr"` // 2 bits, leak past 24h hi/lo
	checksum    [5]byte
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

func (r900 R900) Checksum() []byte {
	return r900.checksum[:]
}

func (r900 R900) String() string {
	return fmt.Sprintf("{ID:%10d Unkn1:0x%02X NoUse:%2d BackFlow:%1d Consumption:%8d Unkn3:0x%02X Leak:%2d LeakNow:%1d}",
		r900.ID,
		r900.Unkn1,
		r900.NoUse,
		r900.BackFlow,
		r900.Consumption,
		r900.Unkn3,
		r900.Leak,
		r900.LeakNow,
	)
}

func (r900 R900) Record() (r []string) {
	r = append(r, strconv.FormatUint(uint64(r900.ID), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Unkn1), 10))
	r = append(r, strconv.FormatUint(uint64(r900.NoUse), 10))
	r = append(r, strconv.FormatUint(uint64(r900.BackFlow), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Consumption), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Unkn3), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Leak), 10))
	r = append(r, strconv.FormatUint(uint64(r900.LeakNow), 10))

	return
}

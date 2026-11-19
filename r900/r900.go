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
	"strconv"
	"sync"

	"github.com/bemasher/rtlamr/protocol"
	"github.com/bemasher/rtlamr/r900/gf"
)

const (
	PayloadSymbols = 42
)

func init() {
	protocol.RegisterParser("r900", NewParser)
}

func NewPacketConfig(chipLength int) (cfg protocol.PacketConfig) {
	return
}

// GF of order 32, polynomial 37, generator 2.
var field = gf.NewField(32, 37, 2)

// Generator polynomial for the five roots a^29..a^33 that Syndrome checks.
var gen = generator(29, 5)

type Parser struct {
	*protocol.Decoder
	cfg   protocol.PacketConfig
	rsBuf [31]byte

	signal    []float32
	csum      []float32
	quantized []byte

	once sync.Once
}

func NewParser(chipLength int) protocol.Parser {
	var p Parser

	p.cfg = protocol.PacketConfig{
		Protocol:        "r900",
		CenterFreq:      912380000,
		DataRate:        32768,
		ChipLength:      chipLength,
		PreambleSymbols: 32,
		PacketSymbols:   116,
		Preamble:        "00000000000000001110010101100100",
	}

	return &p
}

func (p *Parser) SetDecoder(d *protocol.Decoder) {
	p.Decoder = d
}

func (p *Parser) Cfg() protocol.PacketConfig {
	return p.cfg
}

// Perform matched filtering.
func (p *Parser) filter() {
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

	var sum float32
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

	cfg := p.Decoder.Cfg
	cl := cfg.ChipLength
	cl2 := cl * 2
	cl3 := cl * 3
	cl4 := cl * 4

	limit := cfg.BufferLength - cl4

	for idx := 0; idx < limit; idx++ {
		c0 := p.csum[idx]
		c1 := p.csum[idx+cl] + p.csum[idx+cl]
		c2 := p.csum[idx+cl2] + p.csum[idx+cl2]
		c3 := p.csum[idx+cl3] + p.csum[idx+cl3]
		c4 := p.csum[idx+cl4]

		a0 := c2 - c4 - c0           // 1100
		a1 := c1 - c2 + c3 - c4 - c0 // 1010
		a2 := c1 - c3 + c4 - c0      // 1001

		// Quantize in the same loop
		maxAbs := abs(a0)
		argmax := byte(0)
		if abs(a1) > maxAbs {

			maxAbs = abs(a1)
			argmax = 1
		}
		if abs(a2) > maxAbs {
			maxAbs = abs(a2)
			argmax = 2
		}

		p.quantized[idx] = argmax
		if [3]float32{a0, a1, a2}[argmax] > 0 {
			p.quantized[idx] += 3
		}
	}
}

func abs[F float32 | float64](x F) F {
	if x < 0 {
		return -x
	}
	return x
}

// Given a list of indices the preamble exists at, decode and parse a message.
func (p *Parser) Parse(pkts []protocol.Data, msgCh chan protocol.Message, wg *sync.WaitGroup) {
	p.once.Do(func() {
		p.cfg = p.Decoder.Cfg
		p.signal = make([]float32, p.Decoder.Cfg.BufferLength)
		p.csum = make([]float32, p.Decoder.Cfg.BufferLength+1)
		p.quantized = make([]byte, p.Decoder.Cfg.BufferLength)
	})

	cfg := p.cfg
	copy(p.signal, p.signal[cfg.BlockSize:])
	copy(p.signal[cfg.PacketLength:], p.Decoder.Signal[cfg.SymbolLength:])

	p.filter()

	preambleLength := cfg.PreambleLength
	chipLength := cfg.ChipLength

	symbols := make([]byte, 21)
	zeros := make([]byte, 5)

	seen := make(map[string]bool)

	for _, pkt := range pkts {
		if pkt.Idx > cfg.BlockSize {
			break
		}

		// Decoder.Filter delays the quantized signal by SymbolLength, which
		// pkt.Idx carries, so the payload starts that much earlier here.
		payloadIdx := pkt.Idx + preambleLength - p.cfg.SymbolLength
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
		syndrome := field.Syndrome(p.rsBuf[:], 5, 29)

		if !bytes.Equal(zeros, syndrome) {
			continue
		}

		id, _ := strconv.ParseUint(bits[:32], 2, 32)
		unkn1, _ := strconv.ParseUint(bits[32:36], 2, 4)
                metertype, _ := strconv.ParseUint(bits[36:40], 2, 4)
                unkn2, _ := strconv.ParseUint(bits[40:43], 2, 3)
		nouse, _ := strconv.ParseUint(bits[43:46], 2, 3)
		backflow, _ := strconv.ParseUint(bits[46:48], 2, 2)
		consumption, _ := strconv.ParseUint(bits[72:75] + bits[48:72], 2, 27)
		leak, _ := strconv.ParseUint(bits[75:78], 2, 3)
		leaknow, _ := strconv.ParseUint(bits[78:80], 2, 2)

		var r900 R900

		r900.ID = uint32(id)
		r900.Unkn1 = uint8(unkn1)
		r900.AmrType = uint8(metertype)
		r900.Unkn2 = uint8(unkn2)
		r900.NoUse = uint8(nouse)
		r900.BackFlow = uint8(backflow)
		r900.Consumption = uint32(consumption)
		r900.Leak = uint8(leak)
		r900.LeakNow = uint8(leaknow)
		copy(r900.checksum[:], symbols[16:])

		msgCh <- r900
	}

	wg.Done()
}

type R900 struct {
	ID          uint32 `xml:",attr"` // 32 bits
	Unkn1       uint8  `xml:",attr"` // 4 bits
        AmrType     uint8  `xml:",attr"` // 4 bits
        Unkn2       uint8  `xml:",attr"` // 3 bits
	NoUse       uint8  `xml:",attr"` // 3 bits, day bins of no use
	BackFlow    uint8  `xml:",attr"` // 2 bits, backflow past 35d hi/lo
	Consumption uint32 `xml:",attr"` // 27 bits
	Leak        uint8  `xml:",attr"` // 3 bits, day bins of leak
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
	return r900.AmrType
}

func (r900 R900) Checksum() []byte {
	return r900.checksum[:]
}

// Chips sent for each 6-ary symbol digit: 0011, 0101, 0110, then the base
// patterns 1100, 1010, 1001 that filter correlates, which add 3 to the digit.
var chips = [6]byte{0x03, 0x05, 0x06, 0x0C, 0x0A, 0x09}

// SymMap maps R900 bytes to symbols, each symbol is two 6-ary digits.
func (r900 R900) SymMap() []byte {
	data := [2]uint64{
		uint64(r900.ID)<<8 |
			uint64(r900.Unkn1),
		uint64(r900.NoUse)<<34 |
			uint64(r900.BackFlow)<<32 |
			uint64(r900.Consumption)<<8 |
			uint64(r900.Unkn3)<<6 |
			uint64(r900.Leak)<<2 |
			uint64(r900.LeakNow),
	}

	phy := make([]byte, 21)
	for i := 0; i < 16; i++ {
		s := data[i>>3] >> (35 - 5*(i&7)) & 31
		phy[i] = chips[s/6]<<4 | chips[s%6]
	}
	for i, s := range r900.checksum {
		phy[16+i] = chips[s/6]<<4 | chips[s%6]
	}

	return phy
}

// digit returns the 6-ary digit whose chips are chip.
func digit(chip byte) (byte, bool) {
	for d, c := range chips {
		if c == chip {
			return byte(d), true
		}
	}
	return 0, false
}

// generator returns the coefficients of the product of (x + a^i) for i in
// [offset, offset+n), highest degree first.
func generator(offset, n int) []byte {
	g := []byte{1}
	for i := 0; i < n; i++ {
		r := field.Exp(offset + i)
		next := make([]byte, len(g)+1)
		for j, c := range g {
			next[j] ^= c
			next[j+1] ^= field.Mul(c, r)
		}
		g = next
	}
	return g
}

// GenCRC computes the Reed-Solomon parity bytes.
func (r900 *R900) GenCRC() (crc [5]byte) {
	sym := r900.SymMap()

	if len(sym) != 21 {
		panic("r900: SymCRC: want 21 symbols, got " + strconv.Itoa(len(sym)))
	}

	var code [31]byte // 16 data, 10 zeros, 5 parity
	for i, b := range sym[:16] {
		hi, okHi := digit(b >> 4)
		lo, okLo := digit(b & 0x0F)
		if !okHi || !okLo {
			panic("r900: SymCRC: symbol is not two chips")
		}
		code[i] = hi*6 + lo
	}

	// Remainder of the data times x^5 divided by the generator is the parity.
	for i := 0; i < 26; i++ {
		c := code[i]
		for j, g := range gen {
			code[i+j] ^= field.Mul(g, c)
		}
	}

	copy(crc[:], code[26:])
	return crc
}

func (r900 R900) String() string {
	return fmt.Sprintf("{ID:%10d Unkn1:0x%1X MeterType:%02d Unkn2:0x%1X NoUse:%1d BackFlow:%1d Consumption:%8d Leak:%1d LeakNow:%1d}",
		r900.ID,
		r900.Unkn1,
		r900.AmrType,
		r900.Unkn2,
		r900.NoUse,
		r900.BackFlow,
		r900.Consumption,
		r900.Leak,
		r900.LeakNow,
	)
}

func (r900 R900) Record() (r []string) {
	r = append(r, strconv.FormatUint(uint64(r900.ID), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Unkn1), 10))
	r = append(r, strconv.FormatUint(uint64(r900.AmrType), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Unkn2), 10))
	r = append(r, strconv.FormatUint(uint64(r900.NoUse), 10))
	r = append(r, strconv.FormatUint(uint64(r900.BackFlow), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Consumption), 10))
	r = append(r, strconv.FormatUint(uint64(r900.Leak), 10))
	r = append(r, strconv.FormatUint(uint64(r900.LeakNow), 10))

	return
}

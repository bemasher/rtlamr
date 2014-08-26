package main

import (
	"fmt"
	"log"
	"math"
)

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
	log.Println("Preamble:", cfg.Preamble)
}

type Decoder struct {
	cfg PacketConfig

	iq        []byte
	signal    []float64
	quantized []byte

	lut MagLUT

	preamble []byte
	slices   [][]byte

	pkt []byte
}

func NewDecoder(cfg PacketConfig) (d Decoder) {
	d.cfg = cfg

	d.iq = make([]byte, d.cfg.BufferLength<<1)
	d.signal = make([]float64, d.cfg.BufferLength)
	d.quantized = make([]byte, d.cfg.BufferLength)

	d.lut = NewMagLUT()

	d.preamble = make([]byte, len(d.cfg.Preamble))
	for idx := range d.cfg.Preamble {
		if d.cfg.Preamble[idx] == '1' {
			d.preamble[idx] = 1
		}
	}

	d.slices = make([][]byte, d.cfg.SymbolLength2)
	flat := make([]byte, d.cfg.BlockSize2-(d.cfg.BlockSize2%d.cfg.SymbolLength2))

	for symbolOffset := range d.slices {
		lower := symbolOffset * (d.cfg.BlockSize2 / d.cfg.SymbolLength2)
		upper := (symbolOffset + 1) * (d.cfg.BlockSize2 / d.cfg.SymbolLength2)
		d.slices[symbolOffset] = flat[lower:upper]
	}

	d.pkt = make([]byte, d.cfg.PacketSymbols>>3)

	return
}

func (d Decoder) Decode(input []byte) (pkts [][]byte) {
	// Shift new block into buffers.
	copy(d.iq, d.iq[d.cfg.BlockSize<<1:])
	copy(d.signal, d.signal[d.cfg.BlockSize:])
	copy(d.quantized, d.quantized[d.cfg.BlockSize:])
	copy(d.iq[d.cfg.PacketLength<<1:], input[:])

	iqBlock := d.iq[d.cfg.PacketLength<<1:]
	signalBlock := d.signal[d.cfg.PacketLength:]
	d.lut.Execute(iqBlock, signalBlock)

	signalBlock = d.signal[d.cfg.PacketLength-d.cfg.SymbolLength2:]
	d.Filter(signalBlock)
	signalBlock = d.signal[d.cfg.PacketLength-d.cfg.SymbolLength2:]
	Quantize(signalBlock, d.quantized[d.cfg.PacketLength-d.cfg.SymbolLength2:])
	d.Pack(d.quantized[:d.cfg.BlockSize2], d.slices)

	indexes := d.Search(d.slices, d.preamble)

	seen := make(map[string]bool)

	for _, qIdx := range indexes {
		if qIdx > d.cfg.BlockSize {
			continue
		}

		// Packet is 1 bit per byte, pack to 8-bits per byte.
		for pIdx := 0; pIdx < d.cfg.PacketSymbols; pIdx++ {
			d.pkt[pIdx>>3] <<= 1
			d.pkt[pIdx>>3] |= d.quantized[qIdx+(pIdx*d.cfg.SymbolLength2)]
		}

		pktStr := fmt.Sprintf("%02X", d.pkt)
		if !seen[pktStr] {
			seen[pktStr] = true
			pkts = append(pkts, make([]byte, len(d.pkt)))
			copy(pkts[len(pkts)-1], d.pkt)
		}
	}
	return
}

type MagLUT []float64

func NewMagLUT() (lut MagLUT) {
	lut = make([]float64, 0x100)
	for idx := range lut {
		lut[idx] = 127.4 - float64(idx)
		lut[idx] *= lut[idx]
	}
	return
}

func (lut MagLUT) Execute(input []byte, output []float64) {
	for idx := range output {
		lutIdx := idx << 1
		output[idx] = math.Sqrt(lut[input[lutIdx]] + lut[input[lutIdx+1]])
	}
}

func (d Decoder) Filter(input []float64) {
	csum := make([]float64, len(input)+1)

	var sum float64
	for idx, v := range input {
		sum += v
		csum[idx+1] = sum
	}

	lower := csum[d.cfg.SymbolLength:]
	upper := csum[d.cfg.SymbolLength2:]
	for idx := range input[:len(input)-d.cfg.SymbolLength2] {
		input[idx] = (lower[idx] - csum[idx]) - (upper[idx] - lower[idx])
	}

	return
}

func Quantize(input []float64, output []byte) {
	for idx, val := range input {
		output[idx] = byte(math.Float64bits(val)>>63) ^ 0x01
	}

	return
}

func (d Decoder) Pack(input []byte, slices [][]byte) {
	for symbolOffset, slice := range slices {
		for symbolIdx := range slice {
			slice[symbolIdx] = input[symbolIdx*d.cfg.SymbolLength2+symbolOffset]
		}
	}

	return
}

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
				indexes = append(indexes, symbolIdx*d.cfg.SymbolLength2+symbolOffset)
			}
		}
	}

	return
}

func NextPowerOf2(v int) int {
	return 1 << uint(math.Ceil(math.Log2(float64(v))))
}

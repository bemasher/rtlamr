package main

import "strconv"

// A lookup table for calculating magnitude of an interleaved unsigned byte
// stream.
type MagLUT []float64

// Shifts sample by 127.4 (most common DC offset value of rtl-sdr dongles) and
// stores square.
func NewMagLUT() (lut MagLUT) {
	lut = make([]float64, 0x100)
	for idx := range lut {
		lut[idx] = 127.4 - float64(idx)
		lut[idx] *= lut[idx]
	}
	return
}

// Matched filter implemented as integrate and dump. Output array is equal to
// the number of manchester coded symbols per packet.
func MatchedFilter(cfg PacketConfig, input []float64, bits int) (output []float64) {
	output = make([]float64, bits)

	fIdx := 0
	for idx := 0; fIdx < bits; idx += cfg.SymbolLength << 1 {
		offset := idx + cfg.SymbolLength

		for i := 0; i < cfg.SymbolLength; i++ {
			output[fIdx] += input[idx+i] - input[offset+i]
		}
		fIdx++
	}
	return
}

func BitSlice(input []float64) (data Data) {
	for _, v := range input {
		if v > 0.0 {
			data.Bits += "1"
		} else {
			data.Bits += "0"
		}
	}

	if len(data.Bits)%8 != 0 {
		return
	}

	data.Bytes = make([]byte, len(data.Bits)>>3)
	for byteIdx := range data.Bytes {
		bitIdx := byteIdx << 3
		b, _ := strconv.ParseUint(data.Bits[bitIdx:bitIdx+8], 2, 8)
		data.Bytes[byteIdx] = byte(b)
	}

	return
}

package main

import "math"

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

func Filter(input []float64, symbolLength int) {
	csum := make([]float64, len(input)+1)

	var sum float64
	for idx, v := range input {
		sum += v
		csum[idx+1] = sum
	}

	lower := csum[symbolLength:]
	upper := csum[symbolLength*2:]
	for idx := range input[:len(input)-symbolLength<<1] {
		input[idx] = (lower[idx] - csum[idx]) - (upper[idx] - lower[idx])
	}

	return
}

func Quantize(input []float64, output []byte) {
	for idx, val := range input {
		output[idx] = byte(math.Float64bits(val)>>63) ^ 0x01
	}
}

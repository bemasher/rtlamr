package implementation

import "math"

const (
	BlockSize = 16384

	ChipLength   = 72
	SymbolLength = ChipLength * 2
)

// MagNaive Start OMIT
func MagNaive(input []byte, output []float64) {
	inIdx := 0

	for outIdx := range output {
		// Normalize [0,256) to [-1.0,1.0]
		i := (127.5 - float64(input[inIdx])) / 127.5
		q := (127.5 - float64(input[inIdx+1])) / 127.5
		// Compute the magnitude of the sample.
		output[outIdx] = math.Sqrt(i*i + q*q)
		inIdx += 2
	}
}

// MagNaive Stop OMIT

// MagLUT Start OMIT
type MagLUT []float64

func NewMagLUT() (lut MagLUT) {
	lut = make([]float64, 256)
	for idx := range lut {
		// Pre-compute normalized squares.
		lut[idx] = (127.5 - float64(idx)) / 127.5
		lut[idx] *= lut[idx]
	}
	return
}

func (lut MagLUT) Execute(input []byte, output []float64) {
	inIdx := 0

	for outIdx := range output {
		output[outIdx] = math.Sqrt(lut[input[inIdx]] + lut[input[inIdx+1]])
		inIdx += 2
	}
}

// MagLUT Stop OMIT

func (lut MagLUT) ExecuteNoSqrt(input []byte, output []float64) {
	inIdx := 0

	for outIdx := range output {
		output[outIdx] = lut[input[inIdx]] + lut[input[inIdx+1]]
		inIdx += 2
	}
}

// FilterNaive Start OMIT
func FilterNaive(input, output []float64) {
	for idx := range output { // HL
		var sum float64
		for _, val := range input[idx : idx+ChipLength] {
			sum += val
		}
		for _, val := range input[idx+ChipLength : idx+ChipLength*2] {
			sum -= val
		}
		output[idx] = sum
	} // HL

	return
}

// FilterNaive Stop OMIT

var csum = make([]float64, BlockSize+SymbolLength*2+1)

// FilterOpt Start OMIT
func FilterOpt(input, output []float64) {
	// Compute the cummulative sum.
	var sum float64
	for idx, v := range input {
		sum += v
		csum[idx+1] = sum
	}

	lower := csum[ChipLength:]
	upper := csum[ChipLength*2:]
	for idx, l := range lower[:len(output)] {
		output[idx] = (l - csum[idx]) - (upper[idx] - l)
	}

	return
}

// FilterOpt Stop OMIT

// QuantizeNaive Start OMIT
func QuantizeNaive(input []float64, output []uint8) {
	for idx, val := range input {
		if val >= 0.0 {
			output[idx] = 1
		} else {
			output[idx] = 0
		}
	}
}

// QuantizeNaive Stop OMIT

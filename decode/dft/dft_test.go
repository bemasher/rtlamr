package dft

import (
	"math"
	"math/cmplx"
	"testing"
)

const (
	Tolerance = 2.5e-15
)

type DFT struct {
	name   string
	dft    func(ri, ii, ro, io []float64)
	length int
}

func StepFloat(n int) (out []float64) {
	out = make([]float64, n)
	for idx := 0; idx < n>>1; idx++ {
		out[idx] = 1
	}
	return
}

func StepComplex(n int) (out []complex128) {
	out = make([]complex128, n)
	for idx := 0; idx < n>>1; idx++ {
		out[idx] = 1
	}
	return
}

func Error(i, j []complex128) float64 {
	var err float64
	for idx := range i {
		err += cmplx.Abs(i[idx] - j[idx])
	}
	return err / float64(len(i))
}

func DirectFourierTransform(f []complex128, sign float64) {
	n := len(f)
	h := make([]complex128, n)
	phi := sign * 2.0 * math.Pi / float64(n)
	for w := 0; w < n; w++ {
		var t complex128
		for k := 0; k < n; k++ {
			t += f[k] * cmplx.Rect(1, phi*float64(k)*float64(w))
		}
		h[w] = t
	}
	copy(f, h)
}

func TestDFT(t *testing.T) {
	dfts := []DFT{
		{"DFT5", DFT5, 5},
		{"DFT6", DFT6, 6},
		{"DFT7", DFT7, 7},
		{"DFT8", DFT8, 8},
		{"DFT9", DFT9, 9},
		{"DFT10", DFT10, 10},
		{"DFT11", DFT11, 11},
		{"DFT12", DFT12, 12},
		{"DFT13", DFT13, 13},
		{"DFT14", DFT14, 14},
		{"DFT15", DFT15, 15},
		{"DFT16", DFT16, 16},
	}

	for _, dft := range dfts {
		re := StepFloat(dft.length)
		im := make([]float64, dft.length)
		dft.dft(re, im, re, im)

		genOutput := make([]complex128, dft.length)
		for idx := range genOutput {
			genOutput[idx] = complex(re[idx], im[idx])
		}

		directOutput := StepComplex(dft.length)
		DirectFourierTransform(directOutput, -1.0)

		err := Error(genOutput, directOutput)
		t.Logf("{Transform: %s Error: %0.6e}\n", dft.name, err)
		if err > Tolerance {
			t.Fail()
		}
	}
}

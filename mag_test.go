package main

import (
	"crypto/rand"

	"github.com/bemasher/rtlamr/decode"

	"testing"
)

func BenchmarkSqrtMag(b *testing.B) {
	lut := decode.NewSqrtMagLUT()
	input := make([]byte, 8192)
	output := make([]float64, 4096)

	rand.Read(input)

	b.SetBytes(4096)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		lut.Execute(input, output)
	}
}

func BenchmarkAlphaMaxBetaMinMag(b *testing.B) {
	lut := decode.NewAlphaMaxBetaMinLUT()
	input := make([]byte, 8192)
	output := make([]float64, 4096)

	rand.Read(input)

	b.SetBytes(4096)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		lut.Execute(input, output)
	}
}

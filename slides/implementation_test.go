package implementation

import (
	"math/rand"
	"testing"

	crand "crypto/rand"
)

func BenchmarkMagNaive(b *testing.B) {
	in := make([]byte, BlockSize)
	out := make([]float64, BlockSize>>1)

	b.SetBytes(BlockSize >> 1)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		MagNaive(in, out)
	}
}

func BenchmarkMagOpt(b *testing.B) {
	lut := NewMagLUT()

	in := make([]byte, BlockSize)
	out := make([]float64, BlockSize>>1)

	b.SetBytes(BlockSize >> 1)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		lut.Execute(in, out)
	}
}

func BenchmarkMagOptNoSqrt(b *testing.B) {
	lut := NewMagLUT()

	in := make([]byte, BlockSize)
	out := make([]float64, BlockSize>>1)

	b.SetBytes(BlockSize >> 1)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		lut.ExecuteNoSqrt(in, out)
	}
}

func BenchmarkFilterNaive(b *testing.B) {
	in := make([]float64, BlockSize+(SymbolLength*2))
	out := make([]float64, BlockSize)

	b.SetBytes(BlockSize)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		FilterNaive(in, out)
	}
}
func BenchmarkFilterOpt(b *testing.B) {
	in := make([]float64, BlockSize+(SymbolLength*2))
	out := make([]float64, BlockSize)

	b.SetBytes(BlockSize)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		FilterOpt(in, out)
	}
}

func BenchmarkQuantizeNaive(b *testing.B) {
	in := make([]float64, BlockSize)
	out := make([]uint8, BlockSize)

	crand.Read(out)
	for idx := range in {
		in[idx] = rand.Float64()
	}

	b.SetBytes(BlockSize)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		QuantizeNaive(in, out)
	}
}

func BenchmarkQuantizeOpt(b *testing.B) {
	in := make([]float64, BlockSize)
	out := make([]uint8, BlockSize)

	crand.Read(out)
	for idx := range in {
		in[idx] = rand.Float64()
	}

	b.SetBytes(BlockSize)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		QuantizeOpt(in, out)
	}
}

func BenchmarkQuantizeOptBranch(b *testing.B) {
	in := make([]float64, BlockSize)
	out := make([]uint8, BlockSize)

	crand.Read(out)
	for idx := range in {
		in[idx] = rand.Float64()
	}

	b.SetBytes(BlockSize)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		QuantizeOptBranch(in, out)
	}
}

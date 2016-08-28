package decode

import (
	"crypto/rand"
	"reflect"
	"testing"
)

func NewPacketConfig(chipLength int) (cfg PacketConfig) {
	cfg.CenterFreq = 912600155
	cfg.DataRate = 32768
	cfg.ChipLength = chipLength
	cfg.PreambleSymbols = 21
	cfg.PacketSymbols = 96

	cfg.Preamble = "111110010101001100000"

	return
}

func TestFskLUT(t *testing.T) {
	fsk := NewFskLUT()
	input := make([]byte, 4096)
	rand.Read(input)

	out0 := make([]float64, 2048)
	out1 := make([]float64, 2048)

	fsk.Execute(input, out0)
	fsk.naiveExecute(input, out1)

	if !reflect.DeepEqual(out0, out1) {
		t.Logf("%+0.3f\n", out0[:8])
		t.Logf("%+0.3f\n", out1[:8])
	}
}

func BenchmarkDecode(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72), NewMagLUT(), 1)

	block := make([]byte, d.DecCfg.BlockSize2)

	b.SetBytes(int64(d.DecCfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = d.Decode(block)
	}
}

func BenchmarkMagLUT(b *testing.B) {
	lut := NewMagLUT()

	const BlockSize = 16384

	in := make([]byte, BlockSize)
	out := make([]float64, BlockSize>>1)

	b.SetBytes(BlockSize >> 1)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		lut.Execute(in, out)
	}
}

func BenchmarkFskLUT(b *testing.B) {
	lut := NewFskLUT()

	const BlockSize = 16384

	in := make([]byte, BlockSize)
	out := make([]float64, BlockSize>>1)

	b.SetBytes(BlockSize >> 1)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		lut.Execute(in, out)
	}
}

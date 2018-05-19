package decode

import (
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

func TestSymbolLength(t *testing.T) {
	for idx := 7; idx <= 72; idx++ {
		d := NewDecoder(NewPacketConfig(idx))
		pLen := d.Cfg.BlockSize + d.Cfg.PreambleLength
		t.Logf("%d: %d/8 = %0.2f (%d)\n", idx, pLen, float64(pLen)/8.0, len(d.packed))

		block := make([]byte, d.Cfg.BlockSize2)
		d.Decode(block)
		d.Decode(block)
		d.Decode(block)
		d.Decode(block)
	}
}

func BenchmarkMagLUT(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72))

	input := make([]byte, d.Cfg.BlockSize2)

	b.SetBytes(int64(d.Cfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		d.demod.Execute(input, d.Signal[d.Cfg.SymbolLength:])
	}
}

func BenchmarkFilter(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72))

	b.SetBytes(int64(d.Cfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		d.Filter(d.Signal, d.Filtered)
	}
}

func BenchmarkQuantize(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72))

	b.SetBytes(int64(d.Cfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		Quantize(d.Filtered[d.Cfg.SymbolLength:], d.Quantized[d.Cfg.PacketLength:])
	}
}

func BenchmarkSearch(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72))

	b.SetBytes(int64(d.Cfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = d.Search()
	}
}

func BenchmarkDecode(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72))

	block := make([]byte, d.Cfg.BlockSize2)

	b.SetBytes(int64(d.Cfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = d.Decode(block)
	}
}

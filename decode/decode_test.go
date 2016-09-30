package decode

import "testing"

func NewPacketConfig(chipLength int) (cfg PacketConfig) {
	cfg.CenterFreq = 912600155
	cfg.DataRate = 32768
	cfg.ChipLength = chipLength
	cfg.PreambleSymbols = 21
	cfg.PacketSymbols = 96

	cfg.Preamble = "111110010101001100000"

	return
}

func BenchmarkMagLUT(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72), 1)

	input := make([]byte, d.DecCfg.BlockSize2)

	b.SetBytes(int64(d.DecCfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		d.demod.Execute(input, d.Signal[d.DecCfg.SymbolLength:])
	}
}

func BenchmarkFilter(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72), 1)

	b.SetBytes(int64(d.DecCfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		d.Filter(d.Signal, d.Filtered)
	}
}

func BenchmarkQuantize(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72), 1)

	b.SetBytes(int64(d.DecCfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		Quantize(d.Filtered[d.DecCfg.SymbolLength:], d.Quantized[d.DecCfg.PacketLength:])
	}
}

func BenchmarkTranspose(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72), 1)

	b.SetBytes(int64(d.DecCfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		d.Transpose(d.Quantized)
	}
}

func BenchmarkSearch(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72), 1)

	b.SetBytes(int64(d.DecCfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = d.Search()
	}
}

func BenchmarkDecode(b *testing.B) {
	d := NewDecoder(NewPacketConfig(72), 1)

	block := make([]byte, d.DecCfg.BlockSize2)

	b.SetBytes(int64(d.DecCfg.BlockSize))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = d.Decode(block)
	}
}

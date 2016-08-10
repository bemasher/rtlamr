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

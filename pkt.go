package main

import (
	"fmt"
	"math"
)

type PacketDecoder interface {
	PacketConfig() PacketConfig
	SearchPreamble([]float64) int
	Decode(Data) (fmt.Stringer, error)
	Close()
}

type PacketConfig struct {
	SymbolLength int
	BlockSize    uint
	SampleRate   uint

	PreambleSymbols uint
	PacketSymbols   uint

	PreambleLength uint
	PacketLength   uint

	PreambleBits string
}

func (pc PacketConfig) String() string {
	return fmt.Sprintf("{SymbolLength:%d BlockSize:%d SampleRate:%d PreambleSymbols:%d "+
		"PacketSymbols:%d PreambleLength:%d PacketLength:%d PreambleBits:%q}",
		pc.SymbolLength,
		pc.BlockSize,
		pc.SampleRate,
		pc.PreambleSymbols,
		pc.PacketSymbols,
		pc.PreambleLength,
		pc.PacketLength,
		pc.PreambleBits,
	)
}

func NextPowerOf2(v uint) uint {
	return 1 << uint(math.Ceil(math.Log2(float64(v))))
}

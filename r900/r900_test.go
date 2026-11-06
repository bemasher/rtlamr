package r900

import (
	"math"
	"math/rand"
	"sync"
	"testing"

	"github.com/bemasher/rtlamr/protocol"
)

func TestFilterMatchesReference(t *testing.T) {
	const (
		chipLength   = 8
		bufferLength = 4096
	)

	parser := newFilterParser(bufferLength, chipLength)
	random := rand.New(rand.NewSource(1))
	for index := range parser.signal {
		parser.signal[index] = random.Float32()
	}

	parser.filter()
	expected := referenceFilter(parser.signal, chipLength, bufferLength)
	for index := range expected {
		if parser.quantized[index] != expected[index] {
			t.Fatalf("quantized[%d] = %d, want %d", index, parser.quantized[index], expected[index])
		}
	}
}

func TestFilterZeroSignalRemainsZero(t *testing.T) {
	const (
		chipLength   = 8
		bufferLength = 256
	)

	parser := newFilterParser(bufferLength, chipLength)
	parser.filter()
	for index := 0; index < bufferLength-chipLength*4; index++ {
		if parser.quantized[index] != 0 {
			t.Fatalf("quantized[%d] = %d, want 0", index, parser.quantized[index])
		}
	}
}

func BenchmarkR900Filter(b *testing.B) {
	const (
		chipLength   = 8
		bufferLength = 1 << 15
	)

	parser := newFilterParser(bufferLength, chipLength)
	for index := range parser.signal {
		parser.signal[index] = float32((index*37)%255) / 255
	}

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		parser.filter()
	}
}

func BenchmarkR900ParseNoPackets(b *testing.B) {
	parser := newEmptyPacketParser()
	msgCh := make(chan protocol.Message)
	var wg sync.WaitGroup

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		wg.Add(1)
		parser.Parse(nil, msgCh, &wg)
	}
}

func TestParseWithoutPacketsSkipsFilterAndAdvancesSignal(t *testing.T) {
	parser := newEmptyPacketParser()
	msgCh := make(chan protocol.Message)
	var wg sync.WaitGroup
	wg.Add(1)
	parser.Parse(nil, msgCh, &wg)
	wg.Wait()

	if parser.csum[len(parser.csum)-1] != 0 {
		t.Fatal("filter ran without packets")
	}
	if parser.signal[parser.cfg.PacketLength] != parser.Decoder.Signal[parser.cfg.SymbolLength] {
		t.Fatal("signal buffer did not advance without packets")
	}
}

func newEmptyPacketParser() *Parser {
	parser := NewParser(8).(*Parser)
	cfg := parser.Cfg()
	cfg.SymbolLength = cfg.ChipLength * 2
	cfg.PreambleLength = cfg.PreambleSymbols * cfg.SymbolLength
	cfg.PacketLength = cfg.PacketSymbols * cfg.SymbolLength
	cfg.BlockSize = protocol.NextPowerOf2(cfg.PreambleLength)
	cfg.BufferLength = cfg.PacketLength + cfg.BlockSize

	decoder := &protocol.Decoder{
		Cfg:    cfg,
		Signal: make([]float32, cfg.BlockSize+cfg.SymbolLength),
	}
	for index := range decoder.Signal {
		decoder.Signal[index] = 0.5
	}
	parser.SetDecoder(decoder)
	return parser
}

func newFilterParser(bufferLength, chipLength int) *Parser {
	return &Parser{
		Decoder:   &protocol.Decoder{Cfg: protocol.PacketConfig{ChipLength: chipLength, BufferLength: bufferLength}},
		signal:    make([]float32, bufferLength),
		csum:      make([]float32, bufferLength+1),
		quantized: make([]byte, bufferLength),
	}
}

func referenceFilter(signal []float32, chipLength, bufferLength int) []byte {
	cumulative := make([]float32, len(signal)+1)
	for index, value := range signal {
		cumulative[index+1] = cumulative[index] + value
	}

	quantized := make([]byte, bufferLength)
	limit := bufferLength - chipLength*4
	for index := 0; index < limit; index++ {
		c0 := cumulative[index]
		c1 := cumulative[index+chipLength] + cumulative[index+chipLength]
		c2 := cumulative[index+chipLength*2] + cumulative[index+chipLength*2]
		c3 := cumulative[index+chipLength*3] + cumulative[index+chipLength*3]
		c4 := cumulative[index+chipLength*4]

		a0 := c2 - c4 - c0
		a1 := c1 - c2 + c3 - c4 - c0
		a2 := c1 - c3 + c4 - c0

		maxAbs := math.Abs(float64(a0))
		argmax := byte(0)
		if math.Abs(float64(a1)) > maxAbs {
			maxAbs = math.Abs(float64(a1))
			argmax = 1
		}
		if math.Abs(float64(a2)) > maxAbs {
			maxAbs = math.Abs(float64(a2))
			argmax = 2
		}

		quantized[index] = argmax
		if [3]float32{a0, a1, a2}[argmax] > 0 {
			quantized[index] += 3
		}
	}
	return quantized
}

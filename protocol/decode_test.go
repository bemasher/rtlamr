package protocol

import (
	"math"
	"math/rand"
	"testing"
)

func TestFilterMatchesPrefixSum(t *testing.T) {
	const (
		chipLength   = 8
		symbolLength = chipLength * 2
	)

	input := make([]float32, 1<<16+symbolLength)
	random := rand.New(rand.NewSource(1))
	for index := range input {
		input[index] = random.Float32()
	}

	output := make([]byte, len(input)-symbolLength+1)
	decoder := Decoder{
		Cfg:  PacketConfig{ChipLength: chipLength, SymbolLength: symbolLength},
		csum: make([]float32, len(input)+1),
	}
	decoder.Filter(input, output)

	expected := filterWithPrefixSum(input, chipLength, symbolLength, len(output))
	for index := range output {
		if output[index] != expected[index] {
			t.Fatalf("output[%d] = %d, want %d", index, output[index], expected[index])
		}
	}

	lut := newPairMagLUT()
	for index := range input {
		input[index] = lut[(index*37+index/11)&0xffff]
	}
	decoder.Filter(input, output)
	expected = filterWithPrefixSum(input, chipLength, symbolLength, len(output))
	for index := range output {
		if output[index] != expected[index] {
			t.Fatalf("lookup output[%d] = %d, want %d", index, output[index], expected[index])
		}
	}
}

func BenchmarkDecoderFilter(b *testing.B) {
	const (
		chipLength   = 8
		symbolLength = chipLength * 2
		blockSize    = 1 << 15
	)

	input := make([]float32, blockSize+symbolLength)
	output := make([]byte, blockSize+1)
	decoder := Decoder{
		Cfg:  PacketConfig{ChipLength: chipLength, SymbolLength: symbolLength},
		csum: make([]float32, len(input)+1),
	}
	for index := range input {
		input[index] = float32(index%101) / 101
	}

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		decoder.Filter(input, output)
	}
}

func BenchmarkMagLUTExecute(b *testing.B) {
	input := make([]byte, 1<<16)
	output := make([]float32, len(input)/2)
	for index := range input {
		input[index] = byte(index*37 + index/11)
	}
	lut := NewMagLUT()

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		lut.Execute(input, output)
	}
}

func TestPairMagLUTMatchesMagLUT(t *testing.T) {
	input := make([]byte, 1<<17)
	for first := 0; first < 1<<8; first++ {
		for second := 0; second < 1<<8; second++ {
			inputIdx := (first<<8 | second) << 1
			input[inputIdx] = byte(first)
			input[inputIdx+1] = byte(second)
		}
	}

	expected := make([]float32, len(input)/2)
	got := make([]float32, len(input)/2)
	NewMagLUT().Execute(input, expected)
	newPairMagLUT().Execute(input, got)
	for index := range expected {
		if got[index] != expected[index] {
			t.Fatalf("output[%d] = %v, want %v", index, got[index], expected[index])
		}
	}
}

func BenchmarkPairMagLUTExecute(b *testing.B) {
	input := make([]byte, 1<<16)
	output := make([]float32, len(input)/2)
	for index := range input {
		input[index] = byte(index*37 + index/11)
	}
	lut := newPairMagLUT()

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		lut.Execute(input, output)
	}
}

func TestSearchMatchesNaive(t *testing.T) {
	const (
		blockSize    = 128
		symbolLength = 8
		preambleSize = 5
	)

	decoder := newSearchDecoder(blockSize, symbolLength, preambleSize)
	random := rand.New(rand.NewSource(2))
	for index := range decoder.Quantized {
		decoder.Quantized[index] = byte(random.Intn(2))
	}
	preamble := []byte{1, 0, 1, 1, 0}

	got := decoder.Search(preamble)
	var expected []int
	for start := 0; start < blockSize; start++ {
		matches := true
		for preambleIdx, bit := range preamble {
			if decoder.Quantized[start+preambleIdx*symbolLength] != bit {
				matches = false
				break
			}
		}
		if matches {
			expected = append(expected, start)
		}
	}

	if len(got) != len(expected) {
		t.Fatalf("Search returned %d matches, want %d", len(got), len(expected))
	}
	for index := range got {
		if got[index] != expected[index] {
			t.Fatalf("Search match[%d] = %d, want %d", index, got[index], expected[index])
		}
	}
}

func TestSearchPackedMatchesPublicSearch(t *testing.T) {
	const (
		blockSize    = 128
		symbolLength = 8
		preambleSize = 5
	)

	shared := newSearchDecoder(blockSize, symbolLength, preambleSize)
	individual := newSearchDecoder(blockSize, symbolLength, preambleSize)
	random := rand.New(rand.NewSource(3))
	for index := range shared.Quantized {
		shared.Quantized[index] = byte(random.Intn(2))
	}
	copy(individual.Quantized, shared.Quantized)

	preambles := [][]byte{
		{1, 0, 1, 1, 0},
		{0, 1, 0, 0, 1},
		{1, 1, 0, 0, 1},
	}
	shared.packQuantized()
	for _, preamble := range preambles {
		got := shared.searchPacked(preamble)
		want := individual.Search(preamble)
		if len(got) != len(want) {
			t.Fatalf("Search returned %d matches, want %d", len(got), len(want))
		}
		for index := range got {
			if got[index] != want[index] {
				t.Fatalf("Search match[%d] = %d, want %d", index, got[index], want[index])
			}
		}
	}
}

func TestPackQuantizedMatchesBitLoop(t *testing.T) {
	decoder := newSearchDecoder(256, 8, 8)
	random := rand.New(rand.NewSource(4))
	for index := range decoder.Quantized {
		decoder.Quantized[index] = byte(random.Intn(2))
	}

	decoder.packQuantized()
	for packedIndex, packedByte := range decoder.packed {
		var expected byte
		for bitIndex := 0; bitIndex < 8; bitIndex++ {
			expected = (expected << 1) | decoder.Quantized[packedIndex*8+bitIndex]
		}
		if packedByte != expected {
			t.Fatalf("packed[%d] = %08b, want %08b", packedIndex, packedByte, expected)
		}
	}
}

func TestSearchPassByteMatchesReference(t *testing.T) {
	signal := []byte{0x00, 0xff, 0x0f, 0xf0, 0xaa, 0x55}
	indices := []int{0, 1, 2, 3, 4, 5}
	got := make([]int, 0, len(indices))
	_, got = searchPassByte(0xff, signal, indices, got)

	var expected []int
	for _, index := range indices {
		if signal[index] != 0xff {
			expected = append(expected, index)
		}
	}
	if len(got) != len(expected) {
		t.Fatalf("got %v, want %v", got, expected)
	}
	for index := range got {
		if got[index] != expected[index] {
			t.Fatalf("got %v, want %v", got, expected)
		}
	}
}

func BenchmarkDecoderSearch(b *testing.B) {
	const (
		blockSize    = 1 << 15
		symbolLength = 8
		preambleSize = 8
	)

	decoder := newSearchDecoder(blockSize, symbolLength, preambleSize)
	for index := range decoder.Quantized {
		decoder.Quantized[index] = byte((index*31 + index/7) & 1)
	}
	preamble := []byte{1, 0, 1, 1, 0, 0, 1, 0}

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		decoder.Search(preamble)
	}
}

func BenchmarkPackQuantized(b *testing.B) {
	decoder := newSearchDecoder(1<<15, 8, 8)
	for index := range decoder.Quantized {
		decoder.Quantized[index] = byte((index*31 + index/7) & 1)
	}

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		decoder.packQuantized()
	}
}

func BenchmarkSearchPassByte(b *testing.B) {
	const candidateCount = 1 << 12
	signal := make([]byte, candidateCount)
	indices := make([]int, candidateCount)
	for index := range indices {
		signal[index] = byte(index*31 + index/7)
		indices[index] = index
	}
	output := make([]int, 0, candidateCount)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_, output = searchPassByte(0xff, signal, indices, output[:0])
	}
}

func BenchmarkDecoderSearchMultiplePreambles(b *testing.B) {
	const (
		blockSize    = 1 << 15
		symbolLength = 8
		preambleSize = 8
	)

	decoder := newSearchDecoder(blockSize, symbolLength, preambleSize)
	for index := range decoder.Quantized {
		decoder.Quantized[index] = byte((index*31 + index/7) & 1)
	}
	preambles := [][]byte{
		{1, 0, 1, 1, 0, 0, 1, 0},
		{0, 1, 0, 0, 1, 1, 0, 1},
		{1, 1, 0, 0, 1, 0, 0, 1},
		{0, 0, 1, 1, 0, 1, 1, 0},
	}

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for _, preamble := range preambles {
			decoder.Search(preamble)
		}
	}
}

func BenchmarkDecoderSearchSharedPacking(b *testing.B) {
	const (
		blockSize    = 1 << 15
		symbolLength = 8
		preambleSize = 8
	)

	decoder := newSearchDecoder(blockSize, symbolLength, preambleSize)
	for index := range decoder.Quantized {
		decoder.Quantized[index] = byte((index*31 + index/7) & 1)
	}
	preambles := [][]byte{
		{1, 0, 1, 1, 0, 0, 1, 0},
		{0, 1, 0, 0, 1, 1, 0, 1},
		{1, 1, 0, 0, 1, 0, 0, 1},
		{0, 0, 1, 1, 0, 1, 1, 0},
	}

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		decoder.packQuantized()
		for _, preamble := range preambles {
			decoder.searchPacked(preamble)
		}
	}
}

func newSearchDecoder(blockSize, symbolLength, preambleSize int) Decoder {
	quantizedLength := blockSize + symbolLength*preambleSize
	return Decoder{
		Cfg:       PacketConfig{BlockSize: blockSize, SymbolLength: symbolLength},
		Quantized: make([]byte, quantizedLength),
		packed:    make([]byte, (quantizedLength+7)>>3),
		sIdxA:     make([]int, 0, blockSize),
		sIdxB:     make([]int, 0, blockSize),
	}
}

func filterWithPrefixSum(input []float32, chipLength, symbolLength, outputLength int) []byte {
	prefix := make([]float32, len(input)+1)
	for index, value := range input {
		prefix[index+1] = prefix[index] + value
	}

	output := make([]byte, outputLength)
	for index := range output {
		lower := prefix[index+chipLength]
		upper := prefix[index+symbolLength]
		difference := (lower - prefix[index]) - (upper - lower)
		output[index] = 1 - byte(math.Float32bits(difference)>>31)
	}
	return output
}

package nim

import (
	"encoding/binary"
	"io"
	"os"
	"testing"
)

func TestNIM(t *testing.T) {
	inputFile, err := os.Open("908M.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer inputFile.Close()

	// inputBuf := bufio.NewReader(inputFile)

	p := NewParser(66, 1)
	block := make([]byte, p.Cfg().BlockSize2)

	outFile, err := os.Create("filter.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer outFile.Close()

	d := p.Dec()

	for idx := 0; ; idx++ {
		_, err := inputFile.Read(block)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}

		indeces := p.Dec().Decode(block)
		// binary.Write(outFile, binary.LittleEndian, d.Signal[d.DecCfg.SymbolLength:])
		binary.Write(outFile, binary.LittleEndian, d.Filtered)
		for _, pkt := range p.Parse(indeces) {
			_ = pkt
			// fmt.Printf("%02X\n", pkt)
		}

		if err == io.EOF {
			break
		}
	}
}

package gen

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/parse"

	_ "github.com/bemasher/rtlamr/scm"
)

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
}

func TestNewRandSCM(t *testing.T) {
	bch := crc.NewCRC("BCH", 0, 0x6F63, 0)

	for i := 0; i < 512; i++ {
		scm, err := NewRandSCM()
		if err != nil {
			t.Fatal(err)
		}

		checksum := bch.Checksum(scm[2:])
		if checksum != 0 {
			t.Fatalf("Failed checksum: %04X\n", checksum)
		}
	}
}

func TestManchesterLUT(t *testing.T) {
	lut := NewManchesterLUT()

	recv := lut.Encode([]byte{0x00})
	expt := []byte{0x55, 0x55}
	if !bytes.Equal(recv, expt) {
		t.Fatalf("Expected %02X got %02X\n", expt, recv)
	}

	recv = lut.Encode([]byte{0xF9, 0x53})
	expt = []byte{0xAA, 0x96, 0x66, 0x5A}
	if !bytes.Equal(recv, expt) {
		t.Fatalf("Expected %02X got %02X\n", expt, recv)
	}
}

func TestUnpackBits(t *testing.T) {
	t.Logf("%d\n", UnpackBits([]byte{0xF9, 0x53}))
}

func TestUpsample(t *testing.T) {
	t.Logf("%d\n", Upsample(UnpackBits([]byte{0xF9, 0x53}), 8))
}

func TestCmplxOscillatorS8(t *testing.T) {
	t.SkipNow()
	signalFile, err := os.Create("cmplxs8.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer signalFile.Close()

	err = binary.Write(signalFile, binary.BigEndian, CmplxOscillatorS8(1<<10, 5e3, 2.4e6))
	if err != nil {
		t.Fatal(err)
	}
}

func TestCmplxOscillatorU8(t *testing.T) {
	t.SkipNow()

	signalFile, err := os.Create("cmplxu8.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer signalFile.Close()

	err = binary.Write(signalFile, binary.BigEndian, CmplxOscillatorU8(1<<10, 5e3, 2.4e6))
	if err != nil {
		t.Fatal(err)
	}
}

type TestData struct {
	FileSections [][]int64
	Packets      []Packet
}

type Packet struct {
	Data      []byte
	Freq      float64
	Amplitude float64
}

func TestSCMGenerate(t *testing.T) {
	p, err := parse.NewParser("scm", 72, 1)
	if err != nil {
		t.Fatal(err)
	}

	cfg := p.Cfg()
	lut := NewManchesterLUT()

	outFile, err := os.Create("generated.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer outFile.Close()

	noisedB := -35.0
	noiseAmp := math.Pow(10, noisedB/20)

	var block []byte

	noise := make([]byte, cfg.BlockSize<<3)

	for _, pkt := range testData.Packets {
		manchester := lut.Encode(pkt.Data)
		bits := UnpackBits(manchester)
		bits = Upsample(bits, 72)

		carrier := CmplxOscillatorF64(len(bits), pkt.Freq, float64(cfg.SampleRate))
		for idx := range carrier {
			carrier[idx] *= float64(bits[idx>>1]) * pkt.Amplitude
			carrier[idx] += (rand.Float64() - 0.5) * 2.0 * noiseAmp
		}

		if len(block) != len(carrier) {
			block = make([]byte, len(carrier))
		}
		F64toU8(carrier, block)

		outFile.Write(block)

		for idx := range noise {
			noise[idx] = byte((rand.Float64()-0.5)*2.0*noiseAmp*127.5 + 127.5)
		}

		outFile.Write(noise)
	}
}

func TestSCMDecode(t *testing.T) {
	inFile, err := os.Open("generated.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer inFile.Close()

	factors := []int{1, 2, 3, 4, 6, 8, 9, 12, 18}
	results := make([][]int, len(factors))
	for factor := range results {
		results[factor] = make([]int, len(testData.FileSections))
	}

	for factorIdx, factor := range factors {
		p, err := parse.NewParser("scm", 72, factor)
		if err != nil {
			t.Fatal(err)
		}

		cfg := p.Cfg()

		block := make([]byte, cfg.BlockSize2)
		for sectionIdx, section := range testData.FileSections {
			r := io.NewSectionReader(inFile, section[0], section[1]-section[0])

			for {
				_, err := r.Read(block)
				indices := p.Dec().Decode(block)
				for _, msg := range p.Parse(indices) {
					_ = msg
					results[factorIdx][sectionIdx]++
				}

				if err == io.EOF {
					break
				}
			}
		}
	}

	for _, factor := range results {
		var row []string
		for _, count := range factor {
			row = append(row, strconv.Itoa(count))
		}
		t.Log(strings.Join(row, ","))
	}
}

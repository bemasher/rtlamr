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

type TestCase struct {
	*io.PipeReader
	Data           []byte
	SignalLevelIdx int
	DecimationIdx  int
}

func TestGenerateSCM(t *testing.T) {
	genParser, err := parse.NewParser("scm", 72, 1)
	if err != nil {
		t.Fatal(err)
	}

	cfg := genParser.Cfg()
	lut := NewManchesterLUT()

	noisedB := -30.0
	noiseAmp := math.Pow(10, noisedB/20)

	testCases := make(chan TestCase)

	signalLevels := []float64{-40, -35, -30, -25, -20, -15, -10, -5, 0}
	decimationFactors := []int{1, 2, 3, 4, 6, 8, 9, 12, 18}

	go func() {
		var block []byte
		noise := make([]byte, cfg.BlockSize2<<1)

		for signalLevelIdx, signalLevel := range signalLevels {
			for decimationIdx, _ := range decimationFactors {
				for idx := 0; idx < 24; idx++ {
					r, w := io.Pipe()

					scm, _ := NewRandSCM()
					testCases <- TestCase{r, scm, signalLevelIdx, decimationIdx}

					manchester := lut.Encode(scm)
					bits := Upsample(UnpackBits(manchester), 72<<1)

					freq := (rand.Float64() - 0.5) * float64(cfg.SampleRate)
					carrier := CmplxOscillatorF64(len(bits)>>1, freq, float64(cfg.SampleRate))

					signalAmplitude := math.Pow(10, signalLevel/20)
					for idx := range carrier {
						carrier[idx] *= float64(bits[idx]) * signalAmplitude
						carrier[idx] += (rand.Float64() - 0.5) * 2.0 * noiseAmp
					}

					if len(block) != len(carrier) {
						block = make([]byte, len(carrier))
					}
					F64toU8(carrier, block)

					w.Write(block)
					for idx := range noise {
						noise[idx] = byte((rand.Float64()-0.5)*2.0*noiseAmp*127.5 + 127.5)
					}
					w.Write(noise)
					w.Close()
				}
			}
		}
		close(testCases)
	}()

	results := make([][]int, len(decimationFactors))
	for idx := range results {
		results[idx] = make([]int, len(signalLevels))
	}

	for testCase := range testCases {
		p, err := parse.NewParser("scm", 72, decimationFactors[testCase.DecimationIdx])
		if err != nil {
			t.Fatal(err)
		}

		cfg := p.Cfg()
		block := make([]byte, cfg.BlockSize2)

		for {
			_, err := testCase.Read(block)
			indices := p.Dec().Decode(block)
			for _ = range p.Parse(indices) {
				results[testCase.DecimationIdx][testCase.SignalLevelIdx]++
			}

			if err == io.EOF {
				testCase.Close()
				break
			}
		}
	}

	for idx := range results {
		var row []string
		for _, count := range results[idx] {
			row = append(row, strconv.Itoa(count))
		}
		t.Log(strings.Join(row, ","))
	}
}

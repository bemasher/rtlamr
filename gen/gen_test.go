package gen

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"testing"

	"github.com/bemasher/hackrf"
	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/parse"

	_ "github.com/bemasher/rtlamr/scm"
)

var bch crc.CRC

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
	bch = crc.NewCRC("BCH", 0, 0x6F63, 0)
}

func TestNewRandSCM(t *testing.T) {
	for i := 0; i < 512; i++ {
		scm, err := NewRandSCM()
		if err != nil {
			t.Fatal(err)
		}

		checksum := bch.Checksum(scm[2:])
		if checksum != 0 {
			t.Fatalf("Failed checksum: %04X\n", checksum)
		}
		t.Logf("%02X %04X\n", scm, checksum)
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

func TestHackRF(t *testing.T) {
	channels := []uint64{
		909586111, 909782679, 909979247, 910175815, 911224178, 911420746, // 0
		911617314, 911813882, 912010451, 912207019, 912403587, 912600155, // 6
		912796723, 912993291, 913189859, 913386427, 913582995, 913779563, // 12
		913976132, 915024495, 915221063, 915417631, 915614199, 915810767, // 18
		916007335, 916203903, 916400471, 916597040, 916793608, 916990176, // 24
		917186744, 917383312, 917579880, 917776448, 918824811, 919021379, // 30
		919217947, 919414516, 919611084, 919807652, 920004220, 920200788, // 36
		920397356, 920593924, 920790492, 920987060, 921183628, 921380197, // 42
		921576765, 921773333, // 44
	}

	err := hackrf.Init()
	if err != nil {
		t.Fatal(err)
	}
	defer hackrf.Exit()

	var dev hackrf.HackRF
	err = dev.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()

	dev.SetAmp(false)
	dev.SetTXVGAGain(0)

	samplerate := float64(16000000)
	dev.SetSampleRate(samplerate)

	center := channels[24]
	dev.SetFreq(center)

	in, out := io.Pipe()
	lut := NewManchesterLUT()
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt, os.Kill)

	go func() {
		err := dev.StartTX(func(buf []int8) int {
			binary.Read(in, binary.BigEndian, buf)

			return 0
		})
		if err != nil {
			t.Fatal(err)
		}
	}()

	for {
		select {
		case <-sig:
			log.Printf("SIGINT: Exiting...")
			dev.StopTX()
			break
		default:
			scm, _ := NewRandSCM()
			log.Printf("%02X\n", scm)
			manchester := lut.Encode(scm)
			bits := UnpackBits(manchester)
			bits = Upsample(bits, 488)

			channelIdx := rand.Intn(len(channels))
			freq := float64(center) - float64(channels[channelIdx])
			carrier := CmplxOscillatorS8(len(bits), freq, samplerate)

			for idx := range carrier {
				carrier[idx] *= int8(bits[idx>>1])
			}

			binary.Write(out, binary.BigEndian, carrier)
		}
	}
}

func TestGenerate(t *testing.T) {
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

	noisedB := -40.0
	noiseAmp := math.Pow(10, noisedB/20)

	var block []byte

	noise := make([]byte, cfg.BlockSize<<3)

	for sigdB := -50.0; sigdB < 0.0; sigdB += 5.0 {
		sigAmp := math.Pow(10, sigdB/20)

		startOffset, _ := outFile.Seek(0, os.SEEK_CUR)
		for idx := 0; idx < 32; idx++ {
			scm, _ := NewRandSCM()
			// t.Logf("%02X\n", scm)
			manchester := lut.Encode(scm)
			bits := UnpackBits(manchester)
			bits = Upsample(bits, 72)

			carrier := CmplxOscillatorF64(len(bits), 100e3, float64(cfg.SampleRate))
			for idx := range carrier {
				carrier[idx] *= float64(bits[idx>>1]) * sigAmp
				carrier[idx] += rand.NormFloat64() * noiseAmp
			}

			if len(block) != len(carrier) {
				block = make([]byte, len(carrier))
			}
			F64toU8(carrier, block)

			outFile.Write(block)

			for idx := range noise {
				noise[idx] = byte(rand.NormFloat64()*noiseAmp*127.5 + 127.5)
			}

			outFile.Write(noise)
		}
		stopOffset, _ := outFile.Seek(0, os.SEEK_CUR)
		t.Logf("%#v\n", []int64{startOffset, stopOffset})
	}
}

func TestDecimate(t *testing.T) {
	sections := [][]int64{
		{0, 1933312},
		{1933312, 3866624},
		{3866624, 5799936},
		{5799936, 7733248},
		{7733248, 9666560},
		{9666560, 11599872},
		{11599872, 13533184},
		{13533184, 15466496},
		{15466496, 17399808},
		{17399808, 19333120},
	}

	inFile, err := os.Open("generated.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer inFile.Close()

	factors := []int{1, 2, 3, 4, 6, 8, 9, 12, 18}
	results := make([][]int, len(factors))
	for factor := range results {
		results[factor] = make([]int, len(sections))
	}

	for factorIdx, factor := range factors {
		t.Log(factor)
		p, err := parse.NewParser("scm", 72, factor)
		if err != nil {
			t.Fatal(err)
		}

		cfg := p.Cfg()

		block := make([]byte, cfg.BlockSize2)
		for sectionIdx, section := range sections {
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

	for factorIdx, factor := range results {
		fmt.Print(factors[factorIdx], ",")
		for _, count := range factor {
			fmt.Printf("%d,", count)
		}
		fmt.Println()
	}
}

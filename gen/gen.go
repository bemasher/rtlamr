package gen

import (
	"crypto/rand"
	"fmt"
	"math"

	"github.com/bemasher/rtlamr/crc"
)

func NewRandSCM() (pkt []byte, err error) {
	bch := crc.NewCRC("BCH", 0, 0x6F63, 0)

	pkt = make([]byte, 12)
	_, err = rand.Read(pkt)
	if err != nil {
		return nil, err
	}

	pkt[0] = 0xF9
	pkt[1] = 0x53
	pkt[2] &= 0x07

	checksum := bch.Checksum(pkt[2:10])
	pkt[10] = uint8(checksum >> 8)
	pkt[11] = uint8(checksum & 0xFF)

	return
}

type ManchesterLUT [16]byte

func NewManchesterLUT() ManchesterLUT {
	return ManchesterLUT{
		85, 86, 89, 90, 101, 102, 105, 106, 149, 150, 153, 154, 165, 166, 169, 170,
	}
}

func (lut ManchesterLUT) Encode(data []byte) (manchester []byte) {
	manchester = make([]byte, len(data)<<1)

	for idx := range data {
		manchester[idx<<1] = lut[data[idx]>>4]
		manchester[idx<<1+1] = lut[data[idx]&0x0F]
	}

	return
}

func UnpackBits(data []byte) []byte {
	bits := make([]byte, len(data)<<3)

	for idx, b := range data {
		offset := idx << 3
		for bit := 7; bit >= 0; bit-- {
			bits[offset+(7-bit)] = (b >> uint8(bit)) & 0x01
		}
	}

	return bits
}

func Upsample(bits []byte, factor int) []byte {
	signal := make([]byte, len(bits)*factor)

	for idx, b := range bits {
		offset := idx * factor
		for i := 0; i < factor; i++ {
			signal[offset+i] = b
		}
	}

	return signal
}

func CmplxOscillatorS8(samples int, freq float64, samplerate float64) []int8 {
	signal := make([]int8, samples<<1)

	for idx := 0; idx < samples<<1; idx += 2 {
		s, c := math.Sincos(2 * math.Pi * float64(idx) * freq / samplerate)
		signal[idx] = int8(s * 127.5)
		signal[idx+1] = int8(c * 127.5)
	}

	return signal
}

func CmplxOscillatorU8(samples int, freq float64, samplerate float64) []uint8 {
	signal := make([]uint8, samples<<1)

	for idx := 0; idx < samples<<1; idx += 2 {
		s, c := math.Sincos(2 * math.Pi * float64(idx) * freq / samplerate)
		signal[idx] = uint8(s*127.5 + 127.5)
		signal[idx+1] = uint8(c*127.5 + 127.5)
	}

	return signal
}

func CmplxOscillatorF64(samples int, freq float64, samplerate float64) []float64 {
	signal := make([]float64, samples<<1)

	for idx := 0; idx < len(signal); idx += 2 {
		signal[idx], signal[idx+1] = math.Sincos(2 * math.Pi * float64(idx) * freq / samplerate)
	}

	return signal
}

func F64toU8(f64 []float64, u8 []byte) {
	if len(f64) != len(u8) {
		panic(fmt.Errorf("arrays must have same dimensions: %d != %d", len(f64), len(u8)))
	}

	for idx, val := range f64 {
		u8[idx] = uint8(val*127.5 + 127.5)
	}
}

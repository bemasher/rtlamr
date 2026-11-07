package r900

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/bemasher/rtlamr/protocol"
)

// Returns R900 with test data.
func testR900() R900 {
	r900 := R900{
		ID:          0xDEADBEEF,
		Unkn1:       0xA5,
		NoUse:       0x2A,
		BackFlow:    0x3,
		Consumption: 0x123456,
		Unkn3:       0x2,
		Leak:        0x9,
		LeakNow:     0x1,
	}
	r900.checksum = r900.GenCRC()

	return r900
}

func TestSymMap(t *testing.T) {
	r900 := testR900()

	// Same layout Parse reads: fields MSB first, then the parity symbols.
	bits := fmt.Sprintf("%032b%08b%06b%02b%024b%02b%04b%02b",
		r900.ID, r900.Unkn1, r900.NoUse, r900.BackFlow, r900.Consumption,
		r900.Unkn3, r900.Leak, r900.LeakNow)

	var want []byte
	for i := 0; i < 16; i++ {
		s, _ := strconv.ParseUint(bits[5*i:5*i+5], 2, 8)
		want = append(want, chips[s/6]<<4|chips[s%6])
	}
	for _, s := range r900.checksum {
		want = append(want, chips[s/6]<<4|chips[s%6])
	}

	if got := r900.SymMap(); !bytes.Equal(got, want) {
		t.Fatalf("SymMap() = %X, want %X", got, want)
	}
}

// modIQ expands one amplitude per chip to IQ bytes, chipLength samples per chip.
func modIQ(stream []byte, chipLength int) []byte {
	const low, high = 127, 255

	iq := make([]byte, 0, len(stream)*chipLength*2)
	for _, c := range stream {
		i := byte(low)
		if c != 0 {
			i = high
		}
		for n := 0; n < chipLength; n++ {
			iq = append(iq, i, low)
		}
	}
	return iq
}

func TestDecodeRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	want := R900{
		ID:          rng.Uint32(),
		Unkn1:       uint8(rng.Intn(1 << 8)),
		NoUse:       uint8(rng.Intn(1 << 6)),
		BackFlow:    uint8(rng.Intn(1 << 2)),
		Consumption: rng.Uint32() & (1<<24 - 1),
		Unkn3:       uint8(rng.Intn(1 << 2)),
		Leak:        uint8(rng.Intn(1 << 4)),
		LeakNow:     uint8(rng.Intn(1 << 2)),
	}

	// SymCRC only reads the data symbols, so the zero checksum is fine here.
	want.checksum = want.GenCRC()
	symMap := want.SymMap()

	for _, chipLength := range []int{8, 32, 72} {
		t.Run(fmt.Sprintf("chipLength=%d", chipLength), func(t *testing.T) {
			dec := protocol.NewDecoder()
			parser := NewParser(chipLength)
			dec.RegisterProtocol(parser)
			dec.Allocate()

			// Stream of chips as amplitudes: the preamble as Manchester pairs,
			// then each symbol as two digits of four chips.
			preamble := parser.Cfg().Preamble
			var stream []byte
			for _, b := range preamble {
				if b == '1' {
					stream = append(stream, 1, 0)
				} else {
					stream = append(stream, 0, 1)
				}
			}
			for _, s := range symMap {
				hi, _ := digit(s >> 4)
				lo, _ := digit(s & 0x0F)
				for _, d := range []byte{hi, lo} {
					c := chips[d]
					stream = append(stream, c>>3&1, c>>2&1, c>>1&1, c&1)
				}
			}

			// The decoder keeps PacketLength samples of history, so feed
			// enough blocks that the last one sees the packet from index 0.
			blockSize := dec.Cfg.BlockSize
			blocks := dec.Cfg.PacketLength/blockSize + 3
			start := blocks*blockSize - blockSize - dec.Cfg.PacketLength
			samples := blocks * blockSize

			signal := make([]byte, 2*samples)
			for i := range signal {
				signal[i] = 127 // silence: both I and Q at the DC offset
			}
			copy(signal[2*start:], modIQ(stream, chipLength))

			var got []R900
			for n := 0; n < blocks; n++ {
				block := signal[2*n*blockSize : 2*(n+1)*blockSize]
				for msg := range dec.Decode(block) {
					got = append(got, msg.(R900))
				}
			}

			if len(got) != 1 {
				t.Fatalf("decoded %d messages, want 1", len(got))
			}
			if got[0].String() != want.String() {
				t.Fatalf("decoded %s, want %s", got[0], want)
			}
			if !bytes.Equal(got[0].Checksum(), want.Checksum()) {
				t.Fatalf("checksum %X, want %X", got[0].Checksum(), want.Checksum())
			}
		})
	}
}

func TestCapture(t *testing.T) {
	t.Run("capture.bin symlen=32", testCapture("capture.bin", 32))
}

func testCapture(filename string, symbolLength int) func(t *testing.T) {
	return func(t *testing.T) {
		signal, err := os.ReadFile(filename)
		if err != nil {
			t.Skip("capture file not found")
		}

		dec := protocol.NewDecoder()
		dec.RegisterProtocol(NewParser(symbolLength))
		dec.Allocate()

		block := 2 * dec.Cfg.BlockSize
		var decoded []R900
		for n := block; n <= len(signal); n += block {
			for msg := range dec.Decode(signal[n-block : n]) {
				decoded = append(decoded, msg.(R900))
				t.Logf("%#v", msg)
			}
		}

		if len(decoded) == 0 {
			t.Fatal("decoded no messages")
		}
	}
}

func TestGenCRC(t *testing.T) {
	r900 := testR900()

	// Decode the symbols the way Parse does and check the syndromes.
	var code [31]byte // 16 data, 10 zeros, 5 parity
	for i, b := range r900.SymMap() {
		hi, okHi := digit(b >> 4)
		lo, okLo := digit(b & 0x0F)
		if !okHi || !okLo {
			t.Fatalf("byte %d (%02X) is not two chips", i, b)
		}
		if i < 16 {
			code[i] = hi*6 + lo
		} else {
			code[26+i-16] = hi*6 + lo
		}
	}
	for i, syn := range field.Syndrome(code[:], 5, 29) {
		if syn != 0 {
			t.Fatalf("syndrome %d = %d, want 0", i, syn)
		}
	}
}

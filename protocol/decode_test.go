package protocol_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"testing"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/protocol"
	"github.com/bemasher/rtlamr/scm"
)

// newDecoder builds a decoder for a single message type, the same way
// main.go does.
func newDecoder(t *testing.T, msgType string, chipLength int) *protocol.Decoder {
	t.Helper()

	p, err := protocol.NewParser(msgType, chipLength)
	if err != nil {
		t.Fatal(err)
	}

	d := protocol.NewDecoder()
	d.RegisterProtocol(p)
	d.Allocate()

	return &d
}

// decodeStream feeds r through d one block at a time and returns every
// message the parsers emit. Messages are retained across blocks, the way a
// library consumer of the protocol package might hold on to them.
func decodeStream(t *testing.T, d *protocol.Decoder, r io.Reader) (msgs []protocol.Message) {
	t.Helper()

	for {
		block := make([]byte, d.Cfg.BlockSize2)
		if _, err := io.ReadFull(r, block); err != nil {
			return
		}
		for msg := range d.Decode(block) {
			msgs = append(msgs, msg)
		}
	}
}

// modulate turns packet bytes into 8-bit interleaved IQ samples the way an
// ERT transmitter looks to rtl_tcp: on-off keyed, Manchester coded, MSB
// first. A 1 bit is carrier-on for the first chip and off for the second.
func modulate(pkt []byte, chipLength int) []byte {
	var iq []byte
	chip := func(on bool) {
		for i := 0; i < chipLength; i++ {
			if on {
				iq = append(iq, 255, 127)
			} else {
				iq = append(iq, 127, 127)
			}
		}
	}

	for _, b := range pkt {
		for bit := 7; bit >= 0; bit-- {
			one := (b>>uint(bit))&1 == 1
			chip(one)
			chip(!one)
		}
	}

	return iq
}

// silence returns n samples of carrier-off IQ data.
func silence(n int) []byte {
	return bytes.Repeat([]byte{127, 127}, n)
}

// TestSampleFile is a golden regression test against the capture shipped
// in assets/sample.bin. That file predates the current DSP defaults and only
// yields a clean SCM decode at a chip length of 78, so that's what we use.
func TestSampleFile(t *testing.T) {
	f, err := os.Open("../assets/sample.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	msgs := decodeStream(t, newDecoder(t, "scm", 78), f)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d: %v", len(msgs), msgs)
	}

	got, ok := msgs[0].(scm.SCM)
	if !ok {
		t.Fatalf("expected scm.SCM, got %T", msgs[0])
	}

	want := scm.SCM{ID: 17569243, Type: 8, TamperPhy: 1, TamperEnc: 3, Consumption: 101714, ChecksumVal: 0x57F9}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// buildSCM assembles a 96-bit SCM packet with a valid BCH checksum.
func buildSCM(id uint32, ertType, tamperPhy, tamperEnc uint8, consumption uint32) []byte {
	// Bit layout from scm.NewSCM.
	var v [96]byte
	put := func(start, width int, val uint64) {
		for i := 0; i < width; i++ {
			v[start+i] = byte(val>>uint(width-1-i)) & 1
		}
	}

	for i, c := range "111110010101001100000" {
		v[i] = byte(c - '0')
	}
	put(21, 2, uint64(id>>24))
	put(24, 2, uint64(tamperPhy))
	put(26, 4, uint64(ertType))
	put(30, 2, uint64(tamperEnc))
	put(32, 24, uint64(consumption))
	put(56, 24, uint64(id&0xFFFFFF))

	pkt := make([]byte, 12)
	for i, bit := range v {
		pkt[i>>3] |= bit << uint(7-i&7)
	}

	bch := crc.NewCRC("BCH", 0, 0x6F63, 0)
	binary.BigEndian.PutUint16(pkt[10:12], bch.Checksum(pkt[2:10]))

	return pkt
}

// TestSyntheticSCM round-trips a synthetic SCM packet through the whole
// demodulator at the default chip length.
func TestSyntheticSCM(t *testing.T) {
	const chipLength = 72
	want := scm.SCM{ID: 0x2ABCDEF, Type: 7, TamperPhy: 2, TamperEnc: 1, Consumption: 1234567}

	var sig []byte
	sig = append(sig, silence(20000)...)
	sig = append(sig, modulate(buildSCM(want.ID, want.Type, want.TamperPhy, want.TamperEnc, want.Consumption), chipLength)...)
	sig = append(sig, silence(60000)...)

	msgs := decodeStream(t, newDecoder(t, "scm", chipLength), bytes.NewReader(sig))
	if len(msgs) == 0 {
		t.Fatal("no messages decoded")
	}

	got := msgs[0].(scm.SCM)
	got.ChecksumVal = 0
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

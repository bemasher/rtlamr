package protocol_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"testing"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/idm"
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

// buildIDM assembles a 92-byte IDM packet with valid serial and packet CRCs.
func buildIDM(serial uint32, tamper, outage [6]byte) []byte {
	pkt := make([]byte, 92)
	binary.BigEndian.PutUint32(pkt[0:4], 0x555516A3)
	pkt[4] = 0x1C // PacketTypeID
	pkt[5] = 0x5C // PacketLength
	pkt[8] = 0x07 // ERTType
	binary.BigEndian.PutUint32(pkt[9:13], serial)
	copy(pkt[15:21], tamper[:])
	copy(pkt[23:29], outage[:])
	binary.BigEndian.PutUint32(pkt[29:33], 424242)

	ccitt := crc.NewCRC("CCITT", 0xFFFF, 0x1021, 0x1D0F)

	serialBuf := append(append([]byte{}, pkt[9:13]...), 0, 0)
	binary.BigEndian.PutUint16(serialBuf[4:], ^ccitt.Checksum(serialBuf[:4]))
	copy(pkt[88:90], serialBuf[4:])

	binary.BigEndian.PutUint16(pkt[90:92], ^ccitt.Checksum(pkt[4:90]))

	return pkt
}

// TestIDMRetainedMessages decodes two different IDM transmissions and checks
// that the first message's byte-slice fields aren't overwritten by the
// second. Before the fix, TamperCounters and PowerOutageFlags aliased the
// parser's reusable buffer.
func TestIDMRetainedMessages(t *testing.T) {
	const chipLength = 72

	type tx struct {
		serial         uint32
		tamper, outage [6]byte
	}
	txs := []tx{
		{11111111, [6]byte{1, 2, 3, 4, 5, 6}, [6]byte{0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6}},
		{22222222, [6]byte{9, 8, 7, 6, 5, 4}, [6]byte{0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6}},
	}

	// Sanity-check the CRC construction against the same check the parser uses.
	ccitt := crc.NewCRC("CCITT", 0xFFFF, 0x1021, 0x1D0F)
	if r := ccitt.Checksum(buildIDM(1, [6]byte{}, [6]byte{})[4:92]); r != ccitt.Residue {
		t.Fatalf("test packet CRC residue 0x%04X, want 0x%04X", r, ccitt.Residue)
	}

	var sig []byte
	sig = append(sig, silence(20000)...)
	for _, x := range txs {
		sig = append(sig, modulate(buildIDM(x.serial, x.tamper, x.outage), chipLength)...)
		sig = append(sig, silence(150000)...)
	}

	msgs := decodeStream(t, newDecoder(t, "idm", chipLength), bytes.NewReader(sig))

	// The same packet may be found at several adjacent offsets; keep the
	// first message seen for each serial, exactly as it was emitted.
	first := map[uint32]idm.IDM{}
	for _, m := range msgs {
		i := m.(idm.IDM)
		if _, ok := first[i.ERTSerialNumber]; !ok {
			first[i.ERTSerialNumber] = i
		}
	}

	for _, x := range txs {
		got, ok := first[x.serial]
		if !ok {
			t.Fatalf("serial %d not decoded (got %d messages)", x.serial, len(msgs))
		}
		if !bytes.Equal(got.TamperCounters, x.tamper[:]) {
			t.Errorf("serial %d TamperCounters = %X, want %X", x.serial, got.TamperCounters, x.tamper)
		}
		if !bytes.Equal(got.PowerOutageFlags, x.outage[:]) {
			t.Errorf("serial %d PowerOutageFlags = %X, want %X", x.serial, got.PowerOutageFlags, x.outage)
		}
	}
}

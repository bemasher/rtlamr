package crc

import (
	"encoding/binary"
	"testing"
	"time"

	crand "crypto/rand"
	mrand "math/rand"
)

const (
	Trials = 512
)

var crcs = []CRC{
	{"IBM", 0, 0x8005, 0, Table{}},
	{"BCH", 0, 0x6F63, 0, Table{}},
	{"CCITT", 0xFFFF, 0x1021, 0x1D0F, Table{}},
}

func TestIdentity(t *testing.T) {
	for _, crc := range crcs {
		t.Logf("%+v\n", crc)
		crc.tbl = NewTable(crc.Poly)
		for trial := 0; trial < Trials; trial++ {
			length := mrand.Intn(32)&0xFE + 8

			buf := make([]byte, length)
			crand.Read(buf[:length-2])

			intermediate := crc.Checksum(buf[:length-2])
			binary.BigEndian.PutUint16(buf[length-2:], intermediate)

			check := crc.Checksum(buf)
			if check != 0 {
				t.Fatalf("%s failed: %02X %04X %04X\n", crc.Name, buf, intermediate, check)
			}
		}
	}
}

func BenchmarkBCH(b *testing.B) {
	input := make([]byte, 16384)
	crand.Read(input)

	bch := NewCRC("BCH", 0, 0x6F63, 0)

	b.SetBytes(16384 >> 3)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		bch.Checksum(input)
	}
}

func BenchmarkCCITT(b *testing.B) {
	input := make([]byte, 16384)
	crand.Read(input)

	ccitt := NewCRC("CCITT", 0xFFFF, 0x1021, 0x1D0F)

	b.SetBytes(16384 >> 3)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		ccitt.Checksum(input)
	}
}

func init() {
	mrand.Seed(time.Now().UnixNano())
}

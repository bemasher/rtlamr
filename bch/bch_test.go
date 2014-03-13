package bch

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

const (
	GenPoly = 0x16F63
)

var bch = NewBCH(GenPoly)

func TestNOP(t *testing.T) {
	checksum := bch.Encode(strings.Repeat("0", 80))
	if checksum != 0 {
		t.Fatalf("Expected: %d Got: %d\n", 0, checksum)
	}
}

type BitString string

// Generate a random 64-bit bitstring and pad to 80 bits with zeros.
func (bs BitString) Generate(rand *rand.Rand, size int) reflect.Value {
	var bits string
	for i := 0; i < 64; i++ {
		if rand.NormFloat64() > 0.5 {
			bits += "1"
		} else {
			bits += "0"
		}
	}

	bits += strings.Repeat("0", 16)

	return reflect.ValueOf(BitString(bits))
}

// Encode a random bitstring with checksum of 0, replace checksum with
// calculated value and recalculate, result should be zero.
func TestIdentity(t *testing.T) {
	err := quick.Check(func(bs BitString) bool {
		bits := string(bs)

		checksum := bch.Encode(string(bits))
		bits = bits[:64] + fmt.Sprintf("%016b", checksum)

		checksum = bch.Encode(bits)

		return checksum == 0
	}, nil)

	if err != nil {
		t.Fatal("Error testing identity:", err)
	}
}

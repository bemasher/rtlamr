// Implements BCH error correction and detection.
package bch

import (
	"fmt"
)

// BCH Error Correction
type BCH struct {
	GenPoly uint
	PolyLen byte
}

// Given a generator polynomial, message length and number of errors to
// attempt to correct, calculate the polynomial length and pre-compute
// syndromes for number of errors to be corrected.
func NewBCH(poly uint) (bch BCH) {
	bch.GenPoly = poly

	p := bch.GenPoly
	for ; bch.PolyLen < 32 && p > 0; bch.PolyLen, p = bch.PolyLen+1, p>>1 {
	}
	bch.PolyLen--

	return
}

func (bch BCH) String() string {
	return fmt.Sprintf("{GenPoly:%X PolyLen:%d}", bch.GenPoly, bch.PolyLen)
}

// Syndrome calculation implemented using LSFR (linear feedback shift register).
func (bch BCH) Encode(bits string) (checksum uint) {
	// For each byte of data.
	for idx := range bits {
		// Rotate register and shift in bit.
		checksum <<= 1
		if bits[idx] == '1' {
			checksum |= 1
		}
		// If MSB of register is non-zero XOR with generator polynomial.
		if checksum>>bch.PolyLen != 0 {
			checksum ^= bch.GenPoly
		}
	}

	// Mask to valid length
	checksum &= (1 << bch.PolyLen) - 1
	return
}

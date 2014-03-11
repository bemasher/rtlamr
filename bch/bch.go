// Implements BCH error correction and detection.
package bch

import (
	"fmt"
)

// BCH Error Correction
type BCH struct {
	GenPoly   uint
	PolyLen   byte
	Syndromes map[uint][]uint
}

// Given a generator polynomial, message length and number of errors to
// attempt to correct, calculate the polynomial length and pre-compute
// syndromes for number of errors to be corrected.
func NewBCH(poly uint, msgLen, errorCount uint) (bch BCH) {
	bch.GenPoly = poly

	p := bch.GenPoly
	for ; bch.PolyLen < 32 && p > 0; bch.PolyLen, p = bch.PolyLen+1, p>>1 {
	}
	bch.PolyLen--

	bch.ComputeSyndromes(msgLen, errorCount)

	return
}

func (bch BCH) String() string {
	return fmt.Sprintf("{GenPoly:%X PolyLen:%d Syndromes:%d}", bch.GenPoly, bch.PolyLen, len(bch.Syndromes))
}

// Recursively computes syndromes for number of desired errors.
func (bch *BCH) ComputeSyndromes(msgLen, errCount uint) {
	bch.Syndromes = make(map[uint][]uint)

	data := make([]byte, msgLen)
	bch.computeHelper(msgLen, errCount, nil, data)
}

func (bch *BCH) computeHelper(msgLen, depth uint, prefix []uint, data []byte) {
	if depth == 0 {
		return
	}

	// For all possible bit positions.
	for i := uint(0); i < msgLen<<3; i++ {
		inPrefix := false
		for p := uint(0); p < uint(len(prefix)) && !inPrefix; p++ {
			inPrefix = i == prefix[p]
		}
		if inPrefix {
			continue
		}

		// Toggle the bit
		data[i>>3] ^= 1 << uint(i%8)

		// Calculate the syndrome and store with position if new.
		syn := bch.Encode(data)
		if _, exists := bch.Syndromes[syn]; !exists {
			bch.Syndromes[syn] = append(prefix, i)
		}

		// Recurse.
		bch.computeHelper(msgLen, depth-1, append(prefix, i), data)

		data[i>>3] ^= 1 << uint(i%8)
	}
}

// Syndrome calculation implemented using LSFR (linear feedback shift register).
func (bch BCH) Encode(data []byte) (checksum uint) {
	// For each byte of data.
	for _, b := range data {
		// For each bit of byte.
		for i := byte(0); i < 8; i++ {
			// Rotate register and shift in bit.
			checksum = (checksum << 1) | uint((b>>(7-i))&1)
			// If MSB of register is non-zero XOR with generator polynomial.
			if checksum>>bch.PolyLen != 0 {
				checksum ^= bch.GenPoly
			}
		}
	}

	// Mask to valid length
	checksum &= (1 << bch.PolyLen) - 1
	return
}

// Given data, calculate the syndrome and correct errors if syndrome exists in
// pre-computed syndromes.
func (bch BCH) Correct(data []byte) (checksum uint, corrected bool) {
	// Calculate syndrome.
	syn := bch.Encode(data)
	if syn == 0 {
		return syn, false
	}

	// If the syndrome exists then toggle bits the syndrome was
	// calculated from.
	if pos, exists := bch.Syndromes[syn]; exists {
		for _, b := range pos {
			data[b>>3] ^= 1 << uint(b%8)
		}
	}

	// Calculate syndrome of corrected version. If we corrected anything, indicate so.
	checksum = bch.Encode(data)
	if syn != checksum && checksum == 0 {
		corrected = true
	}

	return
}

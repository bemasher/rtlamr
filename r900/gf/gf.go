// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gf implements arithmetic over Galois Fields. Generalized for any valid order.
package gf

import "strconv"

// A Field represents an instance of GF(order) defined by a specific polynomial.
type Field struct {
	order int
	log   []byte // log[0] is unused
	exp   []byte
}

// NewField returns a new field corresponding to the polynomial poly and
// generator α. The choice of generator α only affects the Exp and Log
// operations.
func NewField(order, poly, α int) *Field {
	if order < 0 || order > 256 {
		panic("gf: invalid order: " + strconv.Itoa(order))
	}

	if poly < order || poly >= order<<1 || reducible(poly) {
		panic("gf: invalid polynomial: " + strconv.Itoa(poly))
	}

	f := Field{order - 1, make([]byte, order), make([]byte, (order-1)<<1)}
	x := 1
	for i := 0; i < f.order; i++ {
		if x == 1 && i != 0 {
			panic("gf: invalid generator " + strconv.Itoa(α) +
				" for polynomial " + strconv.Itoa(poly))
		}
		f.exp[i] = byte(x)
		f.exp[i+f.order] = byte(x)
		f.log[x] = byte(i)
		x = mul(x, α, order, poly)
	}
	f.log[0] = byte(f.order)
	for i := 0; i < f.order; i++ {
		if f.log[f.exp[i]] != byte(i) {
			panic("bad log")
		}
		if f.log[f.exp[i+f.order]] != byte(i) {
			panic("bad log")
		}
	}
	for i := 1; i < order; i++ {
		if f.exp[f.log[i]] != byte(i) {
			panic("bad log")
		}
	}

	return &f
}

// nbit returns the number of significant in p.
func nbit(p int) uint {
	n := uint(0)
	for ; p > 0; p >>= 1 {
		n++
	}
	return n
}

// polyDiv divides the polynomial p by q and returns the remainder.
func polyDiv(p, q int) int {
	np := nbit(p)
	nq := nbit(q)
	for ; np >= nq; np-- {
		if p&(1<<(np-1)) != 0 {
			p ^= q << (np - nq)
		}
	}
	return p
}

// mul returns the product x*y mod poly, a GF(order) multiplication.
func mul(x, y, order, poly int) int {
	z := 0
	for x > 0 {
		if x&1 != 0 {
			z ^= y
		}
		x >>= 1
		y <<= 1
		if y&order != 0 {
			y ^= poly
		}
	}
	return z
}

// reducible reports whether p is reducible.
func reducible(p int) bool {
	// Multiplying n-bit * n-bit produces (2n-1)-bit,
	// so if p is reducible, one of its factors must be
	// of np/2+1 bits or fewer.
	np := nbit(p)
	for q := 2; q < 1<<(np/2+1); q++ {
		if polyDiv(p, q) == 0 {
			return true
		}
	}
	return false
}

// Add returns the sum of x and y in the field.
func (f *Field) Add(x, y byte) byte {
	return x ^ y
}

// Exp returns the base-α exponential of e in the field.
// If e < 0, Exp returns 0.
func (f *Field) Exp(e int) byte {
	if e < 0 {
		return 0
	}
	return f.exp[e%f.order]
}

// Log returns the base-α logarithm of x in the field.
// If x == 0, Log returns -1.
func (f *Field) Log(x byte) int {
	if x == 0 {
		return -1
	}
	return int(f.log[x])
}

// Inv returns the multiplicative inverse of x in the field.
// If x == 0, Inv returns 0.
func (f *Field) Inv(x byte) byte {
	if x == 0 {
		return 0
	}
	return f.exp[f.order-int(f.log[x])]
}

// Mul returns the product of x and y in the field.
func (f *Field) Mul(x, y byte) byte {
	if x == 0 || y == 0 {
		return 0
	}
	return f.exp[int(f.log[x])+int(f.log[y])]
}

// Calculate syndrome for a message encoded using the field generated for a
// particular Reed-Solomon polynomial. Offset defines the coefficient offset.
func (f *Field) Syndrome(message []byte, paritySymbolCount, offset int) (syndrome []byte) {
	if offset < 0 || offset > f.order {
		panic("gf: invalid offset: " + strconv.Itoa(offset))
	}

	if paritySymbolCount < 0 || paritySymbolCount > len(message) {
		panic("gf: invalid paritySymbolCount: " + strconv.Itoa(paritySymbolCount))
	}

	syndrome = make([]byte, paritySymbolCount)

	for idx, syn := range syndrome {
		syn = message[0]
		for _, v := range message[1:] {
			syn = f.Mul(syn, f.Exp(offset+idx)) ^ v
		}
		syndrome[idx] = syn
	}

	return syndrome
}

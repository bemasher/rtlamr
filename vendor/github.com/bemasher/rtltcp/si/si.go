package si

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const (
	Y = 1e24
	Z = 1e21
	E = 1e18
	P = 1e15
	T = 1e12
	G = 1e9
	M = 1e6
	k = 1e3
	m = 1e-3
	u = 1e-6
	n = 1e-9
	p = 1e-12
	f = 1e-15
	a = 1e-18
	z = 1e-21
	y = 1e-24
)

type ScientificNotation float64

func (si ScientificNotation) String() (s string) {
	return strconv.FormatFloat(float64(si), 'g', -1, 64)
}

func (si *ScientificNotation) Set(value string) error {
	mantissaStr := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) || r == '.' || r == '-' {
			return r
		}
		return -1
	}, value)

	suffix := strings.Map(func(r rune) rune {
		switch r {
		case 'Y', 'Z', 'E', 'P', 'T', 'G', 'M', 'k', 'm', 'u', 'n', 'p', 'f', 'a', 'z', 'y':
			return r
		}
		return -1
	}, value)

	if len(suffix) > 1 {
		return fmt.Errorf("suffix too long: %q", suffix)
	}

	mantissa, err := strconv.ParseFloat(mantissaStr, 64)
	if err != nil {
		return err
	}

	*si = ScientificNotation(mantissa)

	if len(suffix) > 0 {
		switch suffix[0] {
		case 'Y':
			*si *= Y
		case 'Z':
			*si *= Z
		case 'E':
			*si *= E
		case 'P':
			*si *= P
		case 'T':
			*si *= T
		case 'G':
			*si *= G
		case 'M':
			*si *= M
		case 'k':
			*si *= k
		case 'm':
			*si *= m
		case 'u':
			*si *= u
		case 'n':
			*si *= n
		case 'p':
			*si *= p
		case 'f':
			*si *= f
		case 'a':
			*si *= a
		case 'z':
			*si *= z
		case 'y':
			*si *= y
		}
	}

	return nil
}

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
	// Trim all non-digits from the right, to recover prefix.
	numStr := strings.TrimRightFunc(value, func(r rune) bool {
		return !unicode.IsDigit(r)
	})

	// Recover the suffix.
	suffix := strings.TrimPrefix(value, numStr)

	// Parse num.
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return fmt.Errorf("strconv.ParseFloat: %w", err)
	}

	// Set value.
	*si = ScientificNotation(num)

	// Apply suffix.
	for i, s := range suffix {
		// Ignore everything after first suffix.
		if i > 0 {
			return nil
		}

		switch s {
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
		default:
			return fmt.Errorf("invalid suffix: %q", suffix)
		}
	}

	return nil
}

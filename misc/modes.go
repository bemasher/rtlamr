// Calculates:
// Valid symbol lengths.
// Sample rate to achieve the symbol length.
// Number of channels covered.
// Excess bandwidth outside of the captured channels.

package main

import (
	"fmt"
	"math"
)

const (
	DataRate     = 32768
	ChannelWidth = 196568

	// Valid sample rates fall in one of two bands:
	// http://cgit.osmocom.org/rtl-sdr/tree/src/librtlsdr.c#n1069
	LowerMin = 225e3
	LowerMax = 300e3
	UpperMin = 900e3
	UpperMax = 3.2e6
)

func main() {
	for symbolLength := 1; symbolLength < int(math.Ceil(UpperMax/DataRate)); symbolLength++ {
		sampleRate := symbolLength * DataRate
		if (LowerMin < sampleRate && sampleRate <= LowerMax) || (UpperMin < sampleRate && sampleRate <= UpperMax) {
			fmt.Printf("SymbolLength:%d SampleRate:%d Channels:%d ExcessBandwidth:%d\n", symbolLength, sampleRate, sampleRate/ChannelWidth, sampleRate%ChannelWidth)
		}
	}
}

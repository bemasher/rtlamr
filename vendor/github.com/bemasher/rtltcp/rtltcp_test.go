package rtltcp

import (
	"fmt"
	"io"
	"log"
	"net"
)

func Example_sDR() {
	var sdr SDR

	// Resolve address, this may be ip:port or hostname:port.
	addr, err := net.ResolveTCPAddr("tcp4", "127.0.0.1:1234")
	if err != nil {
		log.Fatal("Error resolving address:", err)
	}

	// Connect to address and defer close.
	sdr.Connect(addr)
	defer sdr.Close()

	// Print dongle info.
	fmt.Printf("%+v\n", sdr.Info)
	// Example: {Magic:"RTL0" Tuner:R820T GainCount:29}

	// Create an array of bytes for samples.
	buf := make([]byte, 16384)

	// Read the entire array. This is usually done in a loop.
	_, err = io.ReadFull(sdr, buf)
	if err != nil {
		log.Fatal("Error reading samples:", err)
	}

	// Do something with data in buf...

}

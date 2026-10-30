package main

import (
	"context"
	"encoding/binary"
	"flag"
	"io"
	"net"
	"sync"
	"testing"
)

// Flags bind to the package-level rcvr (as in main) and can only be
// registered once per test binary.
var registerFlagsOnce sync.Once

// fakeRTLTCP accepts one connection, sends an rtl_tcp dongle header, and
// returns every 5-byte command the client sends until it disconnects.
func fakeRTLTCP(t *testing.T) (addr string, cmds <-chan map[uint8]uint32) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan map[uint8]uint32, 1)
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			ch <- nil
			return
		}
		defer conn.Close()

		// Magic "RTL0", tuner R820T (5), 29 gain steps.
		hdr := []byte{'R', 'T', 'L', '0', 0, 0, 0, 5, 0, 0, 0, 29}
		conn.Write(hdr)

		got := map[uint8]uint32{}
		buf := make([]byte, 5)
		for {
			if _, err := io.ReadFull(conn, buf); err != nil {
				ch <- got
				return
			}
			got[buf[0]] = binary.BigEndian.Uint32(buf[1:])
		}
	}()

	return ln.Addr().String(), ch
}

// TestRTLTCPFlagsReachServer checks that rtltcp-specific flags are actually
// sent to rtl_tcp. Previously HandleFlags ran before Connect and the
// commands were silently dropped.
func TestRTLTCPFlagsReachServer(t *testing.T) {
	addr, cmds := fakeRTLTCP(t)

	registerFlagsOnce.Do(func() {
		rcvr.RegisterFlags()
		RegisterFlags()
	})

	for name, val := range map[string]string{
		"server":         addr,
		"tunergain":      "40.2",
		"freqcorrection": "57",
		"agcmode":        "true",
	} {
		if err := flag.Set(name, val); err != nil {
			t.Fatal(err)
		}
	}
	HandleFlags()

	rcvr.NewReceiver(context.Background())
	if err := context.Cause(rcvr.ctx); err != nil {
		t.Fatal(err)
	}
	rcvr.SDR.Close()

	got := <-cmds
	if got == nil {
		t.Fatal("fake rtl_tcp never received a connection")
	}

	// Command IDs from rtl_tcp.c.
	const (
		cmdTunerGain      = 4
		cmdFreqCorrection = 5
		cmdAGCMode        = 8
	)
	want := map[uint8]uint32{
		cmdTunerGain:      402,
		cmdFreqCorrection: 57,
		cmdAGCMode:        1,
	}
	for cmd, val := range want {
		if v, ok := got[cmd]; !ok {
			t.Errorf("command %d never sent to rtl_tcp", cmd)
		} else if v != val {
			t.Errorf("command %d = %d, want %d", cmd, v, val)
		}
	}
}

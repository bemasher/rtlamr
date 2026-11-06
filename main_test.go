package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/bemasher/rtltcp"
)

func TestFlagServer(t *testing.T) {
	// Test spectrum server address:port other than default.
	addrStr := "127.0.0.1:1235"

	// Resolve address:port.
	addr, err := net.ResolveTCPAddr("tcp", addrStr)
	if err != nil {
		t.Fatalf("net.ResolveTCPAddr: %s", err)
	}

	// New mock server with random samples.
	sdr := rtltcp.NewMockServer(addr)
	sdr.Samples = []byte{0x00, 0x01, 0xfe, 0xff}

	// Start mock server.
	if err := sdr.Start(); err != nil {
		t.Fatal("sdr.Start:", err)
	}
	defer sdr.Close()

	// os.Args are per execution, and multiple tests share an executable.
	// Make a copy so we can put things back the way we found them.
	// This feels like a dirty hack.
	args := append([]string(nil), os.Args...)

	// Append -server flag.
	os.Args = append(os.Args, "-server="+addrStr)
	defer func() {
		os.Args = args
	}()

	// Run server long enough to establish connection.
	ctx, canc := context.WithTimeout(context.Background(), time.Second)
	defer canc()

	// Check exit value.
	exit := run(ctx)
	if exit > 0 {
		t.Fatal("exit:", exit)
	}
}

func TestPprofEndpoint(t *testing.T) {
	addr, stop, err := startPprof("127.0.0.1:0")
	if err != nil {
		t.Fatal("startPprof:", err)
	}
	defer stop()

	response, err := http.Get("http://" + addr.String() + "/debug/pprof/")
	if err != nil {
		t.Fatal("GET /debug/pprof/:", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /debug/pprof/: status %d", response.StatusCode)
	}
}

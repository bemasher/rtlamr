package main

import (
	"encoding/binary"
	"flag"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

type gainControlCommand struct {
	opcode byte
	value  uint32
}

// captureGainControlCommands uses a local mock TCP peer; no SDR or rtl_tcp
// service is needed. Each command on the wire is one opcode and a big-endian
// uint32 parameter.
func captureGainControlCommands(t *testing.T, args ...string) ([]gainControlCommand, error) {
	t.Helper()

	originalFlags := flag.CommandLine
	fs := flag.NewFlagSet("gain-control-test", flag.ContinueOnError)
	flag.CommandLine = fs
	defer func() { flag.CommandLine = originalFlags }()

	rcvr := &Receiver{}
	rcvr.RegisterFlags()
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	commandsCh := make(chan []gainControlCommand, 1)
	errorsCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errorsCh <- err
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		var cmds []gainControlCommand
		for {
			var raw [5]byte
			_, err := io.ReadFull(conn, raw[:])
			if err == io.EOF {
				commandsCh <- cmds
				return
			}
			if err != nil {
				errorsCh <- err
				return
			}
			cmds = append(cmds, gainControlCommand{
				opcode: raw[0],
				value:  binary.BigEndian.Uint32(raw[1:]),
			})
		}
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("expected TCP address")
	}
	client, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	rcvr.SDR.TCPConn = client
	rcvr.SDR.Info.GainCount = 29
	applyErr := rcvr.HandleFlags()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case commands := <-commandsCh:
		return commands, applyErr
	case err := <-errorsCh:
		t.Fatal(err)
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for mock rtl_tcp commands")
	}
	return nil, applyErr
}

// TestGainCommandOrdering pins the accepted upstream gain-mode semantics and
// checks the required mode-before-gain command sequence over a mock TCP peer.
func TestGainCommandOrdering(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []gainControlCommand
	}{
		{
			name: "manual numeric gain with RTL AGC off",
			args: []string{"-tunergainmode=true", "-tunergain=43.9", "-agcmode=false"},
			want: []gainControlCommand{{8, 0}, {3, 1}, {4, 439}},
		},
		{
			name: "manual numeric gain with reverse command-line flag order",
			args: []string{"-tunergain=25.4", "-tunergainmode=true"},
			want: []gainControlCommand{{3, 1}, {4, 254}},
		},
		{
			name: "manual gain index follows mode",
			args: []string{"-gainbyindex=5", "-tunergainmode=true"},
			want: []gainControlCommand{{3, 1}, {13, 5}},
		},
		{
			name: "numeric gain alone retains existing behavior",
			args: []string{"-tunergain=7.7"},
			want: []gainControlCommand{{4, 77}},
		},
		{
			name: "gain index alone retains existing behavior",
			args: []string{"-gainbyindex=5"},
			want: []gainControlCommand{{13, 5}},
		},
		{
			name: "explicit automatic mode unchanged",
			args: []string{"-tunergainmode=false"},
			want: []gainControlCommand{{3, 0}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := captureGainControlCommands(t, tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("rtl_tcp commands: got %#v, want %#v", got, tt.want)
			}
		})
	}
}

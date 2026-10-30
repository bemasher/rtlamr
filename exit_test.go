package main

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// TestExitError checks which receiver stops count as failures (exit 1) and
// which are normal (exit 0).
func TestExitError(t *testing.T) {
	errRead := errors.New("rcvr.Read: connection reset by peer")

	cases := []struct {
		name string
		// stop cancels the main and/or receiver context the way main/Run would.
		stop func(mainCanc context.CancelFunc, rcvrCanc context.CancelCauseFunc)
		want error
	}{
		{"receiver error", func(_ context.CancelFunc, r context.CancelCauseFunc) { r(errRead) }, errRead},
		{"interrupt", func(m context.CancelFunc, _ context.CancelCauseFunc) { m() }, nil},
		{"single finished", func(_ context.CancelFunc, r context.CancelCauseFunc) { r(errSingleDone) }, nil},
		{"receiver closed normally", func(_ context.CancelFunc, r context.CancelCauseFunc) { r(nil) }, nil},
		{"receiver error, then interrupt", func(m context.CancelFunc, r context.CancelCauseFunc) { r(errRead); m() }, errRead},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mainCtx, mainCanc := context.WithCancel(context.Background())
			defer mainCanc()
			rcvrCtx, rcvrCanc := context.WithCancelCause(mainCtx)
			defer rcvrCanc(nil)

			c.stop(mainCanc, rcvrCanc)
			if got := exitError(mainCtx, rcvrCtx); got != c.want {
				t.Fatalf("exitError() = %v, want %v", got, c.want)
			}
		})
	}

	t.Run("duration elapsed", func(t *testing.T) {
		mainCtx, mainCanc := context.WithTimeout(context.Background(), time.Millisecond)
		defer mainCanc()
		rcvrCtx, rcvrCanc := context.WithCancelCause(mainCtx)
		defer rcvrCanc(nil)

		<-rcvrCtx.Done()
		if got := exitError(mainCtx, rcvrCtx); got != nil {
			t.Fatalf("exitError() = %v, want nil", got)
		}
	})
}

// TestReceiverStopsWithErrorOnDisconnect runs a real Receiver against a fake
// rtl_tcp that hangs up mid-stream and checks exitError reports it.
func TestReceiverStopsWithErrorOnDisconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Write([]byte{'R', 'T', 'L', '0', 0, 0, 0, 5, 0, 0, 0, 29}) // dongle header
		go io.Copy(io.Discard, conn)                                    // swallow tuning commands
		time.Sleep(100 * time.Millisecond)
		conn.Write(make([]byte, 1<<20)) // some silence, then hang up
		conn.Close()
	}()

	// Set package state directly rather than registering flags, so this test
	// doesn't collide with other tests that register them.
	oldTypes := msgType
	msgType = StringMap{"scm": true}
	defer func() { msgType = oldTypes }()

	mainCtx, mainCanc := context.WithCancel(context.Background())
	defer mainCanc()

	var r Receiver
	r.Flags.ServerAddr = ln.Addr().String()
	r.NewReceiver(mainCtx)
	if err := context.Cause(r.ctx); err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	r.Run()

	select {
	case <-r.ctx.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("receiver did not stop after rtl_tcp hung up")
	}
	r.Close()

	if err := exitError(mainCtx, r.ctx); err == nil {
		t.Fatal("exitError() = nil after rtl_tcp hung up, want the read error")
	} else {
		t.Logf("exitError() = %v", err)
	}
}

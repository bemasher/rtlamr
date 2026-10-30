package main

import (
	"context"
	"strings"
	"testing"
)

// TestNewReceiverInvalidMsgType checks that an unknown message type cancels
// the receiver with a descriptive cause instead of registering a nil parser
// (which used to panic in RegisterProtocol).
func TestNewReceiverInvalidMsgType(t *testing.T) {
	old := msgType
	msgType = StringMap{"bogus": true}
	defer func() { msgType = old }()

	var r Receiver
	r.NewReceiver(context.Background())

	err := context.Cause(r.ctx)
	if err == nil {
		t.Fatal("expected receiver context to be cancelled")
	}
	if !strings.Contains(err.Error(), `invalid message type: "bogus"`) {
		t.Fatalf("unexpected cause: %v", err)
	}
}

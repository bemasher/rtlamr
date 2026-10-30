package csv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"runtime"
	"testing"
)

func TestRecorderNil(t *testing.T) {
	buf := &bytes.Buffer{}
	enc := Encoder{csv.NewWriter(buf)}

	if err := enc.Encode(nil); err == nil {
		t.Fatalf("%+v\n", err)
	}
}

type Msg struct{}

func (m Msg) Record() []string {
	return []string{}
}

func TestRecorder(t *testing.T) {
	buf := &bytes.Buffer{}
	enc := Encoder{csv.NewWriter(buf)}

	if err := enc.Encode(Msg{}); err != nil {
		t.Fatalf("%+v\n", err)
	}
}

type NonRecorder struct{}

func TestNonRecorder(t *testing.T) {
	buf := &bytes.Buffer{}
	enc := Encoder{csv.NewWriter(buf)}

	err := enc.Encode(NonRecorder{})

	var runtimeErr runtime.Error
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("%+v\n", runtimeErr)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errWrite }

var errWrite = errors.New("write failed")

// TestWriteError checks that an error writing to the underlying stream is
// returned. Previously Encode always returned nil on a non-panicking path.
func TestWriteError(t *testing.T) {
	enc := NewEncoder(failWriter{})

	if err := enc.Encode(Msg{}); !errors.Is(err, errWrite) {
		t.Fatalf("Encode() = %v, want %v", err, errWrite)
	}
}

type panicRecorder struct{}

func (panicRecorder) Record() []string { panic("not an error value") }

// TestNonErrorPanic checks that a panic with a non-error value is still
// reported rather than silently swallowed.
func TestNonErrorPanic(t *testing.T) {
	enc := NewEncoder(&bytes.Buffer{})

	if err := enc.Encode(panicRecorder{}); err == nil {
		t.Fatal("Encode() = nil, want an error")
	}
}

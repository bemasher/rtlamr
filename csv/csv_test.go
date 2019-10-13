package csv

import (
	"bytes"
	"encoding/csv"
	"runtime"
	"testing"

	"golang.org/x/xerrors"
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
	if !xerrors.As(err, &runtimeErr) {
		t.Fatalf("%+v\n", runtimeErr)
	}
}

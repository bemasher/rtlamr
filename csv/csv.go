package csv

import (
	"encoding/csv"
	"io"

	"golang.org/x/xerrors"
)

// Produces a list of fields making up a record.
type Recorder interface {
	Record() []string
}

// An Encoder writes CSV records to an output stream.
type Encoder struct {
	w *csv.Writer
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: csv.NewWriter(w)}
}

// Encode writes a CSV record representing v to the stream followed by a
// newline character. Value given must implement the Recorder interface.
func (enc *Encoder) Encode(v interface{}) (err error) {
	defer func() {
		if err, _ = recover().(error); err != nil {
			err = xerrors.Errorf("recovered: %w", err)
		}
	}()

	err = enc.w.Write(v.(Recorder).Record())
	enc.w.Flush()

	return nil
}

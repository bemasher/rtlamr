package csv

import (
	"encoding/csv"
	"errors"
	"io"
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
	record, ok := v.(Recorder)
	if !ok {
		return errors.New("value does not satisfy Recorder interface")
	}

	err = enc.w.Write(record.Record())
	enc.w.Flush()

	return nil
}

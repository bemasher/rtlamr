package csv

import (
	"encoding/csv"
	"fmt"
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
	defer func() {
		// Only override err if we actually recovered from a panic; otherwise
		// keep whatever error the write returned.
		switch r := recover().(type) {
		case nil:
		case error:
			err = fmt.Errorf("recovered: %w", r)
		default:
			err = fmt.Errorf("recovered: %v", r)
		}
	}()

	if err = enc.w.Write(v.(Recorder).Record()); err != nil {
		return err
	}
	enc.w.Flush()

	return enc.w.Error()
}

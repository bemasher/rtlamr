package parse

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bemasher/rtlamr/decode"

	"github.com/bemasher/rtlamr/csv"
)

const (
	TimeFormat = "2006-01-02T15:04:05.000"
)

type Data struct {
	Bits  string
	Bytes []byte
}

func NewDataFromBytes(data []byte) (d Data) {
	d.Bytes = data
	for _, b := range data {
		d.Bits += fmt.Sprintf("%08b", b)
	}

	return
}

func NewDataFromBits(data string) (d Data) {
	d.Bits = data
	d.Bytes = make([]byte, (len(data)+7)>>3)
	for idx := 0; idx < len(data); idx += 8 {
		b, _ := strconv.ParseUint(d.Bits[idx:idx+8], 2, 8)
		d.Bytes[idx>>3] = uint8(b)
	}
	return
}

type Parser interface {
	Parse([]int) []Message
	Dec() decode.Decoder
	Cfg() decode.PacketConfig
	Log()
}

type Message interface {
	csv.Recorder
	MsgType() string
	MeterID() uint32
	MeterType() uint8
	Checksum() []byte
}

type LogMessage struct {
	Time   time.Time
	Offset int64
	Length int
	Message
}

func (msg LogMessage) String() string {
	return fmt.Sprintf("{Time:%s Offset:%d Length:%d %s:%s}",
		msg.Time.Format(TimeFormat), msg.Offset, msg.Length, msg.MsgType(), msg.Message,
	)
}

func (msg LogMessage) StringNoOffset() string {
	return fmt.Sprintf("{Time:%s %s:%s}", msg.Time.Format(TimeFormat), msg.MsgType(), msg.Message)
}

func (msg LogMessage) Record() (r []string) {
	r = append(r, msg.Time.Format(time.RFC3339Nano))
	r = append(r, strconv.FormatInt(msg.Offset, 10))
	r = append(r, strconv.FormatInt(int64(msg.Length), 10))
	r = append(r, msg.Message.Record()...)
	return r
}

type FilterChain []Filter

func (fc *FilterChain) Add(filter Filter) {
	*fc = append(*fc, filter)
}

func (fc FilterChain) Match(msg Message) bool {
	if len(fc) == 0 {
		return true
	}

	for _, filter := range fc {
		if !filter.Filter(msg) {
			return false
		}
	}

	return true
}

type Filter interface {
	Filter(Message) bool
}

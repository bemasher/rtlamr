package parse

import (
	"fmt"
	"strconv"
	"time"

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
	d.Bytes = make([]byte, len(data)>>3+1)
	for idx := 0; idx < len(data); idx += 8 {
		b, _ := strconv.ParseUint(d.Bits[idx:idx+8], 2, 8)
		d.Bytes[idx>>3] = uint8(b)
	}
	return
}

type Parser interface {
	Parse(Data) (Message, error)
}

type Message interface {
	MsgType() string
	MeterID() uint32
	MeterType() uint8
	csv.Recorder
}

type Logger interface {
	fmt.Stringer
	csv.Recorder
	StringNoOffset() string
}

type HopMessage struct {
	Time          time.Time
	ID            uint32
	Type          uint8
	CenterChannel int
	OffsetChannel int
}

func (msg HopMessage) String() string {
	return fmt.Sprintf("{Time:%s ID:%d Type:%d CenterChannel:%d OffsetChannel:%d}",
		msg.Time.Format(TimeFormat), msg.ID, msg.Type, msg.CenterChannel, msg.OffsetChannel,
	)
}

func (msg HopMessage) StringNoOffset() string {
	return msg.String()
}

func (msg HopMessage) Record() (r []string) {
	r = append(r, msg.Time.Format(time.RFC3339Nano))
	r = append(r, strconv.FormatInt(int64(msg.ID), 10))
	r = append(r, strconv.FormatInt(int64(msg.Type), 10))
	r = append(r, strconv.FormatInt(int64(msg.CenterChannel), 10))
	r = append(r, strconv.FormatInt(int64(msg.OffsetChannel), 10))
	return r
}

type LogMessage struct {
	Time    time.Time
	Offset  int64
	Length  int
	Channel int
	Message
}

func (msg LogMessage) String() string {
	return fmt.Sprintf("{Time:%s Offset:%d Length:%d Channel:%d %s:%s}",
		msg.Time.Format(TimeFormat), msg.Offset, msg.Length, msg.Channel, msg.MsgType(), msg.Message,
	)
}

func (msg LogMessage) StringNoOffset() string {
	return fmt.Sprintf("{Time:%s Channel:%d %s:%s}",
		msg.Time.Format(TimeFormat), msg.Channel, msg.MsgType(), msg.Message,
	)
}

func (msg LogMessage) Record() (r []string) {
	r = append(r, msg.Time.Format(time.RFC3339Nano))
	r = append(r, strconv.FormatInt(msg.Offset, 10))
	r = append(r, strconv.FormatInt(int64(msg.Length), 10))
	r = append(r, msg.Message.Record()...)
	return r
}

package protocol

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bemasher/rtlamr/csv"
)

const (
	TimeFormat = "2006-01-02T15:04:05.000"
)

var (
	parserMutex sync.Mutex
	parsers     = make(map[string]NewParserFunc)
)

type NewParserFunc func(symbolLength int) Parser

// Given a name and a parser, register a parser for use.
// Later used by underscore improting each parser package:
//
// import _ "github.com/bemasher/rtlamr/scm"
//
func RegisterParser(name string, parserFn NewParserFunc) {
	parserMutex.Lock()
	defer parserMutex.Unlock()

	if parserFn == nil {
		panic("parser: new parser func is nil")
	}
	if _, dup := parsers[name]; dup {
		panic(fmt.Sprintf("parser: parser already registered (%s)", name))
	}
	parsers[name] = parserFn
}

// Given a name and symbolLength, lookup the parser and make a new one.
func NewParser(name string, symbolLength int) (Parser, error) {
	parserMutex.Lock()
	defer parserMutex.Unlock()

	if parserFn, exists := parsers[name]; exists {
		return parserFn(symbolLength), nil
	} else {
		return nil, fmt.Errorf("invalid message type: %q\n", name)
	}
}

// Used by parsers to interpret received bits/bytes
// into their appropriate fields.
type Data struct {
	Idx   int
	Bits  string
	Bytes []byte
}

func NewData(data []byte) (d Data) {
	d.Bytes = make([]byte, len(data))
	copy(d.Bytes, data)
	for _, b := range data {
		d.Bits += fmt.Sprintf("%08b", b)
	}

	return
}

// A Parser converts slices of bytes to messages.
type Parser interface {
	Parse([]Data, chan Message, *sync.WaitGroup)
	SetDecoder(*Decoder)
	Cfg() PacketConfig
}

type Message interface {
	csv.Recorder
	MsgType() string
	MeterID() uint32
	MeterType() uint8
	Checksum() []byte
}

// Uniquely identifies a message spanning two sample blocks.
type Digest struct {
	MsgType   string
	MeterType uint8
	MeterID   uint32
	Checksum  string
}

func NewDigest(msg Message) Digest {
	return Digest{
		msg.MsgType(),
		msg.MeterType(),
		msg.MeterID(),
		string(msg.Checksum()),
	}
}

// A LogMessage associates a message with a point in time and an offset and
// length into a binary sample file.
type LogMessage struct {
	Time   time.Time `xml:",attr"`
	Offset int64     `xml:",attr"`
	Length int       `xml:",attr"`
	Type   string    `xml:",attr"`
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

// A FilterChain takse a list of filters and applies them iteratively to
// messages sent through the chain.
type FilterChain []MessageFilter

func (fc *FilterChain) Add(filter MessageFilter) {
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

type MessageFilter interface {
	Filter(Message) bool
}

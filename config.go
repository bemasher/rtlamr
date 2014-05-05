package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"encoding/gob"
	"encoding/json"
	"encoding/xml"
)

type Config struct {
	serverAddr     string
	logFilename    string
	sampleFilename string
	format         string

	ServerAddr *net.TCPAddr
	CenterFreq uint
	TimeLimit  time.Duration
	MeterID    uint

	Log     *log.Logger
	LogFile *os.File

	GobUnsafe bool
	Encoder   Encoder

	SampleFile *os.File

	Quiet  bool
	Single bool
}

func (c *Config) Parse() (err error) {
	flag.StringVar(&c.serverAddr, "server", "127.0.0.1:1234", "address or hostname of rtl_tcp instance")
	flag.StringVar(&c.logFilename, "logfile", "/dev/stdout", "log statement dump file")
	flag.StringVar(&c.sampleFilename, "samplefile", os.DevNull, "received message signal dump file, offset and message length are displayed to log when enabled")
	flag.UintVar(&c.CenterFreq, "centerfreq", 920299072, "center frequency to receive on")
	flag.DurationVar(&c.TimeLimit, "duration", 0, "time to run for, 0 for infinite")
	flag.UintVar(&c.MeterID, "filterid", 0, "display only messages matching given id")
	flag.StringVar(&c.format, "format", "plain", "format to write log messages in: plain, json, xml or gob")
	flag.BoolVar(&c.GobUnsafe, "gobunsafe", false, "allow gob output to stdout")
	flag.BoolVar(&c.Quiet, "quiet", false, "suppress state information printed at startup")
	flag.BoolVar(&c.Single, "single", false, "provides one shot execution, listens until exactly one message is recieved")

	flag.Parse()

	// Parse and resolve rtl_tcp server address.
	c.ServerAddr, err = net.ResolveTCPAddr("tcp", c.serverAddr)
	if err != nil {
		return
	}

	// Open or create the log file.
	if c.logFilename == "/dev/stdout" {
		c.LogFile = os.Stdout
	} else {
		c.LogFile, err = os.Create(c.logFilename)
	}

	// Create a new logger with the log file as output.
	c.Log = log.New(c.LogFile, "", log.Ldate|log.Lmicroseconds)
	if err != nil {
		return
	}

	// Create the sample file.
	c.SampleFile, err = os.Create(c.sampleFilename)
	if err != nil {
		return
	}

	// Create encoder for specified logging format.
	switch strings.ToLower(c.format) {
	case "plain":
		break
	case "json":
		c.Encoder = json.NewEncoder(c.LogFile)
	case "xml":
		c.Encoder = xml.NewEncoder(c.LogFile)
	case "gob":
		c.Encoder = gob.NewEncoder(c.LogFile)

		// Don't let the user output gob to stdout unless they really want to.
		if !c.GobUnsafe && c.logFilename == "/dev/stdout" {
			fmt.Println("Gob encoded messages are not stdout safe, specify logfile or use gobunsafe flag.")
			os.Exit(1)
		}
	default:
		// We didn't get a valid encoder, exit and say so.
		log.Fatal("Invalid log format:", c.format)
	}

	return
}

func (c Config) Close() {
	c.LogFile.Close()
	c.SampleFile.Close()
}

// JSON, XML and GOB all implement this interface so we can simplify log
// output formatting.
type Encoder interface {
	Encode(interface{}) error
}

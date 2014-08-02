package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bemasher/rtlamr/csv"

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
	TimeLimit  time.Duration

	MeterID   uint
	MeterType uint

	SymbolLength int

	BlockSize  uint
	SampleRate uint

	PreambleLength uint
	PacketLength   uint

	Log     *log.Logger
	LogFile *os.File

	GobUnsafe bool
	Encoder   Encoder

	SampleFile *os.File

	Quiet  bool
	Single bool
	Help   bool
}

func (c *Config) Parse() (err error) {
	longHelp := map[string]string{
		// help
		"help": `Print this help.`,

		// centerfreq
		"centerfreq": `Sets the center frequency of the rtl_tcp server. Defaults to 920.29MHz.`,

		// duration
		"duration": `Sets time to receive for, 0 for infinite. Defaults to infinite.
	If the time limit expires during processing of a block (which is quite
	likely) it will exit on the next pass through the receive loop. Exiting
	after an expired duration will print the total runtime to the log file.`,

		// filterid
		"filterid": `Sets a meter id to filter by, 0 for no filtering. Defaults to no filtering.
	Any received messages not matching the given id will be silently ignored.`,

		// filtertype
		"filtertype": `Sets an ert type to filter by, 0 for no filtering. Defaults to no filtering.
	Any received messages not matching the given type will be silently ignored.`,

		// format
		"format": `Sets the log output format. Defaults to plain.
	Plain text is formatted using the following format string:

		{Time:%s Offset:%d Length:%d SCM:{ID:%8d Type:%2d Tamper:%+v Consumption:%8d Checksum:0x%04X}}

	No fields are omitted for csv, json, xml or gob output. Plain text conditionally
	omits offset and length fields if not dumping samples to file via -samplefile.

	For json and xml output each line is an element, there is no root node.`,
		"gobunsafe": `Must be true to allow writing gob encoded output to stdout. Defaults to false.
	Doing so would normally break a terminal, so we disable it unless
	explicitly enabled.`,

		// logfile
		"logfile": `Sets file to dump log messages to. Defaults to os.DevNull and prints to stderr.
	Log messages have the following structure:

		type Message struct {
			Time   time.Time
			Offset int64
			Length int
			SCM    SCM
		}

		type SCM struct {
			ID          uint32
			Type        uint8
			Tamper      Tamper
			Consumption uint32
			Checksum    uint16
		}

		type Tamper struct {
			Phy uint8
			Enc uint8
		}

	Messages are encoded one per line for all encoding formats except gob.`,

		// quiet
		"quiet": `Omits state information logged on startup. Defaults to false.
	Below is sample output:

		2014/07/01 02:45:42.416406 Server: 127.0.0.1:1234
		2014/07/01 02:45:42.417406 BlockSize: 4096
		2014/07/01 02:45:42.417406 SampleRate: 2392064
		2014/07/01 02:45:42.417406 DataRate: 32768
		2014/07/01 02:45:42.417406 SymbolLength: 73
		2014/07/01 02:45:42.417406 PreambleSymbols: 42
		2014/07/01 02:45:42.417406 PreambleLength: 3066
		2014/07/01 02:45:42.417406 PacketSymbols: 192
		2014/07/01 02:45:42.417406 PacketLength: 14016
		2014/07/01 02:45:42.417406 CenterFreq: 920299072
		2014/07/01 02:45:42.417406 TimeLimit: 0
		2014/07/01 02:45:42.417406 Format: plain
		2014/07/01 02:45:42.417406 LogFile: /dev/stdout
		2014/07/01 02:45:42.417406 SampleFile: NUL
		2014/07/01 02:45:43.050442 BCH: {GenPoly:16F63 PolyLen:16}
		2014/07/01 02:45:43.050442 GainCount: 29
		2014/07/01 02:45:43.051442 Running...`,

		// samplefile
		"samplefile": `Sets file to dump samples for decoded packets to. Defaults to os.DevNull.
	Output file format are interleaved in-phase and quadrature samples. Each
	are unsigned bytes. These are unmodified output from the dongle. This flag
	enables offset and length fields in plain text log messages. Only samples
	for correctly received messages are dumped.`,

		// server
		"server": `Sets rtl_tcp server address or hostname and port to connect to. Defaults to 127.0.0.1:1234.`,

		// single
		"single": `Provides one shot execution. Defaults to false.
	Receiver listens until exactly one message is received before exiting.`,

		// symbollength
		"symbollength": `Sets the desired symbol length. Defaults to 73.
	Sample rate is determined from this value as follows:

		DataRate = 32768
		SampleRate = SymbolLength * DataRate

	The symbol length also determines the size of the convolution used for the preamble search:

		PreambleSymbols = 42
		BlockSize = 1 << uint(math.Ceil(math.Log2(float64(PreambleSymbols * SymbolLength))))

	Valid symbol lengths are given below (symbol length: bandwidth):

		BlockSize: 512 (fast)
			7: 229.376 kHz, 8: 262.144 kHz, 9: 294.912 kHz

		BlockSize: 2048 (medium)
			28: 917.504 kHz,  29: 950.272 kHz,  30: 983.040 kHz
			31: 1.015808 MHz, 32: 1.048576 MHz, 33: 1.081344 MHz,
			34: 1.114112 MHz, 35: 1.146880 MHz, 36: 1.179648 MHz,
			37: 1.212416 MHz, 38: 1.245184 MHz, 39: 1.277952 MHz,
			40: 1.310720 MHz, 41: 1.343488 MHz, 42: 1.376256 MHz,
			43: 1.409024 MHz, 44: 1.441792 MHz, 45: 1.474560 MHz,
			46: 1.507328 MHz, 47: 1.540096 MHz, 48: 1.572864 MHz

		BlockSize: 4096 (slow)
			49: 1.605632 MHz, 50: 1.638400 MHz, 51: 1.671168 MHz,
			52: 1.703936 MHz, 53: 1.736704 MHz, 54: 1.769472 MHz,
			55: 1.802240 MHz, 56: 1.835008 MHz, 57: 1.867776 MHz,
			58: 1.900544 MHz, 59: 1.933312 MHz, 60: 1.966080 MHz,
			61: 1.998848 MHz, 62: 2.031616 MHz, 63: 2.064384 MHz,
			64: 2.097152 MHz, 65: 2.129920 MHz, 66: 2.162688 MHz,
			67: 2.195456 MHz, 68: 2.228224 MHz, 69: 2.260992 MHz,
			70: 2.293760 MHz, 71: 2.326528 MHz, 72: 2.359296 MHz,
			73: 2.392064 MHz

		BlockSize: 4096 (slow, untested)
			74: 2.424832 MHz, 75: 2.457600 MHz, 76: 2.490368 MHz,
			77: 2.523136 MHz, 78: 2.555904 MHz, 79: 2.588672 MHz,
			80: 2.621440 MHz, 81: 2.654208 MHz, 82: 2.686976 MHz,
			83: 2.719744 MHz, 84: 2.752512 MHz, 85: 2.785280 MHz,
			86: 2.818048 MHz, 87: 2.850816 MHz, 88: 2.883584 MHz,
			89: 2.916352 MHz, 90: 2.949120 MHz, 91: 2.981888 MHz,
			92: 3.014656 MHz, 93: 3.047424 MHz, 94: 3.080192 MHz,
			95: 3.112960 MHz, 96: 3.145728 MHz, 97: 3.178496 MHz`,
	}

	flag.StringVar(&c.serverAddr, "server", "127.0.0.1:1234", "address or hostname of rtl_tcp instance")
	flag.StringVar(&c.logFilename, "logfile", "/dev/stdout", "log statement dump file")
	flag.StringVar(&c.sampleFilename, "samplefile", os.DevNull, "raw signal dump file")

	// Override centerfreq value so rtlamr can run without any non-default flags.
	centerfreqFlag := flag.Lookup("centerfreq")
	centerfreqStr := strconv.FormatUint(CenterFreq, 10)

	centerfreqFlag.DefValue = centerfreqStr
	err = centerfreqFlag.Value.Set(centerfreqStr)
	if err != nil {
		log.Fatal("Error setting default center frequency:", err)
	}

	flag.IntVar(&c.SymbolLength, "symbollength", 73, `symbol length in samples, see -help for valid lengths`)
	flag.DurationVar(&c.TimeLimit, "duration", 0, "time to run for, 0 for infinite")
	flag.UintVar(&c.MeterID, "filterid", 0, "display only messages matching given id")
	flag.UintVar(&c.MeterType, "filtertype", 0, "display only messages matching given type")
	flag.StringVar(&c.format, "format", "plain", "format to write log messages in: plain, csv, json, xml or gob")
	flag.BoolVar(&c.GobUnsafe, "gobunsafe", false, "allow gob output to stdout")
	flag.BoolVar(&c.Quiet, "quiet", false, "suppress printing state information at startup")
	flag.BoolVar(&c.Single, "single", false, "one shot execution")
	flag.BoolVar(&c.Help, "help", false, "print long help")

	flag.Parse()

	if c.Help {
		flag.VisitAll(func(f *flag.Flag) {
			if help, exists := longHelp[f.Name]; exists {
				f.Usage = help + "\n"
			}
		})

		flag.Usage()
		os.Exit(2)
	}

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

	validSymbolLengths := map[int]bool{
		7: true, 8: true, 9: true, 28: true, 29: true, 30: true, 31: true,
		32: true, 33: true, 34: true, 35: true, 36: true, 37: true, 38: true,
		39: true, 40: true, 41: true, 42: true, 43: true, 44: true, 45: true,
		46: true, 47: true, 48: true, 49: true, 50: true, 51: true, 52: true,
		53: true, 54: true, 55: true, 56: true, 57: true, 58: true, 59: true,
		60: true, 61: true, 62: true, 63: true, 64: true, 65: true, 66: true,
		67: true, 68: true, 69: true, 70: true, 71: true, 72: true, 73: true,
		74: true, 75: true, 76: true, 77: true, 78: true, 79: true, 80: true,
		81: true, 82: true, 83: true, 84: true, 85: true, 86: true, 87: true,
		88: true, 89: true, 90: true, 91: true, 92: true, 93: true, 94: true,
		95: true, 96: true, 97: true,
	}

	if !validSymbolLengths[c.SymbolLength] {
		log.Printf("warning: invalid symbol length, probably won't receive anything")
	}

	c.SampleRate = DataRate * uint(c.SymbolLength)

	c.PreambleLength = PreambleSymbols * uint(c.SymbolLength)
	c.PacketLength = PacketSymbols * uint(c.SymbolLength)

	// Power of two sized DFT requires BlockSize to also be power of two.
	// BlockSize must be greater than the preamble length, so calculate next
	// largest power of two from preamble length.
	c.BlockSize = NextPowerOf2(c.PreambleLength)

	// Create encoder for specified logging format.
	switch strings.ToLower(c.format) {
	case "plain":
		break
	case "csv":
		c.Encoder = csv.NewEncoder(c.LogFile)
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

func NextPowerOf2(v uint) uint {
	return 1 << uint(math.Ceil(math.Log2(float64(v))))
}

/*
RTLAMR is an rtl-sdr receiver for Itron ERT compatible smart meters operating in the 900MHz ISM band.

Command-line Flags:

	-centerfreq=920299072

Sets the center frequency of the rtl_tcp server. Defaults to 920.29MHz.

	-duration=0

Sets time to receive for, 0 for infinite. If the time
limit expires during processing of a block (which is quite likely) it will
exit on the next pass through the receive loop. Exiting after an expired
duration will print the total runtime to the log file. Defaults to infinite.

	-filterid=0

Sets a meter id to filter by, 0 for no filtering. Any received messages not
matching the given id will be silently ignored. Defaults to no filtering.

	-format="plain"

Sets the log output format. Defaults to plain.

Plain text is formatted using the following format string:

	{Time:%s Offset:%d Length:%d SCM:{ID:%8d Type:%2d Tamper:%+v Consumption:%8d Checksum:0x%04X}}

No fields are omitted for json, xml or gob output. Plain text conditionally
omits offset and length fields if not dumping samples to file.

For json and xml output each line is an element, there is no root node.

	-gobunsafe=false: allow gob output to stdout

Must be true to allow writing gob encoded output to stdout. Doing so would
normally break a terminal, so we disable it unless explicitly enabled.

	-logfile="/dev/stdout"

Sets file to dump log messages to.

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

Messages are encoded one per line for all encoding formats except gob.

	-quiet=false

Omits state information logged on startup.

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
	2014/07/01 02:45:43.051442 Running...

Above is example state information.

	-samplefile="NUL"

Sets file to dump samples for decoded packets to. Output file format are
interleaved in-phase and quadrature samples each are unsigned bytes. These are
unmodified output from the dongle. This flag enables offset and length fields
in plain text log messages.

	-server="127.0.0.1:1234"

Sets rtl_tcp server address or hostname and port to connect to.

	-single=false

Provides one shot execution. Receiver listens until exactly one message is received before exiting.

	-symbollength=73

Sets the desired symbol rate. Sample rate and performance are dependent on this value.

	SampleRate = SymbolLength * DataRate

The symbol length also determines the size of the convolution used for the preamble search.

	PreambleSymbols = 42
	BlockSize = 1 << uint(math.Ceil(math.Log2(float64(PreambleSymbols * SymbolLength))))

Valid symbol lengths are given below (symbol length: bandwidth):

	BlockSize: 512 (fastest)
		7: 229.376 kHz, 8: 262.144 kHz, 9: 294.912 kHz

	BlockSize: 2048 (medium)
		28: 917.504 kHz,  29: 950.272 kHz, 30: 983.040 kHz

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
		95: 3.112960 MHz, 96: 3.145728 MHz, 97: 3.178496 MHz
*/
package main

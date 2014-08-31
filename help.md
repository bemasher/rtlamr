### RTLAMR Help
Detailed usage information for the various flags of RTLAMR.

  - `logfile` writes log statements to the given file. Defaults to `/dev/stdout`.
  - `samplefile` writes raw signal to the given file. Samples are interleaved 8-bit inphase and quadrature pairs. Fields Offset and Length are omitted in the plain log format if this option isn't used. Defaults to `/dev/null`.
  - `cpuprofile` writes pprof profiling information to the given filename. Useful for determining bottlenecks and performance of the program. Defaults to blank and writes no profiling information.
  - `duration` sets the amount of time to listen for before exiting. Defaults to 0 for infinite, [GoDoc: time.Duration](http://godoc.org/time#Duration)
  - `fastmag` uses a faster magnitude calculation algorithm, sacrifices accuracy for speed. Defaults to false.
  - `filterid` display and dump raw samples only for messages with a matching meter id. Defaults to 0 for no filtering.
  - `filtertype` display and dump raw samples only for messages with a matching type. Defaults to 0 for no filtering.
  - `format` format to write log messages in. Defaults to plain. Options: plain, csv, json, xml or gob.

    ```go
	type LogMessage struct {
		Time   time.Time
		Offset int64
		Length int
		Message // SCM and IDM both implement Message.
	}
    ```

    ```go
	type SCM struct {
		ID          uint32
		Type        uint8
		TamperPhy   uint8
		TamperEnc   uint8
		Consumption uint32
		Checksum    uint16
	}
    ```

    ```go
	type IDM struct {
		Preamble                         uint32
		PacketTypeID                     uint8
		PacketLength                     uint8
		HammingCode                      uint8
		ApplicationVersion               uint8
		ERTType                          uint8
		ERTSerialNumber                  uint32
		ConsumptionIntervalCount         uint8
		ModuleProgrammingState           uint8
		TamperCounters                   []byte // 6 Bytes
		AsynchronousCounters             uint16
		PowerOutageFlags                 []byte // 6 Bytes
		LastConsumptionCount             uint32
		DifferentialConsumptionIntervals [47]uint16 // 53 Bytes
		TransmitTimeOffset               uint16
		SerialNumberCRC                  uint16
		PacketCRC                        uint16
	}
    ```
  - `gobunsafe` allows gob output to stdout. Gob output is not stdout safe and will bork a terminal so user must specify `-gobunsafe` or specify a non-stdout file via `-logfile`. Defaults to false and warns user.
  - `msgtype` specifies the message type to receive: scm or idm. Defaults to scm.
  - `quiet` suppresses printing state information at startup. Defaults to false.
  - `single` will listen until exactly one message is received that matches all of the given filters if any. Defaults to false.
  - `symbollength` sets the symbol length in samples. Defaults to 73.

    Sample rate is determined by this value as follows:

    ```
DataRate = 32768
SampleRate = SymbolLength * DataRate
    ```

    Sample rates are limited by the dongle such that:

    ```
225 kHz < Sample Rate < 300 kHz
900 kHz < Sample Rate < 3.2 MHz
    ```

    The symbol length also determines the size of sample blocks read and processed on each pass.

    ```
PreambleSymbols = 21 (for SCM) and 32 (for IDM)
BlockSize = 1 << uint(math.Ceil(math.Log2(float64(PreambleSymbols * SymbolLength))))
    ```

    Valid symbol lengths are given below, block size calculated for SCM:

      - Block Size: 512

      Symbol Length | Sample Rate
      ------------- | -----------
      7             | 229.376 kHz
      8             | 262.144 kHz
      9             | 294.912 kHz

      - Block Size: 2048

      Symbol Length | Sample Rate  | Symbol Length | Sample Rate 
      ------------- | -----------  | ------------- | ----------- 
      28            | 917.504 kHz  | 39            | 1.277952 MHz
      29            | 950.272 kHz  | 40            | 1.310720 MHz
      30            | 983.040 kHz  | 41            | 1.343488 MHz
      31            | 1.015808 MHz | 42            | 1.376256 MHz
      32            | 1.048576 MHz | 43            | 1.409024 MHz
      33            | 1.081344 MHz | 44            | 1.441792 MHz
      34            | 1.114112 MHz | 45            | 1.474560 MHz
      35            | 1.146880 MHz | 46            | 1.507328 MHz
      36            | 1.179648 MHz | 47            | 1.540096 MHz
      37            | 1.212416 MHz | 48            | 1.572864 MHz
      38            | 1.245184 MHz

      - Block Size: 4096

      Symbol Length | Sample Rate  | Symbol Length | Sample Rate 
      ------------- | -----------  | ------------- | ----------- 
      49            | 1.605632 MHz | 74            | 2.424832 MHz
      50            | 1.638400 MHz | 75            | 2.457600 MHz
      51            | 1.671168 MHz | 76            | 2.490368 MHz
      52            | 1.703936 MHz | 77            | 2.523136 MHz
      53            | 1.736704 MHz | 78            | 2.555904 MHz
      54            | 1.769472 MHz | 79            | 2.588672 MHz
      55            | 1.802240 MHz | 80            | 2.621440 MHz
      56            | 1.835008 MHz | 81            | 2.654208 MHz
      57            | 1.867776 MHz | 82            | 2.686976 MHz
      58            | 1.900544 MHz | 83            | 2.719744 MHz
      59            | 1.933312 MHz | 84            | 2.752512 MHz
      60            | 1.966080 MHz | 85            | 2.785280 MHz
      61            | 1.998848 MHz | 86            | 2.818048 MHz
      62            | 2.031616 MHz | 87            | 2.850816 MHz
      63            | 2.064384 MHz | 88            | 2.883584 MHz
      64            | 2.097152 MHz | 89            | 2.916352 MHz
      65            | 2.129920 MHz | 90            | 2.949120 MHz
      66            | 2.162688 MHz | 91            | 2.981888 MHz
      67            | 2.195456 MHz | 92            | 3.014656 MHz
      68            | 2.228224 MHz | 93            | 3.047424 MHz
      69            | 2.260992 MHz | 94            | 3.080192 MHz
      70            | 2.293760 MHz | 95            | 3.112960 MHz
      71            | 2.326528 MHz | 96            | 3.145728 MHz
      72            | 2.359296 MHz | 97            | 3.178496 MHz
      73            | 2.392064 MHz
  - `centerfreq` sets the center frequency to receive on. Defaults to 920299072.
  - `samplerate` sets the sample rate. This will override the sample rate calculated by `-symbollength`.
  - If any of the gain-related flags are specified rtlamr won't set any gain options of it's own. By default rtlamr enables `-tunergainmode`. Flags which disable this behavior: `-gainbyindex`, `-tunergainmode`, `-tunergain` and `-agcmode`.

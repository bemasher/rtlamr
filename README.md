### Purpose
For several years now utilities have been using "smart meters" to optimize their residential meter reading infrastructure. Smart meters continuously transmit consumption information in the 900MHz ISM band allowing utilities to simply send readers driving through neighborhoods to collect commodity consumption information. The protocol used to transmit these messages is fairly straight forward, however I have yet to find any reasonably priced product for receiving these messages.

This project is a proof of concept software defined radio receiver for these messages. We make use of an inexpensive rtl-sdr dongle to allow users to non-invasively record and analyze the commodity consumption of their household.

Currently the only known supported and tested meter is the Itron C1SR. However, the protocol is designed to be useful for several different commodities and should be capable of receiving messages from any ERT capable smart meter.

For more info check out the project page: [http://bemasher.github.io/rtlamr/](http://bemasher.github.io/rtlamr/)

[![Build Status](https://travis-ci.org/bemasher/rtlamr.svg?branch=master)](https://travis-ci.org/bemasher/rtlamr)

### Requirements
 * GoLang >=1.2 (Go build environment setup guide: http://golang.org/doc/code.html)
 * GCC (on windows TDM-GCCx64 works nicely)
 * libfftw >=3.3
   * Windows: [pre-built binaries](http://www.fftw.org/install/windows.html)
   * Linux (debian): [package](https://packages.debian.org/stable/libfftw3-dev)
 * rtl-sdr
   * Windows: [pre-built binaries](http://sdr.osmocom.org/trac/attachment/wiki/rtl-sdr/RelWithDebInfo.zip)
   * Linux: [source and build instructions](http://sdr.osmocom.org/trac/wiki/rtl-sdr)

### Building
This project requires two other packages I've written for SDR related things in Go. The package [`github.com/bemasher/rtltcp`](http://godoc.org/github.com/bemasher/rtltcp) provides a means of controlling and sampling from rtl-sdr dongles. This package will be automatically downloaded and installed when getting rtlamr.

The second package needed is [`github.com/bemasher/fftw`](http://godoc.org/github.com/bemasher/fftw), which may require more effort to build. Assuming for linux you already have the necessary library, no extra work should need to be done. For windows a library file will need to be generated from the dll and def files for gcc. The FFTW defs and dlls can be found here: http://www.fftw.org/install/windows.html

#### On Windows

	go get -d github.com/bemasher/fftw
	dlltool -d libfftw3-3.def -D libfftw3-3.dll -l $GOPATH/src/github.com/bemasher/fftw/libfftw3.a
	go get github.com/bemasher/rtlamr

#### On Linux (Debian/Ubuntu)
	
	sudo apt-get install libfftw3-dev
	go get github.com/bemasher/rtlamr

This will produce the binary `$GOPATH/bin/rtlamr`. For convenience it's common to add `$GOPATH/bin` to the path.

#### With Docker
This project can be built and executed using docker:

```
docker pull bemasher/rtlamr
docker run --name rtlamr bemasher/rtlamr
```

This can also be run using the `bemasher/rtl-sdr` container:

```
docker pull bemasher/rtl-sdr
docker run -d --privileged -v /dev/bus/usb:/dev/bus/usb --name rtl_tcp bemasher/rtl-sdr rtl_tcp -a 0.0.0.0
docker run --name rtlamr --link rtl_tcp:rtl_tcp bemasher/rtlamr -server=rtl_tcp:1234
```

### Usage
Available command-line flags are as follows:

```
Usage of rtlamr:
  -centerfreq=920299072: center frequency to receive on
  -duration=0: time to run for, 0 for infinite
  -filterid=0: display only messages matching given id
  -format="plain": format to write log messages in: plain, json, xml or gob
  -gobunsafe=false: allow gob output to stdout
  -help=false: print long help
  -logfile="/dev/stdout": log statement dump file
  -quiet=false: suppress printing state information at startup
  -samplefile="NUL": raw signal dump file
  -server="127.0.0.1:1234": address or hostname of rtl_tcp instance
  -single=false: one shot execution
  -symbollength=73: symbol length in samples, see -help for valid lengths
```

Long Help via `-help`:
```
Usage of rtlamr:
  -centerfreq=920299072: Sets the center frequency of the rtl_tcp server. Defaults to 920.29MHz.

  -duration=0: Sets time to receive for, 0 for infinite. Defaults to infinite.
	If the time limit expires during processing of a block (which is quite
	likely) it will exit on the next pass through the receive loop. Exiting
	after an expired duration will print the total runtime to the log file.

  -filterid=0: Sets a meter id to filter by, 0 for no filtering. Defaults to no filtering.
	Any received messages not matching the given id will be silently ignored.

  -format="plain": Sets the log output format. Defaults to plain.
	Plain text is formatted using the following format string:

		{Time:%s Offset:%d Length:%d SCM:{ID:%8d Type:%2d Tamper:%+v Consumption:%8d Checksum:0x%04X}}

	No fields are omitted for json, xml or gob output. Plain text conditionally
	omits offset and length fields if not dumping samples to file via -samplefile.

	For json and xml output each line is an element, there is no root node.

  -gobunsafe=false: Must be true to allow writing gob encoded output to stdout. Defaults to false.
	Doing so would normally break a terminal, so we disable it unless
	explicitly enabled.

  -help=false: Print this help.

  -logfile="/dev/stdout": Sets file to dump log messages to. Defaults to os.DevNull and prints to stderr.
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

  -quiet=false: Omits state information logged on startup. Defaults to false.
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
		2014/07/01 02:45:43.051442 Running...

  -samplefile="NUL": Sets file to dump samples for decoded packets to. Defaults to os.DevNull.
	Output file format are interleaved in-phase and quadrature samples. Each
	are unsigned bytes. These are unmodified output from the dongle. This flag
	enables offset and length fields in plain text log messages. Only samples
	for correctly received messages are dumped.

  -server="127.0.0.1:1234": Sets rtl_tcp server address or hostname and port to connect to. Defaults to 127.0.0.1:1234.

  -single=false: Provides one shot execution. Defaults to false.
	Receiver listens until exactly one message is received before exiting.

  -symbollength=73: Sets the desired symbol length. Defaults to 73.
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
			95: 3.112960 MHz, 96: 3.145728 MHz, 97: 3.178496 MHz
```

Running the receiver is as simple as starting an `rtl_tcp` instance and then starting the receiver:

```bash
# Terminal A
$ rtl_tcp

# Terminal B
$ rtlamr
```

If you want to run the spectrum server on a different machine than the receiver you'll want to specify an address to listen on that is accessible from the machine `rtlamr` will run on with the `-a` option for `rtl_tcp`.

Using a NooElec NESDR Nano R820T with the provided antenna, I can reliably receive standard consumption messages from ~250 different meters and intermittently from another 400 meters. These figures are calculated from messages received during a 25 minute window where the preamble had no bit errors and no errors were detected or corrected using the checksum. Reliably in this case means receiving at least 10 of the expected 12 messages and intermittently means 3-9 messages.

### Ethics
_Do not use this for nefarious purposes._ If you do, I don't want to know about it, I am not and will not be responsible for your lack of common decency and/or foresight. However, if you find a clever non-evil use for this, by all means, share.

### License
The source of this project is licensed under Affero GPL. According to [http://choosealicense.com/licenses/agpl/](http://choosealicense.com/licenses/agpl/) you may:

#### Required:

 * Source code must be made available when distributing the software. In the case of LGPL, the source for the library (and not the entire program) must be made available.
 * Include a copy of the license and copyright notice with the code.
 * Indicate significant changes made to the code.

#### Permitted:

 * This software and derivatives may be used for commercial purposes.
 * You may distribute this software.
 * This software may be modified.
 * You may use and modify the software without distributing it.

#### Forbidden:

 * Software is provided without warranty and the software author/license owner cannot be held liable for damages.
 * You may not grant a sublicense to modify and distribute this software to third parties not included in the license.

### Feedback
If you have any general questions or feedback leave a comment below. For bugs, feature suggestions and anything directly relating to the program itself, submit an issue in github.

### Future

 * There's still a decent amount of house-keeping that needs to be done to clean up the code for both readability and performance.
 * Move away from dependence on FFTW. While FFTW is a great library integration with Go is messy and it's absence would greatly simplify the build process.
 * Implement direct error correction rather than brute-force method.
 * Finish tools for discovery and usage of hopping pattern for a particular meter. There's enough material in this alone for another writeup.
 * Implement adaptive preamble quality thresholding to improve false positive rejection.

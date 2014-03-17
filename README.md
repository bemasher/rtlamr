### Purpose
For several years now utilities have been using "smart meters" to optimize their residential meter reading infrastructure. Smart meters continuously transmit consumption information in the 900MHz ISM band allowing utilities to simply send readers driving through neighborhoods to collect commodity consumption information. The protocol used to transmit these messages is fairly straight forward, however I have yet to find any reasonably priced product for receiving these messages.

This project is a proof of concept software defined radio receiver for these messages. We make use of an inexpensive rtl-sdr dongle to allow users to non-invasively record and analyze the commodity consumption of their household.

Currently the only known supported and tested meter is the Itron C1SR. However, the protocol is designed to be useful for several different commodities and should be capable of receiving messages from any ERT capable smart meter.

For more info check out the project page: [http://bemasher.github.io/rtlamr/](http://bemasher.github.io/rtlamr/)

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

### Usage
Available command-line flags are as follows:

```bash
$ rtlamr -h
Usage of rtlamr:
	-centerfreq=920299072: center frequency to receive on
	-duration=0: time to run for, 0 for infinite
	-filterid=0: display only messages matching given id
	-format="plain": format to write log messages in: plain, json, xml or gob
	-gobunsafe=false: allow gob output to stdout
	-logfile="/dev/stdout": log statement dump file
	-samplefile="NUL": received message signal dump file, offset and message length are displayed to log when enabled
	-server="127.0.0.1:1234": address or hostname of rtl_tcp instance
```

Note that for both json and xml output, there is no root element. Instead each line is one element.

Running the receiver is as simple as starting an `rtl_tcp` instance and then starting the receiver:

```bash
# Terminal A
$ rtl_tcp

# Terminal B
$ rtlamr
```

If you want to run the spectrum server on a different machine than the receiver you'll want to specify an address to listen on that is accessible from the machine `rtlamr` will run on with the `-a` option for `rtl_tcp`.

Using a NooElec NESDR Nano R820T with the provided antenna, I can reliably receive standard consumption messages from ~250 different meters and intermittently from another 400 meters. These figures are calculated from messages received during a 25 minute window where the preamble had no bit errors and no errors were detected or corrected using the checksum. Reliably in this case means receiving at least 10 of the expected 12 messages and intermittently means 3-9 messages.

### Example

Example output is as follows, note that the meter ID's and checksums have been obscured to avoid releasing potentially sensitive information:

```bash
$ rtlamr
recv.go:435: Server: 127.0.0.1:1234
recv.go:436: BlockSize: 4096
recv.go:437: SampleRate: 2.4e+06
recv.go:438: DataRate: 32768
recv.go:439: SymbolLength: 73.2421875
recv.go:440: PacketSymbols: 192
recv.go:441: PacketLength: 14062.5
recv.go:442: CenterFreq: 920299072
recv.go:443: TimeLimit: 0
recv.go:445: Format: plain
recv.go:446: LogFile: /dev/stdout
recv.go:447: SampleFile: NUL
recv.go:177: BCH: {GenPoly:16F63 PolyLen:16}
recv.go:189: GainCount: 29
recv.go:457: Running...
{Time:2014-03-17T06:32:58.750 SCM:{ID:1758#### Type: 7 Tamper:{Phy:2 Enc:1} Consumption: 2107876 Checksum:0x21##}}
{Time:2014-03-17T06:32:58.909 SCM:{ID:1758#### Type: 7 Tamper:{Phy:1 Enc:1} Consumption:  299869 Checksum:0x9E##}}
{Time:2014-03-17T06:32:59.010 SCM:{ID:1758#### Type: 7 Tamper:{Phy:2 Enc:1} Consumption:  924402 Checksum:0xA9##}}
{Time:2014-03-17T06:32:59.080 SCM:{ID:1758#### Type: 7 Tamper:{Phy:1 Enc:2} Consumption:  990028 Checksum:0x1F##}}
{Time:2014-03-17T06:32:59.514 SCM:{ID:1757#### Type: 7 Tamper:{Phy:2 Enc:1} Consumption: 6659540 Checksum:0xAC##}}
{Time:2014-03-17T06:32:59.663 SCM:{ID:1758#### Type: 7 Tamper:{Phy:3 Enc:0} Consumption: 1897496 Checksum:0x28##}}
{Time:2014-03-17T06:32:59.881 SCM:{ID:1758#### Type: 7 Tamper:{Phy:2 Enc:1} Consumption: 3710076 Checksum:0x57##}}
{Time:2014-03-17T06:33:00.064 SCM:{ID:1758#### Type: 7 Tamper:{Phy:2 Enc:1} Consumption: 2647704 Checksum:0xCD##}}
{Time:2014-03-17T06:33:00.158 SCM:{ID:1757#### Type: 8 Tamper:{Phy:1 Enc:1} Consumption:   31214 Checksum:0x9C##}}
{Time:2014-03-17T06:33:00.429 SCM:{ID:1757#### Type: 8 Tamper:{Phy:1 Enc:1} Consumption:   31214 Checksum:0x87##}}
{Time:2014-03-17T06:33:00.985 SCM:{ID:1758#### Type: 7 Tamper:{Phy:1 Enc:0} Consumption:  923336 Checksum:0xF7##}}
{Time:2014-03-17T06:33:01.096 SCM:{ID:1757#### Type: 7 Tamper:{Phy:2 Enc:0} Consumption: 2321610 Checksum:0xF1##}}
{Time:2014-03-17T06:33:01.197 SCM:{ID:1758#### Type: 7 Tamper:{Phy:2 Enc:1} Consumption: 1153099 Checksum:0x0C##}}
{Time:2014-03-17T06:33:01.248 SCM:{ID:1757#### Type: 7 Tamper:{Phy:2 Enc:2} Consumption: 6434152 Checksum:0x93##}}
```

Below is a photo of the face of the meter I've been testing with along with sample output received from the meter. The messages below are all from the same meter. You can see on the face of the meter the commodity type, in this case electricity is `07` and the meter ID is `17581###` with the last 3 digits censored. The meter displays the current consumption value in kWh's and transmits hundredths of a kWh.

<img class="thumbnail img-responsive" style="margin: 0 auto; padding: 20px" src="https://raw2.github.com/bemasher/rtlamr/master/misc/example.jpg">

```bash
$ rtlamr -samplefile=data/signal.bin
recv.go:557: Config: {ServerAddr:127.0.0.1:1234 Freq:920299072 TimeLimit:0 LogFile:/dev/stdout SampleFile:data/sign
al.bin}
recv.go:558: BlockSize: 16384
recv.go:559: SampleRate: 2.4e+06
recv.go:560: DataRate: 32768
recv.go:561: SymbolLength: 73.2421875
recv.go:562: PacketSymbols: 192
recv.go:563: PacketLength: 14062.5
recv.go:564: CenterFreq: 920299072
recv.go:131: BCH: {GenPoly:16F63 PolyLen:16 Syndromes:0}
recv.go:137: GainCount: 29
recv.go:570: Running...
{ID:17581### Type: 7 Tamper:{Phy:1 Enc:0} Consumption:  899729 Checksum:0x70##}
{ID:17581### Type: 7 Tamper:{Phy:1 Enc:0} Consumption:  899729 Checksum:0x70##}
{ID:17581### Type: 7 Tamper:{Phy:1 Enc:0} Consumption:  899734 Checksum:0x02##}
{ID:17581### Type: 7 Tamper:{Phy:1 Enc:0} Consumption:  899734 Checksum:0x02##}
{ID:17581### Type: 7 Tamper:{Phy:1 Enc:0} Consumption:  899737 Checksum:0x04##}
{ID:17581### Type: 7 Tamper:{Phy:1 Enc:0} Consumption:  899737 Checksum:0x04##}
{ID:17581### Type: 7 Tamper:{Phy:1 Enc:0} Consumption:  899737 Checksum:0x04##}
{ID:17581### Type: 7 Tamper:{Phy:1 Enc:0} Consumption:  899737 Checksum:0x04##}
```

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
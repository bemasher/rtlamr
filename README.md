# Experimental Branch
This branch is an experimental version of rtlamr, and is essentially a rewrite. This version uses a new preamble detection algorithm which doesn't require an FFT and so doesn't require libFFTW. This will allow cross-compiled builds for all major platforms and architectures now that cgo is no longer used. The rest of the documentation will be updated to reflect the build process in the near future.

### Purpose
For several years now utilities have been using "smart meters" to optimize their residential meter reading infrastructure. Smart meters continuously transmit consumption information in the 900MHz ISM band allowing utilities to simply send readers driving through neighborhoods to collect commodity consumption information. The protocol used to transmit these messages is fairly straight forward, however I have yet to find any reasonably priced product for receiving these messages.

This project is a proof of concept software defined radio receiver for these messages. We make use of an inexpensive rtl-sdr dongle to allow users to non-invasively record and analyze the commodity consumption of their household.

Currently the only known supported and tested meter is the Itron C1SR. However, the protocol is designed to be useful for several different commodities and should be capable of receiving messages from any ERT capable smart meter.

For more info check out the project page: [http://bemasher.github.io/rtlamr/](http://bemasher.github.io/rtlamr/)

[![Build Status](https://travis-ci.org/bemasher/rtlamr.svg?branch=master)](https://travis-ci.org/bemasher/rtlamr)

### Requirements
 * GoLang >=1.2 (Go build environment setup guide: http://golang.org/doc/code.html)
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

```
Usage of rtlamr:
  -duration=0: time to run for, 0 for infinite
  -filterid=0: display only messages matching given id
  -filtertype=0: display only messages matching given type
  -format="plain": format to write log messages in: plain, csv, json, xml or gob
  -gobunsafe=false: allow gob output to stdout
  -h=false: print short help
  -help=false: print long help
  -logfile="/dev/stdout": log statement dump file
  -quiet=false: suppress printing state information at startup
  -samplefile="NUL": raw signal dump file
  -single=false: one shot execution
  -symbollength=73: symbol length in samples, see -help for valid lengths

rtltcp specific:
  -agcmode=false: enable/disable rtl agc
  -centerfreq=920299072: center frequency to receive on
  -directsampling=false: enable/disable direct sampling
  -freqcorrection=0: frequency correction in ppm
  -gainbyindex=0: set gain by index
  -h=false: print short help
  -help=false: print long help
  -offsettuning=false: enable/disable offset tuning
  -rtlxtalfreq=0: set rtl xtal frequency
  -samplerate=2400000: sample rate
  -server="127.0.0.1:1234": address or hostname of rtl_tcp instance
  -testmode=false: enable/disable test mode
  -tunergain=0: set tuner gain in dB
  -tunergainmode=true: enable/disable tuner gain
  -tunerxtalfreq=0: set tuner xtal frequency
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

### Compatibility
I've compiled a list of ERT-compatible meters and modules which can be found here: https://github.com/bemasher/rtlamr/blob/master/meters.md

If you've got a meter not on the list that you've successfully received messages from, you can submit this info from the meters list above.

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

 - [ ] There's still a decent amount of house-keeping that needs to be done to clean up the code for both readability and performance.
 - [x] Move away from dependence on FFTW. While FFTW is a great library integration with Go is messy and it's absence would greatly simplify the build process.
 - [ ] Implement direct error correction rather than brute-force method.
 - [ ] Finish tools for discovery and usage of hopping pattern for a particular meter. There's enough material in this alone for another writeup.

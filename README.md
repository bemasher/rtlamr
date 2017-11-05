### Purpose
Utilities often use "smart meters" to optimize their residential meter reading infrastructure. Smart meters transmit consumption information in the various ISM bands allowing utilities to simply send readers driving through neighborhoods to collect commodity consumption information. One protocol in particular: Encoder Receiver Transmitter by Itron is fairly straight forward to decode and operates in the 900MHz ISM band, well within the tunable range of inexpensive rtl-sdr dongles.

This project is a software defined radio receiver for these messages. We make use of an inexpensive rtl-sdr dongle to allow users to non-invasively record and analyze the commodity consumption of their household.

[![Build Status](https://travis-ci.org/bemasher/rtlamr.svg?branch=master&style=flat)](https://travis-ci.org/bemasher/rtlamr)
[![AGPLv3 License](http://img.shields.io/badge/license-AGPLv3-blue.svg?style=flat)](http://choosealicense.com/licenses/agpl-3.0/)

### Requirements
 * GoLang >=1.3 (Go build environment setup guide: http://golang.org/doc/code.html)
 * rtl-sdr
   * Windows: [pre-built binaries](http://sdr.osmocom.org/trac/attachment/wiki/rtl-sdr/RelWithDebInfo.zip)
   * Linux: [source and build instructions](http://sdr.osmocom.org/trac/wiki/rtl-sdr)

### Building
This project requires the package [`github.com/bemasher/rtltcp`](http://godoc.org/github.com/bemasher/rtltcp), which provides a means of controlling and sampling from rtl-sdr dongles via the `rtl_tcp` tool. This package will be automatically downloaded and installed when getting rtlamr. The following command should be all that is required to install rtlamr.

	go get github.com/bemasher/rtlamr

This will produce the binary `$GOPATH/bin/rtlamr`. For convenience it's common to add `$GOPATH/bin` to the path.

### Usage
Available command-line flags are as follows:

```
Usage of rtlamr:
  -decimation=1: integer decimation factor, keep every nth sample
  -duration=0s: time to run for, 0 for infinite, ex. 1h5m10s
  -filterid=: display only messages matching an id in a comma-separated list of ids.
  -filtertype=: display only messages matching a type in a comma-separated list of types.
  -format=plain: format to write log messages in: plain, csv, json, or xml
  -msgtype=scm: message type to receive: scm, scm+, idm, r900 and r900bcd
  -samplefile=/dev/null: raw signal dump file
  -single=false: one shot execution, if used with -filterid, will wait for exactly one packet from each meter id
  -symbollength=72: symbol length in samples
  -unique=false: suppress duplicate messages from each meter
  -version=false: display build date and commit hash
rtltcp specific:
  -agcmode=false: enable/disable rtl agc
  -centerfreq=0: center frequency to receive on
  -directsampling=false: enable/disable direct sampling
  -freqcorrection=0: frequency correction in ppm
  -gainbyindex=0: set gain by index
  -offsettuning=false: enable/disable offset tuning
  -rtlxtalfreq=0: set rtl xtal frequency
  -samplerate=0: sample rate
  -server=127.0.0.1:1234: address or hostname of rtl_tcp instance
  -testmode=false: enable/disable test mode
  -tunergain=0: set tuner gain in dB
  -tunergainmode=false: enable/disable tuner gain
  -tunerxtalfreq=0: set tuner xtal frequency
```

Flag default values may be overridden via environment variables which are a flag's name in all-caps prefixed by `RTLAMR_`. Flags passed at time of execution will override any values set by environment variable.

```bash
rtlamr -h

...
  -msgtype=scm: message type to receive: scm, scm+, idm, r900 and r900bcd
...
```

```bash
RTLAMR_MSGTYPE=idm rtlamr -h

...
  -msgtype=idm: message type to receive: scm, scm+, idm, r900 and r900bcd
...
```

Running the receiver is as simple as starting an `rtl_tcp` instance and then starting the receiver:

```bash
# Terminal A
$ rtl_tcp

# Terminal B
$ rtlamr
```

If you want to run the spectrum server on a different machine than the receiver you'll want to specify an address to listen on that is accessible from the machine `rtlamr` will run on with the `-a` option for `rtl_tcp` with an address accessible by the system running the receiver.

### Messages
Currently both SCM (Standard Consumption Message) and IDM (Interval Data Message) packets can be decoded but are mutually exclusive, you cannot receive both simultaneously. See [RTLAMR: Protocol](http://bemasher.github.io/rtlamr/protocol.html) for more details on packet structure.

There's now experimental support for meters with R900 transmitters!

### Sensitivity
Using a NooElec NESDR Nano R820T with the provided antenna, I can reliably receive standard consumption messages from ~300 different meters and intermittently from another ~600 meters. These figures are calculated from the number of messages received during a 25 minute window. Reliably in this case means receiving at least 10 of the expected 12 messages and intermittently means 3-9 messages.

### Compatibility
Currently the only tested meter is the Itron C1SR. However, the protocol is designed to be useful for several different commodities and should be capable of receiving messages from any ERT capable smart meter.

Check out the table of meters I've been compiling from various internet sources: [ERT Compatible Meters](https://github.com/bemasher/rtlamr/blob/master/meters.md)

If you've got a meter not on the list that you've successfully received messages from, you can submit this info via a form available at the link above.

### Ethics
_Do not use this for nefarious purposes._ If you do, I don't want to know about it, I am not and will not be responsible for your lack of common decency and/or foresight. However, if you find a clever non-evil use for this, by all means, share.

### Use Cases
These are a few examples of ways this tool could be used:

**Ethical**
 * Track down stray appliances.
 * Track power generated vs. power consumed.
 * Find a water leak with rtlamr rather than from your bill.
 * Optimize your thermostat to reduce energy consumption.
 * Mass collection for research purposes. (_Please_ anonymize your data.)

**Unethical**
 * Using data collected to determine living patterns of specific persons with the intent to act on this data, particularly without express permission to do so.

### License
The source of this project is licensed under Affero GPL v3.0. According to [http://choosealicense.com/licenses/agpl-3.0/](http://choosealicense.com/licenses/agpl-3.0/) you may:

#### Required:

 * **Disclose Source:** Source code must be made available when distributing the software. In the case of LGPL, the source for the library (and not the entire program) must be made available.
 * **License and copyright notice:** Include a copy of the license and copyright notice with the code.
 * **Network Use is Distribution:** Users who interact with the software via network are given the right to receive a copy of the corresponding source code.
 * **State Changes:** Indicate significant changes made to the code.

#### Permitted:

 * **Commercial Use:** This software and derivatives may be used for commercial purposes.
 * **Distribution:** You may distribute this software.
 * **Modification:** This software may be modified.
 * **Patent Grant:** This license provides an express grant of patent rights from the contributor to the recipient.
 * **Private Use:** You may use and modify the software without distributing it.

#### Forbidden:

 * **Hold Liable:** Software is provided without warranty and the software author/license owner cannot be held liable for damages.
 * **Sublicensing:** You may not grant a sublicense to modify and distribute this software to third parties not included in the license.

### Feedback
If you have any general questions or feedback leave a comment below. For bugs, feature suggestions and anything directly relating to the program itself, submit an issue in github.

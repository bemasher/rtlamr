// RTLAMR - An rtl-sdr receiver for smart meters operating in the 900MHz ISM band.
// Copyright (C) 2015 Douglas Hall
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/bemasher/rtlamr/csv"
)

var logFilename = flag.String("logfile", "/dev/stdout", "log statement dump file")
var logFile *os.File

var sampleFilename = flag.String("samplefile", os.DevNull, "raw signal dump file")
var sampleFile *os.File

var msgType = flag.String("msgtype", "scm", "message type to receive: scm, idm or r900")
var fastMag = flag.Bool("fastmag", false, "use faster alpha max + beta min magnitude approximation")

var symbolLength = flag.Int("symbollength", 73, "symbol length in samples, see -help for valid lengths")

var timeLimit = flag.Duration("duration", 0, "time to run for, 0 for infinite, ex. 1h5m10s")
var meterID UintMap
var meterType UintMap

var encoder Encoder
var format = flag.String("format", "plain", "format to write log messages in: plain, csv, json, xml or gob")
var gobUnsafe = flag.Bool("gobunsafe", false, "allow gob output to stdout")

var quiet = flag.Bool("quiet", false, "suppress printing state information at startup")
var single = flag.Bool("single", false, "one shot execution")

func RegisterFlags() {
	meterID = make(UintMap)
	meterType = make(UintMap)

	flag.Var(meterID, "filterid", "display only messages matching an id in a comma-separated list of ids.")
	flag.Var(meterType, "filtertype", "display only messages matching a type in a comma-separated list of types.")

	rtlamrFlags := map[string]bool{
		"logfile":      true,
		"samplefile":   true,
		"msgtype":      true,
		"symbollength": true,
		"duration":     true,
		"filterid":     true,
		"filtertype":   true,
		"format":       true,
		"gobunsafe":    true,
		"quiet":        true,
		"single":       true,
		"cpuprofile":   true,
		"fastmag":      true,
	}

	printDefaults := func(validFlags map[string]bool, inclusion bool) {
		flag.CommandLine.VisitAll(func(f *flag.Flag) {
			if validFlags[f.Name] != inclusion {
				return
			}

			format := "  -%s=%s: %s\n"
			fmt.Fprintf(os.Stderr, format, f.Name, f.DefValue, f.Usage)
		})
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		printDefaults(rtlamrFlags, true)

		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "rtltcp specific:")
		printDefaults(rtlamrFlags, false)
	}
}

func HandleFlags() {
	var err error

	if *logFilename == "/dev/stdout" {
		logFile = os.Stdout
	} else {
		logFile, err = os.Create(*logFilename)
		if err != nil {
			log.Fatal("Error creating log file:", err)
		}
	}
	log.SetOutput(logFile)

	sampleFile, err = os.Create(*sampleFilename)
	if err != nil {
		log.Fatal("Error creating sample file:", err)
	}

	*format = strings.ToLower(*format)
	switch *format {
	case "plain":
		break
	case "csv":
		encoder = csv.NewEncoder(logFile)
	case "json":
		encoder = json.NewEncoder(logFile)
	case "xml":
		encoder = xml.NewEncoder(logFile)
	case "gob":
		encoder = gob.NewEncoder(logFile)
		if !*gobUnsafe && *logFilename == "/dev/stdout" {
			fmt.Println("Gob encoded messages are not stdout safe, specify non-stdout -logfile or use -gobunsafe.")
			os.Exit(1)
		}
	}
}

// JSON, XML and GOB all implement this interface so we can simplify log
// output formatting.
type Encoder interface {
	Encode(interface{}) error
}

type UintMap map[uint]bool

func (m UintMap) String() (s string) {
	var values []string
	for k := range m {
		values = append(values, strconv.FormatUint(uint64(k), 10))
	}
	return strings.Join(values, ",")
}

func (m UintMap) Set(value string) error {
	values := strings.Split(value, ",")

	for _, v := range values {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return err
		}

		m[uint(n)] = true
	}

	return nil
}

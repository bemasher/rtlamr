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

package scm

import (
	"fmt"
	"strconv"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/decode"
	"github.com/bemasher/rtlamr/parse"
)

func NewPacketConfig(symbolLength int) (cfg decode.PacketConfig) {
	cfg.CenterFreq = 912600155
	cfg.DataRate = 32768
	cfg.SymbolLength = symbolLength
	cfg.PreambleSymbols = 21
	cfg.PacketSymbols = 96
	cfg.Preamble = "111110010101001100000"

	return
}

type Parser struct {
	decode.Decoder
	crc.CRC
}

func NewParser(symbolLength, decimation int, fastMag bool) (p Parser) {
	p.Decoder = decode.NewDecoder(NewPacketConfig(symbolLength), decimation, fastMag)
	p.CRC = crc.NewCRC("BCH", 0, 0x6F63, 0)
	return
}

func (p Parser) Dec() decode.Decoder {
	return p.Decoder
}

func (p Parser) Cfg() decode.PacketConfig {
	return p.Decoder.Cfg
}

func (p Parser) Parse(indices []int) (msgs []parse.Message) {
	seen := make(map[string]bool)

	for _, pkt := range p.Decoder.Slice(indices) {
		s := string(pkt)
		if seen[s] {
			continue
		}
		seen[s] = true

		data := parse.NewDataFromBytes(pkt)

		// If the packet is too short, bail.
		if l := len(data.Bytes); l < 12 {
			continue
		}

		// If the checksum fails, bail.
		if p.Checksum(data.Bytes[2:12]) != 0 {
			continue
		}

		ertid, _ := strconv.ParseUint(data.Bits[21:23]+data.Bits[56:80], 2, 26)
		erttype, _ := strconv.ParseUint(data.Bits[26:30], 2, 4)
		tamperphy, _ := strconv.ParseUint(data.Bits[24:26], 2, 2)
		tamperenc, _ := strconv.ParseUint(data.Bits[30:32], 2, 2)
		consumption, _ := strconv.ParseUint(data.Bits[32:56], 2, 24)
		checksum, _ := strconv.ParseUint(data.Bits[80:96], 2, 16)

		var scm SCM

		scm.ID = uint32(ertid)
		scm.Type = uint8(erttype)
		scm.TamperPhy = uint8(tamperphy)
		scm.TamperEnc = uint8(tamperenc)
		scm.Consumption = uint32(consumption)
		scm.Checksum = uint16(checksum)

		// If the meter id is 0, bail.
		if scm.ID == 0 {
			continue
		}

		msgs = append(msgs, scm)
	}

	return
}

// Standard Consumption Message
type SCM struct {
	ID          uint32 `xml:",attr"`
	Type        uint8  `xml:",attr"`
	TamperPhy   uint8  `xml:",attr"`
	TamperEnc   uint8  `xml:",attr"`
	Consumption uint32 `xml:",attr"`
	Checksum    uint16 `xml:",attr"`
}

func (scm SCM) MsgType() string {
	return "SCM"
}

func (scm SCM) MeterID() uint32 {
	return scm.ID
}

func (scm SCM) MeterType() uint8 {
	return scm.Type
}

func (scm SCM) String() string {
	return fmt.Sprintf("{ID:%8d Type:%2d Tamper:{Phy:%02X Enc:%02X} Consumption:%8d CRC:0x%04X}",
		scm.ID, scm.Type, scm.TamperPhy, scm.TamperEnc, scm.Consumption, scm.Checksum,
	)
}

func (scm SCM) Record() (r []string) {
	r = append(r, strconv.FormatUint(uint64(scm.ID), 10))
	r = append(r, strconv.FormatUint(uint64(scm.Type), 10))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.TamperPhy), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.TamperEnc), 16))
	r = append(r, strconv.FormatUint(uint64(scm.Consumption), 10))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.Checksum), 16))

	return
}

// RTLAMR - An rtl-sdr receiver for smart meters operating in the 900MHz ISM band.
// Copyright (C) 2014 Douglas Hall
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
	"errors"
	"fmt"
	"strconv"

	"github.com/bemasher/rtlamr/crc"
)

func NewSCMPacketConfig(symbolLength int) (cfg PacketConfig) {
	cfg.DataRate = 32768

	cfg.SymbolLength = symbolLength
	cfg.SymbolLength2 = cfg.SymbolLength << 1

	cfg.SampleRate = cfg.DataRate * cfg.SymbolLength

	cfg.PreambleSymbols = 21
	cfg.PacketSymbols = 96

	cfg.PreambleLength = cfg.PreambleSymbols * cfg.SymbolLength2
	cfg.PacketLength = cfg.PacketSymbols * cfg.SymbolLength2

	cfg.BlockSize = NextPowerOf2(cfg.PreambleLength)
	cfg.BlockSize2 = cfg.BlockSize << 1

	cfg.BufferLength = cfg.PacketLength + cfg.BlockSize

	cfg.Preamble = "111110010101001100000"

	return
}

type SCMParser struct {
	crc.CRC
}

func NewSCMParser() (p SCMParser) {
	p.CRC = crc.NewCRC("BCH", 0, 0x6F63, 0)
	return
}

func (p SCMParser) Parse(data Data) (msg Message, err error) {
	var scm SCM

	if p.Checksum(data.Bytes[2:]) != 0 {
		err = errors.New("checksum failed")
		return
	}

	ertid, _ := strconv.ParseUint(data.Bits[21:23]+data.Bits[56:80], 2, 32)
	erttype, _ := strconv.ParseUint(data.Bits[26:30], 2, 8)
	tamperphy, _ := strconv.ParseUint(data.Bits[24:26], 2, 8)
	tamperenc, _ := strconv.ParseUint(data.Bits[30:32], 2, 8)
	consumption, _ := strconv.ParseUint(data.Bits[32:56], 2, 32)
	checksum, _ := strconv.ParseUint(data.Bits[80:96], 2, 16)

	scm.ID = uint32(ertid)
	scm.Type = uint8(erttype)
	scm.TamperPhy = uint8(tamperphy)
	scm.TamperEnc = uint8(tamperenc)
	scm.Consumption = uint32(consumption)
	scm.Checksum = uint16(checksum)

	if scm.ID == 0 {
		err = errors.New("invalid ert id")
	}

	return scm, err
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

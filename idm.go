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
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bemasher/rtlamr/crc"
)

func NewIDMPacketConfig(symbolLength int) (cfg PacketConfig) {
	cfg.DataRate = 32768

	cfg.SymbolLength = symbolLength
	cfg.SymbolLength2 = cfg.SymbolLength << 1

	cfg.SampleRate = cfg.DataRate * cfg.SymbolLength

	cfg.PreambleSymbols = 32
	cfg.PacketSymbols = 92 * 8

	cfg.PreambleLength = cfg.PreambleSymbols * cfg.SymbolLength2
	cfg.PacketLength = cfg.PacketSymbols * cfg.SymbolLength2

	cfg.BlockSize = NextPowerOf2(cfg.PreambleLength)
	cfg.BlockSize2 = cfg.BlockSize << 1

	cfg.BufferLength = cfg.PacketLength + cfg.BlockSize

	cfg.Preamble = "01010101010101010001011010100011"
	return
}

type IDMParser struct {
	crc.CRC
}

func NewIDMParser() (p IDMParser) {
	p.CRC = crc.NewCRC("CCITT", 0xFFFF, 0x1021, 0x1D0F)
	return
}

// Standard Consumption Message
type IDM struct {
	Preamble                         uint32 // Training and Frame sync.
	PacketTypeID                     uint8
	PacketLength                     uint8 // Packet Length MSB
	HammingCode                      uint8 // Packet Length LSB
	ApplicationVersion               uint8
	ERTType                          uint8
	ERTSerialNumber                  uint32
	ConsumptionIntervalCount         uint8
	ModuleProgrammingState           uint8
	TamperCounters                   []byte // 6 Bytes
	AsynchronousCounters             uint16
	PowerOutageFlags                 []byte // 6 Bytes
	LastConsumptionCount             uint32
	DifferentialConsumptionIntervals Interval // 53 Bytes
	TransmitTimeOffset               uint16
	SerialNumberCRC                  uint16
	PacketCRC                        uint16
}

type Interval [47]uint16

// func (interval Interval) MarshalText() (text []byte, err error) {
// 	return []byte(fmt.Sprintf("%+v", interval)), nil
// }

func (interval Interval) Record() (r []string) {
	for _, val := range interval {
		r = append(r, strconv.FormatUint(uint64(val), 10))
	}
	return
}

func (idm IDM) MsgType() string {
	return "IDM"
}

func (idm IDM) MeterID() uint32 {
	return idm.ERTSerialNumber
}

func (idm IDM) MeterType() uint8 {
	return idm.ERTType
}

func (idm IDM) String() string {
	var fields []string

	fields = append(fields, fmt.Sprintf("Preamble:0x%08X", idm.Preamble))
	fields = append(fields, fmt.Sprintf("PacketTypeID:0x%02X", idm.PacketTypeID))
	fields = append(fields, fmt.Sprintf("PacketLength:0x%02X", idm.PacketLength))
	fields = append(fields, fmt.Sprintf("HammingCode:0x%02X", idm.HammingCode))
	fields = append(fields, fmt.Sprintf("ApplicationVersion:0x%02X", idm.ApplicationVersion))
	fields = append(fields, fmt.Sprintf("ERTType:0x%02X", idm.ERTType))
	fields = append(fields, fmt.Sprintf("ERTSerialNumber:% 10d", idm.ERTSerialNumber))
	fields = append(fields, fmt.Sprintf("ConsumptionIntervalCount:%d", idm.ConsumptionIntervalCount))
	fields = append(fields, fmt.Sprintf("ModuleProgrammingState:0x%02X", idm.ModuleProgrammingState))
	fields = append(fields, fmt.Sprintf("TamperCounters:%02X", idm.TamperCounters))
	fields = append(fields, fmt.Sprintf("AsynchronousCounters:0x%02X", idm.AsynchronousCounters))
	fields = append(fields, fmt.Sprintf("PowerOutageFlags:%02X", idm.PowerOutageFlags))
	fields = append(fields, fmt.Sprintf("LastConsumptionCount:%d", idm.LastConsumptionCount))
	fields = append(fields, fmt.Sprintf("DifferentialConsumptionIntervals:%d", idm.DifferentialConsumptionIntervals))
	fields = append(fields, fmt.Sprintf("TransmitTimeOffset:%d", idm.TransmitTimeOffset))
	fields = append(fields, fmt.Sprintf("SerialNumberCRC:0x%04X", idm.SerialNumberCRC))
	fields = append(fields, fmt.Sprintf("PacketCRC:0x%04X", idm.PacketCRC))

	return "{" + strings.Join(fields, " ") + "}"
}

func (idm IDM) Record() (r []string) {
	r = append(r, fmt.Sprintf("0x%08X", idm.Preamble))
	r = append(r, fmt.Sprintf("0x%02X", idm.PacketTypeID))
	r = append(r, fmt.Sprintf("0x%02X", idm.PacketLength))
	r = append(r, fmt.Sprintf("0x%02X", idm.HammingCode))
	r = append(r, fmt.Sprintf("0x%02X", idm.ApplicationVersion))
	r = append(r, fmt.Sprintf("0x%02X", idm.ERTType))
	r = append(r, fmt.Sprintf("%d", idm.ERTSerialNumber))
	r = append(r, fmt.Sprintf("%d", idm.ConsumptionIntervalCount))
	r = append(r, fmt.Sprintf("0x%02X", idm.ModuleProgrammingState))
	r = append(r, fmt.Sprintf("%02X", idm.TamperCounters))
	r = append(r, fmt.Sprintf("0x%02X", idm.AsynchronousCounters))
	r = append(r, fmt.Sprintf("%02X", idm.PowerOutageFlags))
	r = append(r, fmt.Sprintf("%d", idm.LastConsumptionCount))
	r = append(r, idm.DifferentialConsumptionIntervals.Record()...)
	r = append(r, fmt.Sprintf("%d", idm.TransmitTimeOffset))
	r = append(r, fmt.Sprintf("0x%04X", idm.SerialNumberCRC))
	r = append(r, fmt.Sprintf("0x%04X", idm.PacketCRC))

	return
}

func (p IDMParser) Parse(data Data) (msg Message, err error) {
	var idm IDM

	if residue := p.Checksum(data.Bytes[4:]); residue != p.Residue {
		err = fmt.Errorf("checksum failed: 0x%04X", residue)
		return
	}

	idm.Preamble = binary.BigEndian.Uint32(data.Bytes[0:4])
	idm.PacketTypeID = data.Bytes[4]
	idm.PacketLength = data.Bytes[5]
	idm.HammingCode = data.Bytes[6]
	idm.ApplicationVersion = data.Bytes[7]
	idm.ERTType = data.Bytes[8] & 0x0F
	idm.ERTSerialNumber = binary.BigEndian.Uint32(data.Bytes[9:13])
	idm.ConsumptionIntervalCount = data.Bytes[13]
	idm.ModuleProgrammingState = data.Bytes[14]
	idm.TamperCounters = data.Bytes[15:21]
	idm.AsynchronousCounters = binary.BigEndian.Uint16(data.Bytes[21:23])
	idm.PowerOutageFlags = data.Bytes[23:29]
	idm.LastConsumptionCount = binary.BigEndian.Uint32(data.Bytes[29:33])

	offset := 264
	for idx := range idm.DifferentialConsumptionIntervals {
		interval, _ := strconv.ParseUint(data.Bits[offset:offset+9], 2, 9)
		idm.DifferentialConsumptionIntervals[idx] = uint16(interval)
		offset += 9
	}

	idm.TransmitTimeOffset = binary.BigEndian.Uint16(data.Bytes[86:88])
	idm.SerialNumberCRC = binary.BigEndian.Uint16(data.Bytes[88:90])
	idm.PacketCRC = binary.BigEndian.Uint16(data.Bytes[90:92])

	if idm.ERTSerialNumber == 0 {
		return idm, errors.New("invalid meter id")
	}

	return idm, nil
}

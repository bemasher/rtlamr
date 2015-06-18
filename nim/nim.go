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

package nim

import (
	"encoding/binary"
	"fmt"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/decode"
	"github.com/bemasher/rtlamr/parse"
)

func NewPacketConfig(symbolLength int) (cfg decode.PacketConfig) {
	cfg.CenterFreq = 912600155
	cfg.DataRate = 32768
	cfg.SymbolLength = symbolLength
	cfg.PreambleSymbols = 24
	cfg.PacketSymbols = 13 * 8
	cfg.Preamble = "010101010001011010100011"

	return
}

type Parser struct {
	decode.Decoder
	crc.CRC
}

func NewParser(symbolLength, decimation int, fastMag bool) (p Parser) {
	p.Decoder = decode.NewDecoder(NewPacketConfig(symbolLength), decimation, fastMag)
	// p.CRC = crc.NewCRC("BCH", 0, 0x6F63, 0)
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

		nim := NewNIM(data)

		if nim.ProtocolID != 0x1F {
			continue
		}

		if nim.HammingLength != 0x27E2 {
			continue
		}

		msgs = append(msgs, nim)
	}

	return
}

// Standard Consumption Message
type NIM struct {
	ProtocolID    uint8  `xml:",attr"`
	HammingLength uint16 `xml:",attr"`
	MessageNumber uint8  `xml:",attr"`
	EndpointType  uint8  `xml:",attr"`
	EndpointID    uint32 `xml:",attr"`
}

func NewNIM(data parse.Data) (nim NIM) {
	nim.ProtocolID = data.Bytes[3]
	nim.HammingLength = binary.BigEndian.Uint16(data.Bytes[4:6])
	nim.MessageNumber = data.Bytes[6]
	nim.EndpointType = data.Bytes[7]
	nim.EndpointID = binary.BigEndian.Uint32(data.Bytes[8:12])

	return
}

func (nim NIM) MsgType() string {
	return "NIM"
}

func (nim NIM) MeterID() uint32 {
	return nim.EndpointID
}

func (nim NIM) MeterType() uint8 {
	return nim.EndpointType
}

func (nim NIM) String() string {
	return fmt.Sprintf("{ProtocolID:0x%02X LengthHamming:0x%04X MessageNum:0x%02X EndpointType:%2d EndpointID:%10d}",
		nim.ProtocolID, nim.HammingLength, nim.MessageNumber, nim.EndpointType, nim.EndpointID,
	)
}

func (nim NIM) Record() (r []string) {
	// r = append(r, strconv.FormatUint(uint64(nim.ID), 10))
	// r = append(r, strconv.FormatUint(uint64(nim.Type), 10))
	// r = append(r, "0x"+strconv.FormatUint(uint64(nim.TamperPhy), 16))
	// r = append(r, "0x"+strconv.FormatUint(uint64(nim.TamperEnc), 16))
	// r = append(r, strconv.FormatUint(uint64(nim.Consumption), 10))
	// r = append(r, "0x"+strconv.FormatUint(uint64(nim.Checksum), 16))

	return
}

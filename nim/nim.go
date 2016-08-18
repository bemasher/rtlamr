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
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/decode"
	"github.com/bemasher/rtlamr/parse"
)

func init() {
	parse.Register("nim", NewParser)
}

func NewPacketConfig(chipLength int) (cfg decode.PacketConfig) {
	cfg.CenterFreq = 904600000
	cfg.DataRate = 35411 // chip = 56.48µs
	cfg.ChipLength = chipLength
	cfg.PreambleSymbols = 24
	cfg.PacketSymbols = 92 * 8
	cfg.Preamble = "010101010001011010100011"

	return
}

type Parser struct {
	decode.Decoder
	crc.CRC
}

func NewParser(chipLength, decimation int) (p parse.Parser) {
	return &Parser{
		decode.NewDecoder(NewPacketConfig(chipLength), decode.NewFskLUT(), 1),
		crc.NewCRC("CCITT", 0xFFFF, 0x1021, 0x1D0F),
	}
	return
}

func (p Parser) Dec() decode.Decoder {
	return p.Decoder
}

func (p *Parser) Cfg() *decode.PacketConfig {
	return &p.Decoder.Cfg
}

func (p Parser) Parse(indices []int) (msgs []parse.Message) {
	seen := make(map[string]bool)

	for _, pkt := range p.Decoder.Slice(indices) {
		s := string(pkt)
		if seen[s] {
			continue
		}
		seen[s] = true

		body := pkt[4:]

		if body[0] != 0x1F {
			continue
		}

		fmt.Printf("%02X\n", body)
		// fmt.Printf("%02X %04X\n", body, p.CRC.Checksum(body[:0x66]))

		// log.Printf("%d\n", indices[idx])

		// data := parse.NewDataFromBytes(pkt)

		// msgs = append(msgs, nim)
	}

	return
}

// AAAAAAAAAAAAAA

// Standard Consumption Message
type NIM struct {
	FrameSync     uint16 `xml:",attr"`
	ProtocolID    uint8  `xml:",attr"`
	Length        uint8  `xml:",attr"`
	HammingCode   uint8  `xml:",attr"`
	MessageNumber uint8  `xml:",attr"`
	EndpointType  uint8  `xml:",attr"`
	EndpointID    uint32 `xml:",attr"`
}

func NewNIM(data parse.Data) (nim NIM) {
	binary.Read(bytes.NewReader(data.Bytes), binary.BigEndian, &nim)

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

func (nim NIM) Checksum() []byte {
	checksum := make([]byte, 2)
	// binary.BigEndian.PutUint16(checksum, nim.ChecksumVal)
	return checksum
}

func (nim NIM) String() string {
	return fmt.Sprintf("{FrameSync:0x%04X ProtocolID:0x%02X Length:0x%02X HammingCode:0x%02X MessageNumber:0x%02X EndpointType:%02X EndpointID:%08X}",
		nim.FrameSync,
		nim.ProtocolID,
		nim.Length,
		nim.HammingCode,
		nim.MessageNumber,
		nim.EndpointType,
		nim.EndpointID,
	)
}

func (nim NIM) Record() (r []string) {
	r = append(r, "0x"+strconv.FormatUint(uint64(nim.FrameSync), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(nim.ProtocolID), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(nim.Length), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(nim.HammingCode), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(nim.MessageNumber), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(nim.EndpointType), 16))
	r = append(r, strconv.FormatUint(uint64(nim.EndpointID), 10))

	return
}

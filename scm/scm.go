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
	"encoding/binary"
	"fmt"
	"strconv"
	"sync"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/protocol"
)

func init() {
	protocol.RegisterParser("scm", NewParser)
}

type Parser struct {
	crc.CRC
	cfg  protocol.PacketConfig
	data protocol.Data
}

func NewParser(chipLength int) (p protocol.Parser) {
	return &Parser{
		CRC: crc.NewCRC("BCH", 0, 0x6F63, 0),
		cfg: protocol.PacketConfig{
			Protocol:        "scm",
			CenterFreq:      912600155,
			DataRate:        32768,
			ChipLength:      chipLength,
			PreambleSymbols: 21,
			PacketSymbols:   96,
			Preamble:        "111110010101001100000",
		},
		data: protocol.Data{Bytes: make([]byte, 96>>3)},
	}
}

func (p Parser) SetDecoder(d *protocol.Decoder) {}

func (p *Parser) Cfg() protocol.PacketConfig {
	return p.cfg
}

func (p Parser) Parse(pkts []protocol.Data, msgCh chan protocol.Message, wg *sync.WaitGroup) {
	seen := make(map[string]bool)

	for _, pkt := range pkts {
		p.data.Idx = pkt.Idx
		p.data.Bits = pkt.Bits[0:p.cfg.PacketSymbols]
		copy(p.data.Bytes, pkt.Bytes)

		s := string(p.data.Bytes)
		if seen[s] {
			continue
		}
		seen[s] = true

		// If the checksum fails, bail.
		if p.Checksum(p.data.Bytes[2:12]) != 0 {
			continue
		}

		scm := NewSCM(p.data)

		// If the meter id is 0, bail.
		if scm.ID == 0 {
			continue
		}

		msgCh <- scm
	}

	wg.Done()
}

// Standard Consumption Message
type SCM struct {
	ID          uint32 `xml:",attr"`
	Type        uint8  `xml:",attr"`
	TamperPhy   uint8  `xml:",attr"`
	TamperEnc   uint8  `xml:",attr"`
	Consumption uint32 `xml:",attr"`
	ChecksumVal uint16 `xml:"Checksum,attr"`
}

func NewSCM(data protocol.Data) (scm SCM) {
	ertid, _ := strconv.ParseUint(data.Bits[21:23]+data.Bits[56:80], 2, 26)
	erttype, _ := strconv.ParseUint(data.Bits[26:30], 2, 4)
	tamperphy, _ := strconv.ParseUint(data.Bits[24:26], 2, 2)
	tamperenc, _ := strconv.ParseUint(data.Bits[30:32], 2, 2)
	consumption, _ := strconv.ParseUint(data.Bits[32:56], 2, 24)
	checksum, _ := strconv.ParseUint(data.Bits[80:96], 2, 16)

	scm.ID = uint32(ertid)
	scm.Type = uint8(erttype)
	scm.TamperPhy = uint8(tamperphy)
	scm.TamperEnc = uint8(tamperenc)
	scm.Consumption = uint32(consumption)
	scm.ChecksumVal = uint16(checksum)

	return
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

func (scm SCM) Checksum() []byte {
	checksum := make([]byte, 2)
	binary.BigEndian.PutUint16(checksum, scm.ChecksumVal)
	return checksum
}

func (scm SCM) String() string {
	return fmt.Sprintf("{ID:%8d Type:%2d Tamper:{Phy:%02X Enc:%02X} Consumption:%8d CRC:0x%04X}",
		scm.ID, scm.Type, scm.TamperPhy, scm.TamperEnc, scm.Consumption, scm.ChecksumVal,
	)
}

func (scm SCM) Record() (r []string) {
	r = append(r, strconv.FormatUint(uint64(scm.ID), 10))
	r = append(r, strconv.FormatUint(uint64(scm.Type), 10))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.TamperPhy), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.TamperEnc), 16))
	r = append(r, strconv.FormatUint(uint64(scm.Consumption), 10))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.ChecksumVal), 16))

	return
}

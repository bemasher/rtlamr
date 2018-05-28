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

package scmplus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"sync"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/protocol"
)

func init() {
	protocol.RegisterParser("scm+", NewParser)
}

type Parser struct {
	crc.CRC
	cfg  protocol.PacketConfig
	data protocol.Data
}

func (p Parser) SetDecoder(d *protocol.Decoder) {}

func (p *Parser) Cfg() protocol.PacketConfig {
	return p.cfg
}

func NewParser(chipLength int) (p protocol.Parser) {
	return &Parser{
		CRC: crc.NewCRC("CCITT", 0xFFFF, 0x1021, 0x1D0F),
		cfg: protocol.PacketConfig{
			Protocol:        "scm+",
			CenterFreq:      912600155,
			DataRate:        32768,
			ChipLength:      chipLength,
			PreambleSymbols: 16,
			PacketSymbols:   16 * 8,
			Preamble:        "0001011010100011",
		},
		data: protocol.Data{Bytes: make([]byte, 16)},
	}
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
		if residue := p.Checksum(p.data.Bytes[2:]); residue != p.Residue {
			continue
		}

		scm := NewSCM(p.data)

		// If the EndpointID is 0 or ProtocolID is invalid, bail.
		if scm.EndpointID == 0 || scm.ProtocolID != 0x1E {
			continue
		}

		msgCh <- scm
	}

	wg.Done()
}

// Standard Consumption Message Plus
type SCM struct {
	FrameSync    uint16 `xml:",attr"`
	ProtocolID   uint8  `xml:",attr"`
	EndpointType uint8  `xml:",attr"`
	EndpointID   uint32 `xml:",attr"`
	Consumption  uint32 `xml:",attr"`
	Tamper       uint16 `xml:",attr"`
	PacketCRC    uint16 `xml:"Checksum,attr",json:"Checksum"`
}

func NewSCM(data protocol.Data) (scm SCM) {
	binary.Read(bytes.NewReader(data.Bytes), binary.BigEndian, &scm)

	return
}

func (scm SCM) MsgType() string {
	return "SCM+"
}

func (scm SCM) MeterID() uint32 {
	return scm.EndpointID
}

func (scm SCM) MeterType() uint8 {
	return scm.EndpointType
}

func (scm SCM) Checksum() []byte {
	checksum := make([]byte, 2)
	binary.BigEndian.PutUint16(checksum, scm.PacketCRC)
	return checksum
}

func (scm SCM) String() string {
	return fmt.Sprintf("{ProtocolID:0x%02X EndpointType:0x%02X EndpointID:%10d Consumption:%10d Tamper:0x%04X PacketCRC:0x%04X}",
		scm.ProtocolID,
		scm.EndpointType,
		scm.EndpointID,
		scm.Consumption,
		scm.Tamper,
		scm.PacketCRC,
	)
}

func (scm SCM) Record() (r []string) {
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.FrameSync), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.ProtocolID), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.EndpointType), 16))
	r = append(r, strconv.FormatUint(uint64(scm.EndpointID), 10))
	r = append(r, strconv.FormatUint(uint64(scm.Consumption), 10))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.Tamper), 16))
	r = append(r, "0x"+strconv.FormatUint(uint64(scm.PacketCRC), 16))

	return
}

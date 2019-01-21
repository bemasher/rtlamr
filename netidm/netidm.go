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

package netidm

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/protocol"
)

func init() {
	protocol.RegisterParser("netidm", NewParser)
}

func NewPacketConfig(chipLength int) (cfg protocol.PacketConfig) {
	cfg.CenterFreq = 912600155
	cfg.DataRate = 32768
	cfg.ChipLength = chipLength
	cfg.PreambleSymbols = 32
	cfg.PacketSymbols = 92 * 8
	cfg.Preamble = "01010101010101010001011010100011"

	return
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
			Protocol:        "netidm",
			CenterFreq:      912600155,
			DataRate:        32768,
			ChipLength:      chipLength,
			PreambleSymbols: 32,
			PacketSymbols:   92 * 8,
			Preamble:        "01010101010101010001011010100011",
		},
		data: protocol.Data{Bytes: make([]byte, 92)},
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
		if residue := p.Checksum(p.data.Bytes[4:92]); residue != p.Residue {
			continue
		}

		// If the serial checksum fails, bail.
		buf := make([]byte, 6)
		copy(buf, p.data.Bytes[9:13])
		copy(buf[4:], p.data.Bytes[88:90])
		if residue := p.Checksum(buf); residue != p.Residue {
			continue
		}

		netidm := NewNetIDM(p.data)

		// If the meter id is 0, bail.
		if netidm.ERTSerialNumber == 0 {
			continue
		}

		msgCh <- netidm
	}

	wg.Done()
}

// Net Meter Interval Data Message
type NetIDM struct {
	Preamble                         uint32 // Training and Frame sync.
	ProtocolID                       uint8
	PacketLength                     uint8 // Packet Length MSB
	HammingCode                      uint8 // Packet Length LSB
	ApplicationVersion               uint8
	ERTType                          uint8
	ERTSerialNumber                  uint32
	ConsumptionIntervalCount         uint8
	ProgrammingState                 uint8
	LastGeneration                   uint32
	LastConsumption                  uint32
	LastConsumptionNet               uint32
	DifferentialConsumptionIntervals Interval // 53 Bytes
	TransmitTimeOffset               uint16
	SerialNumberCRC                  uint16
	PacketCRC                        uint16
}

func NewNetIDM(data protocol.Data) (netidm NetIDM) {
	netidm.Preamble = binary.BigEndian.Uint32(data.Bytes[0:4])
	netidm.ProtocolID = data.Bytes[4]
	netidm.PacketLength = data.Bytes[5]
	netidm.HammingCode = data.Bytes[6]
	netidm.ApplicationVersion = data.Bytes[7]
	netidm.ERTType = data.Bytes[8] & 0x0F
	netidm.ERTSerialNumber = binary.BigEndian.Uint32(data.Bytes[9:13])
	netidm.ConsumptionIntervalCount = data.Bytes[13]
	netidm.ProgrammingState = data.Bytes[14]

	netidm.LastConsumption = uint32(data.Bytes[25])<<16 | uint32(data.Bytes[26])<<8 | uint32(data.Bytes[27])
	netidm.LastGeneration = uint32(data.Bytes[28])<<16 | uint32(data.Bytes[29])<<8 | uint32(data.Bytes[30])
	netidm.LastConsumptionNet = binary.BigEndian.Uint32(data.Bytes[34:38])

	offset := 38 << 3
	for idx := range netidm.DifferentialConsumptionIntervals {
		in, _ := strconv.ParseUint(data.Bits[offset:offset+14], 2, 14)
		netidm.DifferentialConsumptionIntervals[idx] = uint16(in)

		offset += 14
	}

	netidm.TransmitTimeOffset = binary.BigEndian.Uint16(data.Bytes[86:88])
	netidm.SerialNumberCRC = binary.BigEndian.Uint16(data.Bytes[88:90])
	netidm.PacketCRC = binary.BigEndian.Uint16(data.Bytes[90:92])

	return
}

type Interval [27]uint16

func (interval Interval) Record() (r []string) {
	for _, val := range interval {
		r = append(r, strconv.FormatUint(uint64(val), 10))
	}
	return
}

func (netidm NetIDM) MsgType() string {
	return "NetIDM"
}

func (netidm NetIDM) MeterID() uint32 {
	return netidm.ERTSerialNumber
}

func (netidm NetIDM) MeterType() uint8 {
	return netidm.ERTType
}

func (netidm NetIDM) Checksum() []byte {
	checksum := make([]byte, 2)
	binary.BigEndian.PutUint16(checksum, netidm.PacketCRC)
	return checksum
}

func (netidm NetIDM) String() string {
	var fields []string

	fields = append(fields, fmt.Sprintf("Preamble:0x%08X", netidm.Preamble))
	fields = append(fields, fmt.Sprintf("ProtocolID:0x%02X", netidm.ProtocolID))
	fields = append(fields, fmt.Sprintf("PacketLength:0x%02X", netidm.PacketLength))
	fields = append(fields, fmt.Sprintf("HammingCode:0x%02X", netidm.HammingCode))
	fields = append(fields, fmt.Sprintf("ApplicationVersion:0x%02X", netidm.ApplicationVersion))
	fields = append(fields, fmt.Sprintf("ERTType:0x%02X", netidm.ERTType))
	fields = append(fields, fmt.Sprintf("ERTSerialNumber:% 10d", netidm.ERTSerialNumber))
	fields = append(fields, fmt.Sprintf("ConsumptionIntervalCount:%d", netidm.ConsumptionIntervalCount))
	fields = append(fields, fmt.Sprintf("ProgrammingState:0x%02X", netidm.ProgrammingState))
	fields = append(fields, fmt.Sprintf("LastGeneration:%d", netidm.LastGeneration))
	fields = append(fields, fmt.Sprintf("LastConsumption:%d", netidm.LastConsumption))
	fields = append(fields, fmt.Sprintf("LastConsumptionNet:%d", netidm.LastConsumptionNet))
	fields = append(fields, fmt.Sprintf("DifferentialConsumptionIntervals:%d", netidm.DifferentialConsumptionIntervals))
	fields = append(fields, fmt.Sprintf("TransmitTimeOffset:%d", netidm.TransmitTimeOffset))
	fields = append(fields, fmt.Sprintf("SerialNumberCRC:0x%04X", netidm.SerialNumberCRC))
	fields = append(fields, fmt.Sprintf("PacketCRC:0x%04X", netidm.PacketCRC))

	return "{" + strings.Join(fields, " ") + "}"
}

func (netidm NetIDM) Record() (r []string) {
	r = append(r, fmt.Sprintf("0x%08X", netidm.Preamble))
	r = append(r, fmt.Sprintf("0x%02X", netidm.ProtocolID))
	r = append(r, fmt.Sprintf("0x%02X", netidm.PacketLength))
	r = append(r, fmt.Sprintf("0x%02X", netidm.HammingCode))
	r = append(r, fmt.Sprintf("0x%02X", netidm.ApplicationVersion))
	r = append(r, fmt.Sprintf("0x%02X", netidm.ERTType))
	r = append(r, fmt.Sprintf("%d", netidm.ERTSerialNumber))
	r = append(r, fmt.Sprintf("%d", netidm.ConsumptionIntervalCount))
	r = append(r, fmt.Sprintf("0x%02X", netidm.ProgrammingState))
	r = append(r, fmt.Sprintf("%d", netidm.LastGeneration))
	r = append(r, fmt.Sprintf("%d", netidm.LastConsumption))
	r = append(r, fmt.Sprintf("%d", netidm.LastConsumptionNet))
	r = append(r, netidm.DifferentialConsumptionIntervals.Record()...)
	r = append(r, fmt.Sprintf("%d", netidm.TransmitTimeOffset))
	r = append(r, fmt.Sprintf("0x%04X", netidm.SerialNumberCRC))
	r = append(r, fmt.Sprintf("0x%04X", netidm.PacketCRC))

	return
}

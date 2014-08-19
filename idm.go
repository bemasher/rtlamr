package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bemasher/rtlamr/crc"
	"github.com/bemasher/rtlamr/preamble"
)

type IDMDecoder struct {
	pd  preamble.PreambleDetector
	crc crc.CRC

	cfg PacketConfig
}

func (idm IDMDecoder) String() string {
	return fmt.Sprintf("{Packetconfig:%s CRC:%s}", idm.cfg, idm.crc)
}

func NewIDMDecoder(symbolLength int) (idm IDMDecoder) {
	var pc PacketConfig

	pc.SymbolLength = symbolLength

	pc.PreambleSymbols = 64
	pc.PacketSymbols = (92 * 8) << 1

	pc.PreambleLength = pc.PreambleSymbols * uint(pc.SymbolLength)
	pc.PacketLength = pc.PacketSymbols * uint(pc.SymbolLength)
	pc.BlockSize = NextPowerOf2(pc.PreambleLength)

	pc.SampleRate = DataRate * uint(pc.SymbolLength)

	pc.PreambleBits = strconv.FormatUint(0x555516A3, 2)
	pc.PreambleBits = strings.Repeat("0", int(pc.PreambleSymbols>>1)-len(pc.PreambleBits)) + pc.PreambleBits

	idm.cfg = pc

	idm.pd = preamble.NewPreambleDetector(uint(pc.BlockSize<<1), pc.SymbolLength, pc.PreambleBits)
	idm.crc = crc.NewCRC("CCITT", 0xFFFF, 0x1021, 0x1D0F)

	return
}

func (idm IDMDecoder) Close() {
	idm.pd.Close()
}

func (idm IDMDecoder) PacketConfig() PacketConfig {
	return idm.cfg
}

func (idm IDMDecoder) CRC() crc.CRC {
	return idm.crc
}

func (idm IDMDecoder) SearchPreamble(buf []float64) int {
	idm.pd.Execute(buf)
	return idm.pd.ArgMax()
}

// Standard Consumption Message
type IDM struct {
	Preamble                 uint32 // Training and Frame sync.
	PacketTypeID             uint8
	PacketLength             uint8 // Packet Length MSB
	HammingCode              uint8 // Packet Length LSB
	ApplicationVersion       uint8
	ERTType                  uint8
	ERTSerialNumber          uint32
	ConsumptionIntervalCount uint8
	ModuleProgrammingState   uint8
	TamperCounters           []byte // 6 Bytes
	AsynchronousCounters     uint16
	PowerOutageFlags         []byte // 6 Bytes
	LastConsumptionCount     uint32
	// DifferentialConsumptionIntervals [47]uint16 // 53 Bytes
	DifferentialConsumptionIntervals Interval // 53 Bytes
	TransmitTimeOffset               uint16
	SerialNumberCRC                  uint16
	PacketCRC                        uint16
}

type Interval [47]uint16

func (interval Interval) MarshalText() (text []byte, err error) {
	return []byte(fmt.Sprintf("%+v", interval)), nil
}

func (idm IDM) String() string {
	indented, _ := json.MarshalIndent(idm, "", "\t")
	return string(indented)

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

func (idmd IDMDecoder) Decode(data Data) (fmt.Stringer, error) {
	var idm IDM

	if len(data.Bits) != int(idmd.cfg.PacketSymbols>>1) {
		return idm, fmt.Errorf("invalid input length, expected %d got %d", int(idmd.cfg.PacketSymbols>>1), len(data.Bits))
	}

	idm.Preamble = binary.BigEndian.Uint32(data.Bytes[0:4])
	idm.PacketTypeID = data.Bytes[4]
	idm.PacketLength = data.Bytes[5]
	idm.HammingCode = data.Bytes[6]
	idm.ApplicationVersion = data.Bytes[7]
	idm.ERTType = data.Bytes[8]
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

	if idmd.crc.Checksum(data.Bytes[4:]) != idmd.crc.Residue {
		return idm, errors.New("checksum failed")
	}

	// if idm.ERTSerialNumber == 0 {
	if idm.ERTSerialNumber == 0 || idm.ERTSerialNumber != 17581447 {
		return idm, errors.New("invalid meter id")
	}

	return idm, nil
}

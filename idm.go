package main

import (
	"encoding/binary"
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

	pktConfig PacketConfig
}

func (idm IDMDecoder) String() string {
	return fmt.Sprintf("{Packetconfig:%s CRC:%s}", idm.pktConfig, idm.crc)
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

	idm.pktConfig = pc

	idm.pd = preamble.NewPreambleDetector(uint(pc.BlockSize<<1), pc.SymbolLength, pc.PreambleBits)
	idm.crc = crc.NewCRC("CCITT", 0xFFFF, 0x1021, 0x1D0F)

	return
}

func (idm IDMDecoder) Close() {
	idm.pd.Close()
}

func (idm IDMDecoder) PacketConfig() PacketConfig {
	return idm.pktConfig
}

func (idm IDMDecoder) SearchPreamble(buf []float64) int {
	idm.pd.Execute(buf)
	return idm.pd.ArgMax()
}

// Standard Consumption Message
type IDM struct {
	Preamble        uint32
	PacketType      uint8
	PacketLength    uint8
	AppVersion      uint8
	CommodityType   uint8
	SerialNumber    uint32
	SerialNumberCRC uint16
	PacketCRC       uint16
}

func (idm IDM) String() string {
	return fmt.Sprintf("{Preamble:%08X PktType:%02X PktLen:%2d AppVer:%02X CommType:%02X Serial:% 10d SerCRC:%04X PktCRC:%04X}",
		idm.Preamble,
		idm.PacketType,
		idm.PacketLength,
		idm.AppVersion,
		idm.CommodityType,
		idm.SerialNumber,
		idm.SerialNumberCRC,
		idm.PacketCRC,
	)
}

func (idmd IDMDecoder) Decode(data Data) (fmt.Stringer, error) {
	var idm IDM

	if len(data.Bits) != int(idmd.pktConfig.PacketSymbols>>1) {
		return idm, errors.New("invalid input length")
	}

	idm.Preamble = binary.BigEndian.Uint32(data.Bytes[0:4])
	idm.PacketType = data.Bytes[4]
	idm.PacketLength = data.Bytes[5]
	idm.AppVersion = data.Bytes[7]
	idm.CommodityType = data.Bytes[8] & 0x0F
	idm.SerialNumber = binary.BigEndian.Uint32(data.Bytes[9:13])
	idm.SerialNumberCRC = binary.BigEndian.Uint16(data.Bytes[88:90])
	idm.PacketCRC = binary.BigEndian.Uint16(data.Bytes[90:92])

	if idmd.crc.Checksum(data.Bytes[4:]) != idmd.crc.Residue {
		return idm, errors.New("checksum failed")
	}

	if idm.SerialNumber == 0 {
		return idm, errors.New("invalid meter id")
	}

	return idm, nil
}

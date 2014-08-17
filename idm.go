package main

import (
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
	pc.PreambleBits += strings.Repeat("0", int(pc.PreambleSymbols>>1)-len(pc.PreambleBits))

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
	ID     uint32
	Type   uint8
	Tamper struct {
		Phy uint8
		Enc uint8
	}
	Consumption uint32
	Checksum    uint16
}

func (scm IDM) String() string {
	return fmt.Sprintf("{ID:%8d Type:%2d Tamper:{Phy:%d Enc:%d} Consumption:%8d Checksum:0x%04X}",
		scm.ID, scm.Type, scm.Tamper.Phy, scm.Tamper.Enc, scm.Consumption, scm.Checksum,
	)
}

func (idm IDMDecoder) Decode(data Data) (fmt.Stringer, error) {
	var scm IDM

	if len(data.Bits) != int(idm.pktConfig.PacketSymbols>>1) {
		return scm, errors.New("invalid input length")
	}

	if idm.crc.Checksum(data.Bytes[2:]) != 0 {
		return scm, errors.New("checksum failed")
	}

	id, _ := strconv.ParseUint(data.Bits[21:23]+data.Bits[56:80], 2, 32)
	ertType, _ := strconv.ParseUint(data.Bits[26:30], 2, 8)
	tamperPhy, _ := strconv.ParseUint(data.Bits[24:26], 2, 8)
	tamperEnc, _ := strconv.ParseUint(data.Bits[30:32], 2, 8)
	consumption, _ := strconv.ParseUint(data.Bits[32:56], 2, 32)
	checksum, _ := strconv.ParseUint(data.Bits[80:96], 2, 16)

	scm.ID = uint32(id)
	scm.Type = uint8(ertType)
	scm.Tamper.Phy = uint8(tamperPhy)
	scm.Tamper.Enc = uint8(tamperEnc)
	scm.Consumption = uint32(consumption)
	scm.Checksum = uint16(checksum)

	if scm.ID == 0 {
		return scm, errors.New("invalid meter id")
	}

	return scm, nil
}

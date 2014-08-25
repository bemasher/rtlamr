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

func (p SCMParser) Parse(data Data) (msg interface{}, err error) {
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
	ID          uint32
	Type        uint8
	TamperPhy   uint8
	TamperEnc   uint8
	Consumption uint32
	Checksum    uint16
}

func (scm SCM) String() string {
	return fmt.Sprintf("{ID:%8d Type:%2d TamperPhy:%02X TamperEnd:%02X Consumption:%8d Checksum:0x%04X}",
		scm.ID, scm.Type, scm.TamperPhy, scm.TamperEnc, scm.Consumption, scm.Checksum,
	)
}

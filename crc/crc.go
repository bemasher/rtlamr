package crc

import "fmt"

type CRC struct {
	Name    string
	Init    uint16
	Poly    uint16
	Residue uint16

	tbl Table
}

func NewCRC(name string, init, poly, residue uint16) (crc CRC) {
	crc.Name = name
	crc.Init = init
	crc.Poly = poly
	crc.Residue = residue
	crc.tbl = NewTable(crc.Poly)

	return
}

func (crc CRC) String() string {
	return fmt.Sprintf("{Name:%s Init:0x%04X Poly:0x%04X Residue:0x%04X}", crc.Name, crc.Init, crc.Poly, crc.Residue)
}

func (crc CRC) Checksum(data []byte) uint16 {
	return Checksum(crc.Init, data, crc.tbl)
}

type Table [256]uint16

func NewTable(poly uint16) (table Table) {
	for tIdx := range table {
		crc := uint16(tIdx) << 8
		for bIdx := 0; bIdx < 8; bIdx++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ poly
			} else {
				crc = crc << 1
			}
		}
		table[tIdx] = crc
	}
	return table
}

func Checksum(init uint16, data []byte, table Table) (crc uint16) {
	crc = init
	for _, v := range data {
		crc = crc<<8 ^ table[crc>>8^uint16(v)]
	}
	return
}

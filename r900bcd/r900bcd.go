// RTLAMR - An rtl-sdr receiver for smart meters operating in the 900MHz ISM band.
// Copyright (C) 2014 Douglas Hall
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

package r900bcd

import (
	"strconv"

	"github.com/bemasher/rtlamr/parse"
	"github.com/bemasher/rtlamr/r900"
)

func init() {
	parse.Register("r900bcd", NewParser)
}

type Parser struct {
	parse.Parser
}

func NewParser(ChipLength, decimation int) parse.Parser {
	return Parser{r900.NewParser(ChipLength, decimation)}
}

// Parse messages using r900 parser and convert consumption from BCD to int.
func (p Parser) Parse(indices []int) (msgs []parse.Message) {
	msgs = p.Parser.Parse(indices)
	for idx, msg := range msgs {
		r900msg := msg.(r900.R900)
		hex := strconv.FormatUint(uint64(r900msg.Consumption), 16)
		consumption, _ := strconv.ParseUint(hex, 10, 32)
		r900msg.Consumption = uint32(consumption)
		msgs[idx] = r900msg
	}
	return
}

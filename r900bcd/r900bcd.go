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
	"sync"

	"github.com/bemasher/rtlamr/protocol"
	"github.com/bemasher/rtlamr/r900"
)

func init() {
	protocol.RegisterParser("r900bcd", NewParser)
}

type Parser struct {
	protocol.Parser
}

func NewParser(ChipLength int) protocol.Parser {
	return Parser{r900.NewParser(ChipLength)}
}

type R900BCD struct {
	r900.R900
}

func (r R900BCD) MsgType() string {
	return "R900BCD"
}

// Parse messages using r900 parser and convert consumption from BCD to int.
func (p Parser) Parse(pkts []protocol.Data, msgCh chan protocol.Message, wg *sync.WaitGroup) {
	localWg := new(sync.WaitGroup)

	localMsgCh := make(chan protocol.Message)
	localWg.Add(1)

	go func() {
		localWg.Wait()
		close(localMsgCh)
	}()

	go p.Parser.Parse(pkts, localMsgCh, localWg)

	for msg := range localMsgCh {
		r900bcd := R900BCD{msg.(r900.R900)}
		hex := strconv.FormatUint(uint64(r900bcd.Consumption), 16)
		consumption, _ := strconv.ParseUint(hex, 10, 32)
		r900bcd.Consumption = uint32(consumption)
		msgCh <- r900bcd
	}

	wg.Done()
}

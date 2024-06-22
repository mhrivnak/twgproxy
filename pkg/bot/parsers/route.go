package parsers

import (
	"fmt"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewRouteParser(broker *events.Broker) Parser {
	return &parseRoute{broker: broker}
}

type parseRoute struct {
	lines  []string
	broker *events.Broker
}

func (p *parseRoute) Parse(line string) error {
	p.lines = append(p.lines, strings.TrimSpace(line))
	return nil
}

func (p *parseRoute) Done() bool {
	length := len(p.lines)
	if length < 2 {
		return false
	}
	last := p.lines[length-1]
	if last == "" {
		p.finalize()
		return true
	}
	return false
}

func (p *parseRoute) finalize() {
	rLines := p.lines[1 : len(p.lines)-1]
	route := strings.Join(rLines, " ")
	sectors := strings.Split(route, " ")
	p.broker.Publish(&events.Event{
		Kind: events.ROUTEDISPLAY,
		Data: route,
		// create a key so async waiters can get the correct route
		ID: fmt.Sprintf("%s:%s", sectors[0], strings.Trim(sectors[len(sectors)-1], "()")),
	})
}

package parsers

import (
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
	p.broker.Publish(&events.Event{
		Kind: events.ROUTEDISPLAY,
		Data: route,
	})
}

package parsers

import (
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewRobResultParser(broker *events.Broker) Parser {
	return &parseRobResult{broker: broker}
}

type parseRobResult struct {
	lines  []string
	broker *events.Broker
}

func (p *parseRobResult) Parse(line string) error {
	p.lines = append(p.lines, strings.TrimSpace(line))
	return nil
}

func (p *parseRobResult) Done() bool {
	length := len(p.lines)
	last := p.lines[length-1]
	if strings.HasPrefix(last, "You connect to their control computer to siphon the funds out") {
		p.finalize()
		return true
	}
	return false
}

func (p *parseRobResult) finalize() {
	for _, line := range p.lines {
		if strings.HasPrefix(line, "You connect to their control computer to siphon the funds out") {
			if strings.Contains(line, "Success") {
				p.broker.Publish(&events.Event{
					Kind: events.ROBRESULT,
					ID:   string(events.ROBSUCCESS),
				})
			} else if strings.Contains(line, "Suddenly") {
				p.broker.Publish(&events.Event{
					Kind: events.ROBRESULT,
					ID:   string(events.ROBBUSTED),
				})
			}
		}
	}
}

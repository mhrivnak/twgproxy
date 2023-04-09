package parsers

import (
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewStealResultParser(broker *events.Broker, sector int) Parser {
	return &parseStealResult{
		broker: broker,
		sector: sector,
	}
}

type parseStealResult struct {
	lines  []string
	broker *events.Broker
	sector int
}

func (p *parseStealResult) Parse(line string) error {
	p.lines = append(p.lines, strings.TrimSpace(line))
	return nil
}

func (p *parseStealResult) Done() bool {
	length := len(p.lines)
	last := p.lines[length-1]
	if strings.HasPrefix(last, "You start your droids loading the cargo") {
		p.finalize()
		return true
	}
	return false
}

func (p *parseStealResult) finalize() {
	for _, line := range p.lines {
		if strings.HasPrefix(line, "You start your droids loading the cargo") {
			if strings.Contains(line, "Success") {
				p.broker.Publish(&events.Event{
					Kind: events.STEALRESULT,
					ID:   string(events.CRIMESUCCESS),
				})
			} else if strings.Contains(line, "Suddenly") {
				p.broker.Publish(&events.Event{
					Kind:    events.STEALRESULT,
					ID:      string(events.CRIMEBUSTED),
					DataInt: p.sector,
				})
			}
		}
	}
}

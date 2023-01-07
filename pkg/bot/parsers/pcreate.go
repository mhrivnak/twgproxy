package parsers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewPCreateParser(broker *events.Broker) Parser {
	return &ParsePCreate{
		broker: broker,
	}
}

type ParsePCreate struct {
	lines  []string
	broker *events.Broker
}

var pCreateInfo *regexp.Regexp = regexp.MustCompile(`Class (.)`)

func (p *ParsePCreate) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *ParsePCreate) Done() bool {
	length := len(p.lines)
	last := p.lines[length-1]
	if strings.HasPrefix(last, "What do you want to name this planet?") {
		p.finalize()
		return true
	}
	return false
}

func (p *ParsePCreate) finalize() {
	last := p.lines[len(p.lines)-1]
	parts := pCreateInfo.FindStringSubmatch(last)
	if len(parts) != 2 {
		fmt.Println("failed to match planet create type")
		return
	}

	p.broker.Publish(&events.Event{
		Kind: events.PLANETCREATE,
		ID:   parts[1],
	})
}

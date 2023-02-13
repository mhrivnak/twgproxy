package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewParseBuyDetonators(broker *events.Broker) Parser {
	return &ParseBuyDetonators{
		broker: broker,
	}
}

type ParseBuyDetonators struct {
	lines  []string
	broker *events.Broker
}

var buyDetonatorPrompt *regexp.Regexp = regexp.MustCompile(`How many Atomic Detonators do you want \(Max ([0-9]+)\)`)

func (p *ParseBuyDetonators) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *ParseBuyDetonators) Done() bool {
	length := len(p.lines)
	last := p.lines[length-1]
	if strings.HasPrefix(last, "How many Atomic Detonators do you want") {
		p.finalize()
		return true
	}
	return false
}

func (p *ParseBuyDetonators) finalize() {
	length := len(p.lines)
	last := p.lines[length-1]

	parts := buyDetonatorPrompt.FindStringSubmatch(last)
	if len(parts) != 2 {
		fmt.Println("failed to parse detonator prompt")
		return
	}

	max, err := strconv.Atoi(parts[1])
	if err != nil {
		fmt.Println("failed to parse max from detonator prompt")
		return
	}

	p.broker.Publish(&events.Event{
		Kind:    events.DETONATORBUYMAX,
		DataInt: max,
	})
}

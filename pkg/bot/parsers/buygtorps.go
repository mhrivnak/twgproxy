package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewParseBuyGTorp(broker *events.Broker) Parser {
	return &ParseBuyGTorps{
		broker: broker,
	}
}

type ParseBuyGTorps struct {
	lines  []string
	broker *events.Broker
}

var buyGTorpPrompt *regexp.Regexp = regexp.MustCompile(`How many Genesis Torpedoes do you want \(Max ([0-9]+)\)`)

func (p *ParseBuyGTorps) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *ParseBuyGTorps) Done() bool {
	length := len(p.lines)
	last := p.lines[length-1]
	if strings.HasPrefix(last, "How many Genesis Torpedoes do you want") {
		p.finalize()
		return true
	}
	return false
}

func (p *ParseBuyGTorps) finalize() {
	length := len(p.lines)
	last := p.lines[length-1]

	parts := buyGTorpPrompt.FindStringSubmatch(last)
	if len(parts) != 2 {
		fmt.Println("failed to parse gtorp purchase prompt")
		return
	}

	max, err := strconv.Atoi(parts[1])
	if err != nil {
		fmt.Println("failed to parse max from gtorp purchase prompt")
		return
	}

	p.broker.Publish(&events.Event{
		Kind:    events.GTORPBUYMAX,
		DataInt: max,
	})
}

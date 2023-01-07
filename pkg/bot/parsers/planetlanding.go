package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewPlanetLandingParser(broker *events.Broker) Parser {
	return &ParsePlanetLanding{
		broker: broker,
	}
}

type ParsePlanetLanding struct {
	lines  []string
	broker *events.Broker
}

var pLandingInfo *regexp.Regexp = regexp.MustCompile(` +< +([0-9]+)>`)

func (p *ParsePlanetLanding) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *ParsePlanetLanding) Done() bool {
	length := len(p.lines)
	last := p.lines[length-1]
	if strings.TrimSpace(last) == "" {
		p.finalize()
		return true
	}
	return false
}

func (p *ParsePlanetLanding) finalize() {
	planets := []int{}
	for _, line := range p.lines {
		parts := pLandingInfo.FindStringSubmatch(line)
		if len(parts) != 2 {
			continue
		}
		pid, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Printf("failed to parse planet ID: %s\n", err.Error())
			return
		}
		planets = append(planets, pid)
	}

	p.broker.Publish(&events.Event{
		Kind:         events.PLANETLANDINGDISPLAY,
		DataSliceInt: planets,
	})
}

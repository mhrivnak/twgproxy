package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewFigDeployParser(broker *events.Broker) Parser {
	return &parseFigDeploy{broker: broker}
}

type parseFigDeploy struct {
	lines  []string
	broker *events.Broker
}

var figDeployInfo *regexp.Regexp = regexp.MustCompile(`^You have ([0-9,]+) fighters available.`)

func (p *parseFigDeploy) Parse(line string) error {
	p.lines = append(p.lines, strings.TrimSpace(line))
	return nil
}

func (p *parseFigDeploy) Done() bool {
	length := len(p.lines)
	last := p.lines[length-1]
	if strings.HasPrefix(last, "Your ship can support up to") {
		p.finalize()
		return true
	}
	return false
}

func (p *parseFigDeploy) finalize() {
	var found bool
	var available int
	var err error

	for _, line := range p.lines {
		parts := figDeployInfo.FindStringSubmatch(line)
		if len(parts) == 0 {
			continue
		}
		found = true
		available, err = strconv.Atoi(removeCommas(parts[1]))
		if err != nil {
			fmt.Printf("failed to parse available figs: %s\n", err.Error())
			return
		}
		break
	}

	if found {
		p.broker.Publish(&events.Event{
			Kind:    events.FIGDEPLOY,
			DataInt: available,
		})
	}
}

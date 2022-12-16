package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func NewPortRobParser(data *models.Data, broker *events.Broker) Parser {
	return &ParsePortRob{
		data:   data,
		broker: broker,
	}
}

type ParsePortRob struct {
	lines  []string
	data   *models.Data
	broker *events.Broker
}

var portRobCreds *regexp.Regexp = regexp.MustCompile(`^The Trade Journals estimate this port has in excess of ([0-9,]+) creds onhand\.`)

func (p *ParsePortRob) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *ParsePortRob) Done() bool {
	length := len(p.lines)
	if length < 1 {
		return false
	}
	last := p.lines[length-1]
	switch {
	case strings.HasPrefix(last, "The Trade Journals estimate this port has"):
		p.finalize()
		return true
	default:
		return false
	}
}

func (p *ParsePortRob) finalize() {
	if len(p.lines) < 1 {
		return
	}

	parts := portRobCreds.FindStringSubmatch(p.lines[0])
	if len(parts) != 2 {
		fmt.Println("no match for port rob creds")
		return
	}

	creds, err := strconv.Atoi(removeCommas(parts[1]))
	if err != nil {
		fmt.Println("could not convert port rob creds to int")
		return
	}

	sector, ok := p.data.Sectors[p.data.Status.Sector]
	if !ok {
		fmt.Printf("sector %d not found for port rob\n", p.data.Status.Sector)
		return
	}

	if sector.Port == nil {
		fmt.Println("sector has nil port")
		return
	}
	sector.Port.Creds = creds

	p.broker.Publish(&events.Event{
		Kind:    events.PORTROBCREDS,
		ID:      fmt.Sprint(sector),
		DataInt: creds,
	})
}

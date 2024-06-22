package parsers

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

var xportRange *regexp.Regexp = regexp.MustCompile(` has a transport range of ([0-9]+) hops.`)

func NewParseXportRange(broker *events.Broker, data *models.Data) Parser {
	return &ParseXportRange{
		broker: broker,
		data:   data,
	}
}

type ParseXportRange struct {
	broker *events.Broker
	data   *models.Data
	done   bool
}

func (p *ParseXportRange) Parse(line string) error {
	parts := xportRange.FindStringSubmatch(line)
	if len(parts) == 2 {
		p.done = true
		x, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Printf("could not parse xport range: %s\n", err.Error())
			return err
		}
		p.broker.Publish(&events.Event{
			Kind:    events.XPORTRANGE,
			DataInt: x,
		})
		ship, ok := p.data.GetShip(p.data.Status.Ship)
		if ok {
			ship.XportRange = x
		}
		return nil
	}
	return nil
}

func (p *ParseXportRange) Done() bool {
	return p.done
}

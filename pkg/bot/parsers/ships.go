package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

var xportRange *regexp.Regexp = regexp.MustCompile(` has a transport range of ([0-9]+) hops.`)

func NewParseAvailableShipScan(broker *events.Broker, data *models.Data) Parser {
	return &ParseAvailableShipScan{
		broker: broker,
		data:   data,
	}
}

type ParseAvailableShipScan struct {
	broker       *events.Broker
	data         *models.Data
	done         bool
	startParsing bool
}

func (p *ParseAvailableShipScan) Parse(line string) error {
	parts := xportRange.FindStringSubmatch(line)
	if len(parts) == 2 {
		x, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Printf("could not parse xport range: %s\n", err.Error())
			p.done = true
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

	if strings.TrimSpace(line) == "-----------------------------------------------------------------------------" {
		p.startParsing = true
		return nil
	}

	if strings.TrimSpace(line) == "" && p.startParsing {
		p.done = true
		return nil
	}

	if p.startParsing && len(line) > 11 {
		shipID, err := strconv.Atoi(strings.TrimSpace(line[:4]))
		if err != nil {
			fmt.Printf("unable to parse ship ID: %s\n", err.Error())
			return err
		}
		sector, err := strconv.Atoi(strings.TrimSpace(line[5:10]))
		if err != nil {
			fmt.Printf("unable to parse ship sector: %s\n", err.Error())
			return err
		}
		ship := models.Ship{
			ID:     shipID,
			Sector: sector,
		}

		p.data.PutShip(&ship)

		p.broker.Publish(&events.Event{
			Kind:    events.AVAILABLESHIPS,
			ID:      fmt.Sprint(shipID),
			DataInt: sector,
		})
	}

	return nil
}

func (p *ParseAvailableShipScan) Done() bool {
	return p.done
}

package parsers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

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
	if strings.TrimSpace(line) == "-----------------------------------------------------------------------------" {
		p.startParsing = true
		return nil
	}

	if strings.TrimSpace(line) == "" {
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

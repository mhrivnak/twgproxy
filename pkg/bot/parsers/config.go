package parsers

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func NewParseConfig(broker *events.Broker, data *models.Data) Parser {
	return &ParseConfig{
		broker: broker,
		data:   data,
	}
}

type ParseConfig struct {
	broker *events.Broker
	data   *models.Data
	done   bool
	count  int
}

var starDockConfig *regexp.Regexp = regexp.MustCompile(`The StarDock is located in sector ([0-9]+)\.`)

func (p *ParseConfig) Parse(line string) error {
	parts := starDockConfig.FindStringSubmatch(line)
	if len(parts) == 2 {
		sector, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Println("failed to parse stardock sector from game config")
			return err
		}
		p.data.Status.StarDock = sector
		p.broker.Publish(&events.Event{
			Kind: events.CONFIGDISPLAY,
		})
		p.done = true
		return nil
	}
	p.count += 1
	if p.count > 10 {
		p.done = true
		fmt.Println("did not see stardock setting in game config")
	}
	return nil
}

func (p *ParseConfig) Done() bool {
	return p.done
}

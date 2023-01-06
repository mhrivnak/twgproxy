package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func NewParseDensityScan(data *models.Data, broker *events.Broker) Parser {
	return &ParseDensityScan{
		data:   data,
		broker: broker,
	}
}

type ParseDensityScan struct {
	lines  []string
	data   *models.Data
	broker *events.Broker
}

var densityInfo *regexp.Regexp = regexp.MustCompile(`^Sector +([0-9]+) +==> +([0-9]+)  Warps : ([0-9]+)`)

func (p *ParseDensityScan) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *ParseDensityScan) Done() bool {
	length := len(p.lines)
	if length < 3 {
		return false
	}
	last := p.lines[length-1]
	if strings.TrimSpace(last) == "" {
		p.finalize()
		return true
	}
	return false
}

func (p *ParseDensityScan) finalize() {
	for _, line := range p.lines[2:] {
		parts := densityInfo.FindStringSubmatch(line)
		if len(parts) != 4 {
			continue
		}

		sID, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Printf("failed to parse sector: %s\n", err.Error())
			return
		}

		sector, ok := p.data.Sectors[sID]
		if !ok {
			// this parser just adds data to an existing Sector. It doesn't
			// collect enough info to make a brand new Sector entry.
			continue
		}

		density, err := strconv.Atoi(parts[2])
		if err != nil {
			fmt.Printf("failed to parse density: %s\n", err.Error())
			return
		}
		sector.Density = density

		warpCount, err := strconv.Atoi(parts[3])
		if err != nil {
			fmt.Printf("failed to parse warp count: %s\n", err.Error())
			return
		}
		sector.WarpCount = warpCount
	}

	p.broker.Publish(&events.Event{
		Kind: events.DENSITYDISPLAY,
	})
}

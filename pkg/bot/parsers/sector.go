package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func NewSectorParser(data *models.Data, broker *events.Broker) Parser {
	return &ParseSector{
		data:   data,
		broker: broker,
	}
}

type ParseSector struct {
	lines  []string
	data   *models.Data
	broker *events.Broker
}

var sectorInfo *regexp.Regexp = regexp.MustCompile(`^Sector  : (\d+)`)
var portType *regexp.Regexp = regexp.MustCompile(`^Ports   : [a-zA-Z '-]+, Class \d \(([SB]{3})\)`)
var figInfo *regexp.Regexp = regexp.MustCompile(`^Fighters: ([0-9,]+) \((.+?)\) \[([A-Za-z]+)\]`)
var minesInfo *regexp.Regexp = regexp.MustCompile(`^Mines   : ([0-9]+) \(Type 1 Armid\) \(([A-Za-z ]+)\)`)
var warpsInfo *regexp.Regexp = regexp.MustCompile(`^Warps to Sector\(s\) :  ([0-9 -\(\)]+)`)

func (p *ParseSector) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *ParseSector) Done() bool {
	length := len(p.lines)
	if length < 1 {
		return false
	}
	last := p.lines[length-1]
	switch {
	case strings.HasPrefix(last, "Warps to Sector(s) :"):
		p.finalize()
		return true
	case strings.TrimSpace(last) == "":
		p.finalize()
		return true
	default:
		return false
	}
}

func (p *ParseSector) finalize() {
	if len(p.lines) < 2 {
		return
	}
	parts := sectorInfo.FindStringSubmatch(p.lines[0])
	if len(parts) != 2 {
		return
	}
	num, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	s := models.Sector{
		ID: num,
	}

	for _, line := range p.lines {
		parts = portType.FindStringSubmatch(line)
		if len(parts) == 2 {
			s.Port = &models.Port{
				Type: parts[1],
			}
			continue
		}

		parts = figInfo.FindStringSubmatch(line)
		if len(parts) == 4 {
			// remove commas from number
			count, err := strconv.Atoi(removeCommas(parts[1]))
			if err != nil {
				return
			}
			s.Figs = count
			s.FigsType = parts[3]
			switch parts[2] {
			case "yours", "belong to your Corp":
				s.FigsFriendly = true
			}
		}

		parts = minesInfo.FindStringSubmatch(line)
		if len(parts) == 3 {
			count, err := strconv.Atoi(parts[1])
			if err != nil {
				return
			}
			s.Mines = count
			switch parts[2] {
			case "yours", "belong to your Corp":
				s.MinesFriendly = true
			}
		}

		parts = warpsInfo.FindStringSubmatch(line)
		if len(parts) == 2 {
			sectors := strings.Split(parts[1], " - ")
			for _, sector := range sectors {
				warp, err := strconv.Atoi(strings.Trim(sector, "() "))
				if err != nil {
					fmt.Printf("failed to parse warp %s: %s", sector, err.Error())
					return
				}
				s.Warps = append(s.Warps, warp)
			}
			s.WarpCount = len(s.Warps)
		}
	}

	// copy previously-known warps. This helps since holo-scans don't include
	// warp info.
	existing, ok := p.data.Sectors[num]
	if ok {
		if len(s.Warps) == 0 {
			s.Warps = existing.Warps
			s.WarpCount = existing.WarpCount
		}
	}

	p.data.Sectors[num] = &s
	p.broker.Publish(&events.Event{
		Kind: events.SECTORDISPLAY,
		ID:   fmt.Sprint(s.ID),
	})
}

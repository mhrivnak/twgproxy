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
var portType *regexp.Regexp = regexp.MustCompile(`^Ports   : [a-zA-Z0-9 '-]+, Class \d \(([SB]{3})\)`)
var figInfo *regexp.Regexp = regexp.MustCompile(`^Fighters: ([0-9,]+) \((.+?)\) \[([A-Za-z]+)\]`)
var minesInfo *regexp.Regexp = regexp.MustCompile(`^Mines   : ([0-9]+) \(Type 1 Armid\) \(([A-Za-z ]+)\)`)
var warpsInfo *regexp.Regexp = regexp.MustCompile(`^Warps to Sector\(s\) :  ([()0-9 -]+)`)
var traderInfo0 *regexp.Regexp = regexp.MustCompile(`([a-zA-Z0-9 ]+), w/ ([0-9,]+) ftrs`)
var traderInfo1 *regexp.Regexp = regexp.MustCompile(`\(([a-zA-Z ]+)\)`)

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
		ID:         num,
		IsFedSpace: strings.Contains(p.lines[0], "The Federation"),
	}

	var parsingTraderType models.TraderType
	var parsingTraders bool
	var parsingTradersIndex int
	var halfParsedTrader *models.Trader

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
			s.FigType, err = models.FigTypeFromString(parts[3])
			if err != nil {
				fmt.Println(err.Error())
				return
			}

			switch parts[2] {
			case "yours", "belong to your Corp":
				s.FigsFriendly = true
			}
			continue
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
			continue
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
			continue
		}

		if strings.HasPrefix(line, "Alien Tr:") {
			parsingTraderType = models.TraderTypeAlien
			parsingTraders = true
			parsingTradersIndex = 0
		} else if strings.HasPrefix(line, "Traders :") {
			parsingTraderType = models.TraderTypeNormal
			parsingTraders = true
			parsingTradersIndex = 0
		} else if strings.HasPrefix(line, "Grey    :") {
			parsingTraderType = models.TraderTypeGrey
			parsingTraders = true
			parsingTradersIndex = 0
		} else if strings.HasPrefix(line, "Ferrengi:") {
			parsingTraderType = models.TraderTypeFerrengi
			parsingTraders = true
			parsingTradersIndex = 0
		} else if len(line) == 0 || !strings.HasPrefix(line, "          ") {
			parsingTraders = false
		}

		if parsingTraders {
			if parsingTradersIndex%2 == 0 {
				parts := traderInfo0.FindStringSubmatch(line)
				if len(parts) != 3 {
					fmt.Printf("failed to parse trader info: %s\n", line)
					parsingTraders = false
					continue
				}
				figs, err := strconv.Atoi(removeCommas(parts[2]))
				if err != nil {
					fmt.Printf("failed to parse trader figs: %s\n", line)
					parsingTraders = false
					continue
				}
				halfParsedTrader = &models.Trader{
					Name: models.StripTitleFromName(strings.TrimSpace(parts[1])),
					Figs: figs,
					Type: parsingTraderType,
				}
			} else {
				parts := traderInfo1.FindStringSubmatch(line)
				if len(parts) != 2 {
					fmt.Printf("failed to parse trader info: %s\n", line)
					parsingTraders = false
					continue
				}
				halfParsedTrader.ShipType = models.ShipTypeFromString(parts[1])
				s.Traders = append(s.Traders, *halfParsedTrader)
				halfParsedTrader = nil
			}
			parsingTradersIndex += 1
		}
	}

	p.data.SectorLock.Lock()
	// copy previously-known warps. This helps since holo-scans don't include
	// warp info.
	existing, ok := p.data.Sectors[num]
	if ok {
		if len(s.Warps) == 0 {
			s.Warps = existing.Warps
			s.WarpCount = existing.WarpCount
		}
		s.Density = existing.Density
	}

	p.data.Sectors[num] = &s
	p.data.SectorLock.Unlock()

	p.broker.Publish(&events.Event{
		Kind:    events.SECTORDISPLAY,
		ID:      fmt.Sprint(s.ID),
		DataInt: s.ID,
	})
}

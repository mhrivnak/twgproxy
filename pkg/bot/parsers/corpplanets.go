package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func NewCorpPlanetsParser(data *models.Data, broker *events.Broker) Parser {
	return &ParseCorpPlanetsDisplay{
		data:   data,
		broker: broker,
	}
}

type ParseCorpPlanetsDisplay struct {
	lines  []string
	data   *models.Data
	broker *events.Broker
}

var corpPlanetListLine0 *regexp.Regexp = regexp.MustCompile(`(\d+)[ T]+#(\d+).*?Class ([A-Z]), `)
var corpPlanetListLine1 *regexp.Regexp = regexp.MustCompile(`\([0-9M-]+\) +[0-9T]+ +[0-9T]+ +[0-9T]+ +([0-9MT]+) +([0-9MT]+) +([0-9MT]+) +([0-9MT]+)`)

func (p *ParseCorpPlanetsDisplay) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *ParseCorpPlanetsDisplay) Done() bool {
	length := len(p.lines)
	if length < 5 {
		return false
	}
	last := p.lines[length-1]
	switch {
	case strings.HasPrefix(last, "======  "):
		p.finalize()
		return true
	default:
		return false
	}
}

func (p *ParseCorpPlanetsDisplay) finalize() {
	// skip the first 5 lines
	lines := p.lines[5 : len(p.lines)-1]
	for i, line := range lines {
		// parse lines in pairs
		if i%2 == 1 {
			continue
		}

		parts0 := corpPlanetListLine0.FindStringSubmatch(line)
		if len(parts0) != 4 {
			fmt.Println("error parsing line 0 of planet entry")
			fmt.Println(line)
			return
		}
		parts1 := corpPlanetListLine1.FindStringSubmatch(lines[i+1])
		if len(parts1) != 5 {
			fmt.Println("error parsing line 1 of planet entry")
			return
		}
		pid, err := strconv.Atoi(parts0[2])
		if err != nil {
			fmt.Println("error parsing planet ID")
			return
		}
		sector, err := strconv.Atoi(parts0[1])
		if err != nil {
			fmt.Println("error parsing planet ID")
			return
		}
		planet, ok := p.data.Planets[pid]
		if !ok {
			planet = models.Planet{
				ID:    pid,
				Class: parts0[3],
			}
		}
		planet.Sector = sector

		summary := models.PlanetCorpSummary{}

		summary.Ore, err = summaryToInt(parts1[1])
		if err != nil {
			fmt.Println("error parsing planet ore")
			return
		}

		summary.Org, err = summaryToInt(parts1[2])
		if err != nil {
			fmt.Println("error parsing planet org")
			return
		}

		summary.Equ, err = summaryToInt(parts1[3])
		if err != nil {
			fmt.Println("error parsing planet org")
			return
		}

		summary.Figs, err = summaryToInt(parts1[4])
		if err != nil {
			fmt.Println("error parsing planet figs")
			return
		}

		planet.Summary = &summary
		p.data.Planets[pid] = planet
	}

	p.broker.Publish(&events.Event{
		Kind: events.CORPPLANETLISTDISPLAY,
	})
}

func summaryToInt(n string) (int, error) {
	if n == "---" {
		return 0, nil
	}

	length := len(n)
	factor := 1

	switch string(n[length-1]) {
	case "T":
		factor = 1000
		n = n[:length-1]
	case "M":
		factor = 1000000
		n = n[:length-1]
	}
	ret, err := strconv.Atoi(n)
	if err != nil {
		return 0, err
	}
	return ret * factor, nil
}

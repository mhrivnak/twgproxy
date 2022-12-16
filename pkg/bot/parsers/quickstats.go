package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func NewQuickStatsParser(data *models.Data, broker *events.Broker) Parser {
	return &ParseQuickStats{
		data:   data,
		broker: broker,
	}
}

type ParseQuickStats struct {
	lines  []string
	data   *models.Data
	broker *events.Broker
}

var quickStatsLn0 *regexp.Regexp = regexp.MustCompile(`^ Sect ([0-9]+).Turns [0-9]+.Creds ([0-9,]+).Figs ([0-9,]+).Shlds ([0-9,]+).Hlds ([0-9]+).`)
var quickStatsLn2 *regexp.Regexp = regexp.MustCompile(`Exp ([0-9,]+)`)

func (p *ParseQuickStats) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *ParseQuickStats) Done() bool {
	length := len(p.lines)
	if length < 3 {
		return false
	}
	last := p.lines[length-1]
	switch {
	case strings.TrimSpace(last) == "":
		p.finalize()
		return true
	default:
		return false
	}
}

func (p *ParseQuickStats) finalize() {
	for i, line := range p.lines {
		switch i {
		case 0:
			parts := quickStatsLn0.FindStringSubmatch(line)
			if len(parts) != 6 {
				fmt.Println("failed to parse quick stats line 0")
			}

			sector, err := strconv.Atoi(removeCommas(parts[1]))
			if err != nil {
				fmt.Println("error parsing sector")
			}
			p.data.Status.Sector = sector

			creds, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println("error parsing creds")
			}
			p.data.Status.Creds = creds

			figs, err := strconv.Atoi(removeCommas(parts[3]))
			if err != nil {
				fmt.Println("error parsing figs")
			}
			p.data.Status.Figs = figs

			shields, err := strconv.Atoi(removeCommas(parts[4]))
			if err != nil {
				fmt.Println("error parsing shields")
			}
			p.data.Status.Shields = shields

			holds, err := strconv.Atoi(removeCommas(parts[5]))
			if err != nil {
				fmt.Println("error parsing holds")
			}
			p.data.Status.Holds = holds
		case 2:
			parts := quickStatsLn2.FindStringSubmatch(line)
			if len(parts) != 2 {
				fmt.Println("failed to parse quick stats line 0")
			}

			exp, err := strconv.Atoi(removeCommas(parts[1]))
			if err != nil {
				fmt.Println("error parsing exp")
			}
			p.data.Status.Exp = exp
		}
		fmt.Println(line)
	}
}

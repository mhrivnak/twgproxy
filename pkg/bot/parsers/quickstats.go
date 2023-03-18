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

var quickStatsItem *regexp.Regexp = regexp.MustCompile(`^([a-zA-Z]+) ([a-zA-Z0-9,-]+)`)

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
	items := []string{}
	for _, line := range p.lines {
		items = append(items, splitQStats(strings.TrimSpace(line))...)
	}

	for _, item := range items {
		parts := quickStatsItem.FindStringSubmatch(item)
		if len(parts) != 3 {
			fmt.Printf("failed to parse quick stat item: %s\n", item)
			return
		}
		switch parts[1] {
		case "Sect":
			sector, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println("error parsing sector")
			}
			p.data.Status.Sector = sector
		case "Creds":
			creds, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println("error parsing creds")
			}
			p.data.Status.Creds = creds
		case "Figs":
			figs, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println("error parsing figs")
			}
			p.data.Status.Figs = figs
		case "Shlds":
			shields, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println("error parsing shields")
			}
			p.data.Status.Shields = shields
		case "Hlds":
			holds, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println("error parsing holds")
			}
			p.data.Status.Holds = holds
		case "Exp":
			exp, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println("error parsing exp")
			}
			p.data.Status.Exp = exp
		case "GTorp":
			gtorps, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println("error parsing gtorps")
			}
			p.data.Status.GTorps = gtorps
		case "AtmDt":
			atmdt, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println("error parsing atmdts")
			}
			p.data.Status.AtmDts = atmdt
		case "LRS":
			switch parts[2] {
			case "None":
				p.data.Status.LRS = models.LRSNONE
			case "Holo":
				p.data.Status.LRS = models.LRSHOLO
			}
		case "TWarp":
			switch parts[2] {
			case "No":
				p.data.Status.TWarp = models.TWarpTypeNone
			case "1":
				p.data.Status.TWarp = models.TWarpType1
			case "2":
				p.data.Status.TWarp = models.TWarpType2
			}
		}
	}
	p.broker.Publish(&events.Event{
		Kind: events.QUICKSTATDISPLAY,
	})
}

// splitQStats splits the lines of quick stats into strings that each have the
// key and value pair. strings.Split() didn't work with the extended ascii
// separator character, which I think is ASCII int code 172. I don't know why
// it didn't work.
func splitQStats(line string) []string {
	var ret []string
	chars := []byte{}
	for _, c := range []byte(line) {
		if int(c) <= 128 {
			chars = append(chars, c)
		} else if len(chars) > 0 {
			ret = append(ret, string(chars))
			chars = []byte{}
		}
	}
	if len(chars) > 0 {
		ret = append(ret, string(chars))
	}
	return ret
}

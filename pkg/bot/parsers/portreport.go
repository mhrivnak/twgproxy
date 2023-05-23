package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func NewPortReportParser(data *models.Data, broker *events.Broker) Parser {
	return &parsePortReport{
		data:   data,
		broker: broker,
	}
}

type parsePortReport struct {
	lines  []string
	broker *events.Broker
	data   *models.Data
}

var fInfo *regexp.Regexp = regexp.MustCompile(`Fuel Ore   (Buying|Selling) +(\d+) +(\d+)%`)
var oInfo *regexp.Regexp = regexp.MustCompile(`Organics   (Buying|Selling) +(\d+) +(\d+)%`)
var eInfo *regexp.Regexp = regexp.MustCompile(`Equipment  (Buying|Selling) +(\d+) +(\d+)%`)
var portReportSectorUserInput *regexp.Regexp = regexp.MustCompile(`What sector is the port in?.+\[\d+\] (\d+)`)
var portReportSectorDefault *regexp.Regexp = regexp.MustCompile(`What sector is the port in?.+\[(\d+)\]`)

func (p *parsePortReport) Parse(line string) error {
	p.lines = append(p.lines, strings.TrimSpace(line))
	return nil
}

func (p *parsePortReport) Done() bool {
	length := len(p.lines)

	last := p.lines[length-1]
	if strings.HasPrefix(last, "I have no information about a port") {
		return true
	}

	if strings.HasPrefix(last, "Equipment") {
		p.finalize()
		return true
	}

	return false
}

func (p *parsePortReport) finalize() {
	var sectorID int
	var err error

	if strings.HasPrefix(p.lines[0], "What sector is the port in?") {
		parts := portReportSectorUserInput.FindStringSubmatch(p.lines[0])
		if len(parts) == 2 {
			sectorID, err = strconv.Atoi(parts[1])
			if err != nil {
				fmt.Printf("failed parsing sector from %s\n", parts[1])
				return
			}
		} else {
			parts = portReportSectorDefault.FindStringSubmatch(p.lines[0])
			if len(parts) == 2 {
				sectorID, err = strconv.Atoi(parts[1])
				if err != nil {
					fmt.Printf("failed parsing sector from %s\n", parts[1])
					return
				}
			}
		}
	} else {
		sectorID = p.data.Status.Sector
	}

	fuel, err := parsePortReportLine(p.lines[len(p.lines)-3], fInfo)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	org, err := parsePortReportLine(p.lines[len(p.lines)-2], oInfo)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	equ, err := parsePortReportLine(p.lines[len(p.lines)-1], eInfo)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	report := models.PortReport{
		Time: time.Now(),
		Fuel: fuel,
		Org:  org,
		Equ:  equ,
	}

	sector, ok := p.data.GetSector(sectorID)
	if ok && sector.Port != nil {
		sector.Port.Report = &report
		fmt.Printf("added port report for sector %d\n", sectorID)
	}

	p.data.PortReportLock.Lock()
	p.data.PortReports[sectorID] = &report
	p.data.PortReportLock.Unlock()

	p.broker.Publish(&events.Event{
		Kind:    events.PORTREPORTDISPLAY,
		ID:      fmt.Sprint(sectorID),
		DataInt: sectorID,
	})
}

func parsePortReportLine(line string, r *regexp.Regexp) (models.PortItem, error) {
	item := models.PortItem{}

	parts := r.FindStringSubmatch(line)
	if len(parts) != 4 {
		fmt.Println(line)
		return item, fmt.Errorf("matched %d parts instead of 4", len(parts))
	}

	switch parts[1] {
	case "Buying":
		item.Status = models.BUYING
	case "Selling":
		item.Status = models.SELLING
	default:
		return item, fmt.Errorf("unknown port item status %s", parts[1])
	}

	var err error
	item.Trading, err = strconv.Atoi(parts[2])
	if err != nil {
		return item, err
	}

	item.Percent, err = strconv.Atoi(parts[3])
	if err != nil {
		return item, err
	}

	return item, nil
}

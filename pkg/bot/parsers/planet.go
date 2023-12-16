package parsers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func NewPlanetParser(data *models.Data, broker *events.Broker) Parser {
	p := parsePlanet{
		data:   data,
		broker: broker,
	}

	go func() {
		<-broker.WaitFor(context.TODO(), events.PROMPTDISPLAY, events.PLANETPROMPT)
		p.done = true
		p.finalize()
	}()

	return &p
}

type parsePlanet struct {
	lines  []string
	data   *models.Data
	broker *events.Broker
	done   bool
}

var planetInfo *regexp.Regexp = regexp.MustCompile(`^Planet #([0-9]+) in sector ([0-9]+): (.+)`)
var classInfo *regexp.Regexp = regexp.MustCompile(`^Class ([A-Z])`)
var fuelInfo *regexp.Regexp = regexp.MustCompile(`^Fuel Ore +?([0-9,]+) +?[0-9,]+ +?[0-9,]+ +?([0-9,]+) +?[0-9]+ +?([0-9,]+)`)
var orgInfo *regexp.Regexp = regexp.MustCompile(`^Organics +?([0-9,]+) +?[0-9,N/A]+ +?[0-9,]+ +?([0-9,]+) +?[0-9]+ +?([0-9,]+)`)
var equInfo *regexp.Regexp = regexp.MustCompile(`^Equipment +?([0-9,]+) +?[0-9,]+ +?[0-9,]+ +?([0-9,]+) +?[0-9]+ +?([0-9,]+)`)
var citatelInfo *regexp.Regexp = regexp.MustCompile(`^Planet has a level ([0-6]) Citadel`)
var figsInfo *regexp.Regexp = regexp.MustCompile(`^Fighters +?N/A +?[0-9,]+ +?[0-9,]+ +?([0-9,]+) `)

func (p *parsePlanet) Parse(line string) error {
	p.lines = append(p.lines, line)
	return nil
}

func (p *parsePlanet) Done() bool {
	return p.done
}

func (p *parsePlanet) finalize() {
	planet := models.Planet{}
	for _, line := range p.lines {
		switch {
		case strings.HasPrefix(line, "Planet #"):
			parts := planetInfo.FindStringSubmatch(line)
			if len(parts) != 4 {
				continue
			}
			id, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			sector, err := strconv.Atoi(parts[2])
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet = models.Planet{
				ID:     id,
				Sector: sector,
				Name:   strings.TrimSpace(parts[3]),
			}
		case strings.HasPrefix(line, "Class "):
			parts := classInfo.FindStringSubmatch(line)
			if len(parts) != 2 {
				continue
			}
			planet.Class = parts[1]
		case strings.HasPrefix(line, "Fuel Ore"):
			parts := fuelInfo.FindStringSubmatch(line)
			if len(parts) != 4 {
				continue
			}
			fuel, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.Ore = fuel
			fuelMax, err := strconv.Atoi(removeCommas(parts[3]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.OreMax = fuelMax
			fmt.Printf("max fuel %d\n", fuelMax)
			cols, err := strconv.Atoi(removeCommas(parts[1]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.FuelCols = cols
		case strings.HasPrefix(line, "Organics"):
			parts := orgInfo.FindStringSubmatch(line)
			if len(parts) != 4 {
				continue
			}
			org, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.Org = org
			orgMax, err := strconv.Atoi(removeCommas(parts[3]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.OrgMax = orgMax
			fmt.Printf("max org %d\n", orgMax)
			cols, err := strconv.Atoi(removeCommas(parts[1]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.OrgCols = cols
		case strings.HasPrefix(line, "Equipment"):
			parts := equInfo.FindStringSubmatch(line)
			if len(parts) != 4 {
				continue
			}
			equ, err := strconv.Atoi(removeCommas(parts[2]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.Equ = equ
			equMax, err := strconv.Atoi(removeCommas(parts[3]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.EquMax = equMax
			fmt.Printf("max equ %d\n", equMax)
			cols, err := strconv.Atoi(removeCommas(parts[1]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.EquCols = cols
		case strings.HasPrefix(line, "Planet has a level "):
			parts := citatelInfo.FindStringSubmatch(line)
			if len(parts) != 2 {
				continue
			}
			level, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.Level = level
		case strings.HasPrefix(line, "Fighters "):
			parts := figsInfo.FindStringSubmatch(line)
			if len(parts) != 2 {
				continue
			}
			figs, err := strconv.Atoi(removeCommas(parts[1]))
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			planet.Figs = figs
		}
	}

	p.data.PlanetLock.Lock()
	existing, ok := p.data.Planets[planet.ID]
	if ok {
		planet.Summary = existing.Summary
	}

	p.data.Planets[planet.ID] = &planet
	p.data.PlanetLock.Unlock()

	p.broker.Publish(&events.Event{
		Kind:    events.PLANETDISPLAY,
		ID:      fmt.Sprint(planet.ID),
		DataInt: planet.ID,
	})
}

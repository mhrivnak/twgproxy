package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actions/tools"
	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

type pRouteTrade struct {
	actuator *actuator.Actuator
	points   []int
	done     chan struct{}
}

func NewPRouteTrade(points string, actuator *actuator.Actuator) (Action, error) {
	done := make(chan struct{})

	parsedPoints, err := tools.ParsePoints(points)

	return &pRouteTrade{
		actuator: actuator,
		done:     done,
		points:   parsedPoints,
	}, err
}

func (p *pRouteTrade) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *pRouteTrade) run(ctx context.Context) {
	defer close(p.done)

	// list corp planets
	p.actuator.Send("tlq")

	select {
	case <-ctx.Done():
		return
	case <-p.actuator.Broker.WaitFor(ctx, events.CORPPLANETLISTDISPLAY, ""):
	}

	completed := map[int]struct{}{}

	planetsBySector := map[int][]*models.Planet{}
	for pid, planet := range p.actuator.Data.Planets {
		planetsBySector[planet.Sector] = append(planetsBySector[planet.Sector], p.actuator.Data.Planets[pid])
	}

	for _, point := range p.points {
		route, err := p.actuator.RouteTo(ctx, point)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		for i, sectorID := range route {
			if i > 0 {
				// move to the next sector
				err = p.actuator.MoveSafe(ctx, sectorID, false)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
			}

			// skip if we already processed this sector
			_, ok := completed[sectorID]
			if ok {
				fmt.Printf("skipping sector %d that we already visited\n", sectorID)
				continue
			}
			// mark completed
			completed[sectorID] = struct{}{}

			sector, ok := p.actuator.Data.GetSector(sectorID)
			if !ok {
				fmt.Println("current sector not in cache; that shouldn't happen")
				return
			}
			if sector.Port == nil {
				fmt.Println("skipping sector without port")
				continue
			}

			// get port report
			commandPromptWait := p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.COMMANDPROMPT)
			p.actuator.Send("cr\rq")

			select {
			case <-ctx.Done():
				return
			case <-p.actuator.Broker.WaitFor(ctx, events.PORTREPORTDISPLAY, ""):
			}

			// make sure we're back at the command prompt before proceeding.
			// Command sent, such as landing, before getting back to the command
			// prompt can get swallowed.
			select {
			case <-commandPromptWait:
			case <-ctx.Done():
				return
			}

			// re-fetch the sector to get the updated port report
			sector, ok = p.actuator.Data.GetSector(sectorID)
			if !ok {
				fmt.Println("current sector not in cache; that shouldn't happen")
				return
			}

			if sector.Port == nil {
				fmt.Println("skipping sector without a port")
				continue
			}

			if sector.Port.Report == nil {
				fmt.Println("failed to get port report")
				return
			}

			report := sector.Port.Report

			if report.Org.Status == models.SELLING {
				fmt.Println("skipping port that doesn't buy org")
				continue
			}

			if report.Org.Percent != 100 {
				fmt.Printf("skipping port at %d percent\n", report.Org.Percent)
				continue
			}

			//pick a planet
			planet := choosePlanet(sector, planetsBySector[sectorID])
			if planet == nil {
				fmt.Println("no suitable planet")
				continue
			}

			fmt.Printf("chose planet %d in sector %d\n", planet.ID, sectorID)

			// run the ptrade action
			wait := p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.PLANETPROMPT)
			p.actuator.Land(planet.ID)
			select {
			case <-wait:
			case <-ctx.Done():
				return
			}

			p.actuator.MombotPlanetSell(ctx, models.ORG)

			// lift off from planet
			p.actuator.Send("q")
		}

	}
}

func choosePlanet(sector *models.Sector, planets []*models.Planet) *models.Planet {
	var choice *models.Planet

	for i, planet := range planets {
		// skip if there isn't as much org as the port is buying
		if planet.Summary.Org < sector.Port.Report.Org.Trading {
			continue
		}
		if choice != nil {
			// pick the planet with the most org
			if choice.Summary.Org >= planet.Summary.Org {
				continue
			}
		}
		choice = planets[i]
	}

	return choice
}

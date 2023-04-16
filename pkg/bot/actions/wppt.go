package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
	"github.com/mhrivnak/twgproxy/pkg/models/persist"
)

type wppt struct {
	actuator *actuator.Actuator
	done     chan struct{}
}

func NewWPPT(a *actuator.Actuator) Action {
	done := make(chan struct{})
	return &wppt{
		actuator: a,
		done:     done,
	}
}

func (p *wppt) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *wppt) canConsiderPort(sector *models.Sector, saved *persist.Sector) bool {
	report := sector.Port.Report
	if sector == nil || sector.Port == nil || sector.Port.Report == nil {
		return false
	}

	switch {
	case report.Equ.Percent < 80:
		return false
	case report.Org.Percent < 80:
		return false
	case report.Equ.Trading > 10000:
		return false
	case report.Org.Trading > 10000:
		return false
	case saved.Busted != nil:
		return false
	}

	return true
}

func (p *wppt) sendBuy0(ctx context.Context, sector *models.Sector) {
	if sector.Port == nil {
		fmt.Println("unexpected nil port info")
		// fall back to sending three 0s. Extras won't hurt.
		p.actuator.Send("0\r0\r0\r")
	}
	rep := sector.Port.Report
	if rep.Fuel.Status == models.SELLING {
		p.actuator.Send("0\r")
	}
	if rep.Org.Status == models.SELLING {
		p.actuator.Send("0\r")
	}
	if rep.Equ.Status == models.SELLING {
		p.actuator.Send("0\r")
	}

}

func (p *wppt) trade(ctx context.Context, plan *tradePlan, current, next *models.Sector) {
	for i := 0; i < plan.NumTrades; i++ {
		if i > 0 {
			p.actuator.Move(ctx, next.ID, actuator.MoveOptions{DropFigs: 1}, false)
			current, next = next, current
		}
		if ctx.Err() != nil {
			return
		}

		// if there's nothing to buy on the first round, don't bother porting.
		if i == 0 && plan.BuyFuelFrom != current.ID && plan.BuyOrgFrom != current.ID && plan.BuyEquFrom != current.ID {
			continue
		}

		lastTime := i+1 == plan.NumTrades
		p.port(ctx, plan, current, lastTime)
	}
}

func (p *wppt) port(ctx context.Context, plan *tradePlan, current *models.Sector, lastTime bool) {
	p.actuator.Send("pt")

	// sell if product onboard, else buy
	select {
	case <-ctx.Done():
		return
	case <-p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.SELLPROMPT):
		// sell all
		p.actuator.Send("\r")
	case e := <-p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.BUYPROMPT):
		fmt.Println("got buy prompt right away")
		if lastTime {
			fmt.Println("done trading this pair")
			p.sendBuy0(ctx, current)
			return
		}
		if p.buy(ctx, plan, current, e) {
			p.port(ctx, plan, current, lastTime)
		}
		return
	}

	// buy, since the above step was a sell
	select {
	case <-ctx.Done():
		return
	case <-p.actuator.Broker.WaitFor(ctx, events.PORTNOTINTERESTED, ""):
		// TODO make sure we're not asked next to buy something else. For
		// example, got a "not interested" response negotiating to buy fuel, but
		// then get a prompt to buy org.
		// try again
		p.port(ctx, plan, current, lastTime)
		return
	case <-p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.COMMANDPROMPT):
		return
	case e := <-p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.BUYPROMPT):
		if lastTime {
			fmt.Println("done trading this pair")
			p.sendBuy0(ctx, current)
			return
		}
		if p.buy(ctx, plan, current, e) {
			p.port(ctx, plan, current, lastTime)
		}
	}
}

// buy responds to the first buy prompt a port presents after docking. return
// value indicates if the port and trade operation should be tried again,
// usually because in negotiation, the port says they were not interested.
func (p *wppt) buy(ctx context.Context, plan *tradePlan, sector *models.Sector, e *events.Event) bool {
	output := ""
	rep := sector.Port.Report

	switch {
	case plan.BuyFuelFrom == sector.ID:
		output = "\r"
	case plan.BuyOrgFrom == sector.ID:
		if rep.Fuel.Status == models.SELLING {
			output = "0\r"
		}
		output += "\r"
	case plan.BuyEquFrom == sector.ID:
		if rep.Fuel.Status == models.SELLING {
			output = "0\r"
		}
		if rep.Org.Status == models.SELLING {
			output += "0\r"
		}
		output += "\r"
	default:
		// not buying anything
		if rep.Fuel.Status == models.SELLING {
			output = "0\r"
		}
		if rep.Org.Status == models.SELLING {
			output += "0\r"
		}
		if rep.Equ.Status == models.SELLING {
			output += "0\r"
		}

	}
	p.actuator.Send(output)

	select {
	case <-ctx.Done():
		return false
	case <-p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.COMMANDPROMPT):
		return false
	case <-p.actuator.Broker.WaitFor(ctx, events.PORTNOTINTERESTED, ""):
		// try again
		return true
	}
}

func (p *wppt) run(ctx context.Context) {
	defer close(p.done)
	visited := map[int]struct{}{}

	// clear the holds
	p.actuator.Send("jy")

	currentSectorID := p.actuator.Data.Status.Sector

	for {
		if ctx.Err() != nil {
			return
		}

		// holo-scan
		p.actuator.Send("sh")

		// wait for the sectors to be parsed
		select {
		case <-ctx.Done():
			return
		case <-p.actuator.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(currentSectorID)):
		}

		visited[currentSectorID] = struct{}{}

		current, ok := p.actuator.Data.GetSector(currentSectorID)
		if !ok {
			fmt.Println("can't get current sector")
			return
		}

		if current.Port != nil {
			fmt.Println("considering port")
			p.actuator.Send("cr\rq")

			select {
			case <-ctx.Done():
				return
			case <-p.actuator.Broker.WaitFor(ctx, events.PORTREPORTDISPLAY, ""):
			}

			// re-fetch current sector
			current, ok := p.actuator.Data.GetSector(currentSectorID)
			if !ok {
				fmt.Println("can't get current sector")
				return
			}
			if current.Port == nil {
				fmt.Println("current sector's port disappeared???")
				return
			}
			savedCurrent, ok := p.actuator.Data.Persist.SectorCache.Get(currentSectorID)
			if !ok {
				fmt.Println("can't get current sector from DB cache")
				return
			}

			plans := map[int]*tradePlan{}

			if p.canConsiderPort(current, savedCurrent) {
				fmt.Printf("considering %d neighbors\n", len(current.Warps))
				// check neighbors
				for _, neighborID := range current.Warps {
					fmt.Printf("considering neighbor %d\n", neighborID)
					neighbor, ok := p.actuator.Data.GetSector(neighborID)
					if !ok {
						fmt.Println("can't get neighbor from cache")
						return
					}
					if neighbor.Port == nil {
						continue
					}

					if !neighbor.IsSafe() {
						continue
					}

					savedNeighbor, ok := p.actuator.Data.Persist.SectorCache.Get(neighborID)
					if !ok {
						fmt.Printf("error getting neighbor %d\r", neighborID)
						return
					}

					// make sure there's a direct warp back to the current sector
					_, ok = p.actuator.Data.Persist.WarpCache.Get(neighborID)
					if !ok {
						p.actuator.QueryWarps(ctx, neighborID, true)
					}
					_, ok = p.actuator.Data.Persist.WarpCache.Get(neighborID)
					if !ok {
						fmt.Printf("failed to query warps for sector %d\n", neighborID)
						return
					}
					if !p.actuator.Data.Persist.WarpCache.Exists(neighborID, currentSectorID) {
						fmt.Println("return warp does not exist")
						continue
					}

					neighbor, ok = p.actuator.Data.GetSector(neighborID)
					if !ok {
						fmt.Printf("neighbor port %d disappeared???\n", neighborID)
						return
					}

					// get fresh report
					p.actuator.Sendf("cr%d\rq", neighborID)

					select {
					case <-ctx.Done():
						return
					case <-p.actuator.Broker.WaitFor(ctx, events.PORTREPORTDISPLAY, fmt.Sprint(neighborID)):
					}

					if p.canConsiderPort(neighbor, savedNeighbor) {
						tp := p.createTradePlan(current, neighbor)
						if tp != nil {
							fmt.Printf("created trade plan for neighbor %d\n", neighborID)
							plans[neighborID] = tp
						} else {
							fmt.Println("no trade plan")
						}
					}
				}
			}

			if len(plans) >= 1 {
				fmt.Println("found one or more pairs")
				for k, v := range plans {
					fmt.Printf("sector: %d, score: %d\n", k, v.Score)
				}

				// pick the best plan
				bestPlanKey := 0
				for k, plan := range plans {
					if bestPlanKey == 0 {
						bestPlanKey = k
						continue
					}
					if plan.Score > plans[bestPlanKey].Score {
						bestPlanKey = k
					}
				}

				fmt.Printf("best plan: %d with score %d\n", bestPlanKey, plans[bestPlanKey].Score)
				out, _ := json.Marshal(plans[bestPlanKey])
				fmt.Println(string(out))

				neighbor, _ := p.actuator.Data.GetSector(bestPlanKey)
				p.trade(ctx, plans[bestPlanKey], current, neighbor)
				fmt.Println("DONE TRADING PAIR")
			}
		}

		current, ok = p.actuator.Data.GetSector(p.actuator.Data.Status.Sector)
		if !ok {
			fmt.Println("can't get current sector")
			return
		}

		safeHops := []int{}
		for _, w := range current.Warps {
			s, ok := p.actuator.Data.GetSector(w)
			if !ok {
				continue
			}
			if s.IsSafe() {
				safeHops = append(safeHops, w)
			}
		}

		if len(safeHops) == 0 {
			fmt.Println("No safe moves available. Stopping.")
			return
		}

		unexplored := p.actuator.Data.Persist.WarpCache.TrimExplored(safeHops)

		unvisited := []int{}
		for _, w := range safeHops {
			_, ok := visited[w]
			if !ok {
				unvisited = append(unvisited, w)
			}
		}

		fmt.Printf("of %d safe sectors: %d unexplored, %d unvisited\n", len(safeHops), len(unexplored), len(unvisited))

		var next int
		switch {
		case len(unexplored) > 0:
			// bias toward unexplored sectors
			next = unexplored[rand.Intn(len(unexplored))]
		case len(unvisited) > 0:
			// bias toward sectors not visited during this action
			next = unvisited[rand.Intn(len(unvisited))]
		default:
			next = safeHops[rand.Intn(len(safeHops))]
		}

		p.actuator.Move(ctx, next, actuator.MoveOptions{DropFigs: 1, MinFigs: 100}, false)
		currentSectorID = next
	}

	// if not at a port, check adjacent ports
	// port should not be full-upgraded, be >90% on equ and/or org, not be current bust, not be avoided
	// if looking good, check its neighbors for a candidate.

	// if pair not found, pick safe no-yet-visited-this-action sector at random and move there
	// if all sectors have been visited, pick one at random

}

func (p *wppt) createTradePlan(a, b *models.Sector) *tradePlan {
	tp := tradePlan{}

	repA := a.Port.Report
	repB := b.Port.Report

	// can we trade equ?
	if repA.Equ.Status != repB.Equ.Status {
		if repA.Equ.Status == models.SELLING {
			tp.BuyEquFrom = a.ID
			// can we also trade org or fuel the other direction?
			tp.checkOrgFuel(b, a)
		} else {
			tp.BuyEquFrom = b.ID
			// can we also trade org or fuel the other direction?
			tp.checkOrgFuel(a, b)
		}

	} else {
		// can we trade only org and fuel?
		tp.checkJustOrgFuel(a, b)
	}

	tp.addScore()
	if tp.Score < 800 {
		return nil
	}

	shipHolds := p.actuator.Data.Status.Holds

	maxTrades := 0

	if tp.BuyEquFrom != 0 {
		x := math.Min(float64(repA.Equ.Trading), float64(repB.Equ.Trading)) * .8
		maxTrades = int(x / float64(shipHolds))
	}
	if tp.BuyOrgFrom != 0 {
		x := math.Min(float64(repA.Org.Trading), float64(repB.Org.Trading)) * .8
		if maxTrades > 0 {
			maxTrades = min(maxTrades, int(x/float64(shipHolds)))
		} else {
			maxTrades = int(x / float64(shipHolds))
		}
	}
	if tp.BuyFuelFrom != 0 {
		x := math.Min(float64(repA.Fuel.Trading), float64(repB.Fuel.Trading)) * .8
		if maxTrades > 0 {
			maxTrades = min(maxTrades, int(x/float64(shipHolds)))
		} else {
			maxTrades = int(x / float64(shipHolds))
		}
	}
	tp.NumTrades = maxTrades * 2

	return &tp
}

func (t *tradePlan) checkOrgFuel(from, to *models.Sector) {
	if from.Port.Report.Org.Status != to.Port.Report.Org.Status && from.Port.Report.Org.Status == models.SELLING {
		t.BuyOrgFrom = from.ID
		return
	}

	if from.Port.Report.Fuel.Status != to.Port.Report.Fuel.Status && from.Port.Report.Fuel.Status == models.SELLING {
		t.BuyFuelFrom = from.ID
		return
	}
}

func (t *tradePlan) checkJustOrgFuel(a, b *models.Sector) {
	aRep := a.Port.Report
	bRep := b.Port.Report

	// can't trade org
	if aRep.Org.Status == bRep.Org.Status {
		return
	}

	// can't trade fuel
	if aRep.Fuel.Status == bRep.Fuel.Status {
		return
	}

	// can't trade both directions
	if aRep.Org.Status == aRep.Fuel.Status {
		return
	}

	if aRep.Org.Status == models.SELLING {
		t.BuyOrgFrom = a.ID
		t.BuyFuelFrom = b.ID
	} else {
		t.BuyOrgFrom = b.ID
		t.BuyFuelFrom = a.ID
	}
}

type tradePlan struct {
	BuyEquFrom  int
	BuyOrgFrom  int
	BuyFuelFrom int

	// how many times to port and trade
	NumTrades int

	Score int
}

func (t *tradePlan) otherSector(sectorID int) int {
	switch {
	case t.BuyFuelFrom != 0 && t.BuyFuelFrom != sectorID:
		return t.BuyFuelFrom
	case t.BuyOrgFrom != 0 && t.BuyOrgFrom != sectorID:
		return t.BuyOrgFrom
	case t.BuyEquFrom != 0 && t.BuyEquFrom != sectorID:
		return t.BuyEquFrom
	}
	return 0
}

func (t *tradePlan) addScore() {
	t.Score = 0

	if t.BuyEquFrom != 0 {
		t.Score += 1000
	}
	if t.BuyOrgFrom != 0 {
		t.Score += 500
	}
	if t.BuyFuelFrom != 0 {
		t.Score += 300
	}
}

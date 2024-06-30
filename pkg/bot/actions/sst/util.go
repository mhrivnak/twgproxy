package sst

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
	"github.com/mhrivnak/twgproxy/pkg/models/persist"
)

func PortCanBeUsed(ctx context.Context, a *actuator.Actuator, sector *persist.Sector) bool {
	// is this an xxB port?
	if sector.Equ == string(models.BUYING) {
		fmt.Printf("considering port %d\n", sector.ID)

		// don't use FedSpace
		if int(sector.ID) == a.Data.Status.StarDock {
			return false
		}
		if sector.ID <= 10 {
			return false
		}

		// any report within the last 2 minutes is recent enough
		report, err := a.GetPortReport(ctx, int(sector.ID), time.Minute*2)
		if err != nil {
			fmt.Println(err.Error())
			return false
		}

		// typically because enemy figs are in the sector
		if report == nil {
			return false
		}

		switch {
		case report.Equ.Status != models.BUYING:
			return false
		case report.Equ.Percent < 80:
			return false
		case report.Equ.Trading > 10000:
			return false
		case report.Org.Trading > 10000:
			return false
		case sector.Busted != nil:
			return false
		}
		return true
	}

	return false
}

func checkDistance(ctx context.Context, a *actuator.Actuator, distance, sectorA, sectorB, shipOther int) bool {
	outbound, err := a.RouteFromTo(ctx, sectorA, sectorB)
	if err != nil {
		return false
	}
	if len(outbound) >= distance {
		return false
	}

	inbound, err := a.RouteFromTo(ctx, sectorB, sectorA)
	if err != nil {
		return false
	}
	// the route starts with the current sector, so even a 1-hop route will have
	// two points
	if len(inbound) > distance+1 {
		return false
	}

	// make sure the current ship can go to sectorA and xport back to the other ship
	// TODO make this smarter. Maybe move to sectorB first if it's closer rather than
	// veto the pair.
	if sectorA != shipOther {
		aToOtherShip, err := a.RouteFromTo(ctx, sectorA, shipOther)
		if err != nil {
			return false
		}
		if len(aToOtherShip) > distance+1 {
			return false
		}
	}

	return true
}

type ShipMover func(context.Context, int) error

func FindPorts(ctx context.Context, a *actuator.Actuator, moveOpts actuator.MoveOptions,
	xportRange, shipCurrent, shipOtherSector int, moveShips ShipMover) (error, int, int) {

	visited := map[int]struct{}{}

	for {
		fmt.Println("####################### START FIND PORTS ITERATION ##########################")
		start := a.Data.Status.Sector
		visited[start] = struct{}{}

		// holo-scan
		a.Send("sh")

		candidates, unexplored := a.FindXXBPair(ctx, start, xportRange, []int{})

		for _, candidate := range candidates {
			sector, err := a.GetSectorWithVisit(ctx, candidate, moveOpts)
			if err != nil {
				return err, 0, 0
			}
			if PortCanBeUsed(ctx, a, sector) {
				fmt.Printf("found suitable portA: %d\n", sector.ID)
				// look for a companion
				companions, cUnexplored := a.FindXXBPair(ctx, candidate, xportRange, []int{int(sector.ID)})
				fmt.Printf("%d potential companions\n", len(companions))
				for _, companion := range companions {
					fmt.Printf("considering companion %d\n", companion)
					cSector, err := a.GetSectorWithVisit(ctx, companion, moveOpts)
					if err != nil {
						return err, 0, 0
					}
					if PortCanBeUsed(ctx, a, cSector) && checkDistance(ctx, a, xportRange, candidate, companion, shipOtherSector) {
						fmt.Printf("found a pair: %d, %d\n", candidate, companion)
						return nil, candidate, companion
					}
				}
				// explore
				for _, uc := range cUnexplored {
					fmt.Printf("moving to unexplored sector %d\n", uc)
					err = a.Move(ctx, uc, moveOpts, false)
					if err != nil {
						return err, 0, 0
					}
					// holo-scan since there's a good chance there are more
					// unexplored sectors nearby
					a.Send("sh")
					cSector, ok := a.Data.Persist.SectorCache.Get(uc)
					if !ok {
						fmt.Println("cound not get current sector from cache")
						continue
					}
					if PortCanBeUsed(ctx, a, cSector) && checkDistance(ctx, a, xportRange, candidate, uc, shipOtherSector) {
						return nil, candidate, uc
					}

				}
				// out of companions to check. Try another primary candidate.
				fmt.Printf("Giving up on primary candidate %d\n", sector.ID)
			}

		}

		// in case we went exploring to get sector info
		fmt.Println("moving back to the other ship to start towing it")
		err := a.Move(ctx, start, moveOpts, false)
		if err != nil {
			return err, 0, 0
		}

		current, _ := a.Data.GetSector(a.Data.Status.Sector)
		safeHops := []int{}
		for _, warp := range current.Warps {
			s, ok := a.Data.GetSector(warp)
			if !ok {
				fmt.Printf("cache miss getting sector %d for safe hops\n", warp)
				continue
			}
			if s.IsSafe() {
				safeHops = append(safeHops, warp)
			}
		}
		if len(safeHops) == 0 {
			fmt.Println("No safe moves available. Stopping.")
			return fmt.Errorf("no safe moves"), 0, 0
		}

		unexplored = a.Data.Persist.WarpCache.TrimExplored(safeHops)

		unvisited := []int{}
		for _, warp := range safeHops {
			_, ok := visited[warp]
			if !ok {
				unvisited = append(unvisited, warp)
			}
		}

		fmt.Printf("of %d safe sectors: %d unexplored, %d unvisited\n", len(safeHops), len(unexplored), len(unvisited))

		var next int
		switch {
		case len(unexplored) > 0:
			// bias toward unexplored sectors
			fmt.Println("picking a random unexplored sector")
			next = unexplored[rand.Intn(len(unexplored))]
		case len(unvisited) > 0:
			// bias toward sectors not visited during this action
			fmt.Println("picking a random unvisited sector")
			next = unvisited[rand.Intn(len(unvisited))]
		default:
			next = safeHops[rand.Intn(len(safeHops))]
			fmt.Println("picking a random sector")
		}

		err = moveShips(ctx, next)
		if err != nil {
			return err, 0, 0
		}
	}
}

func Refurb(ctx context.Context, a *actuator.Actuator) error {
	a.Sendf("pta")

	holdsChan := a.Broker.WaitFor(ctx, events.HOLDSTOBUY, "")
	figsChan := a.Broker.WaitFor(ctx, events.FIGSTOBUY, "")
	shieldsChan := a.Broker.WaitFor(ctx, events.SHIELDSTOBUY, "")

	// buy holds
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-holdsChan:
		a.Sendf("%d\ry", e.DataInt)
	}

	// buy figs
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-figsChan:
		if e.DataInt > 0 {
			a.Sendf("b%d\r", e.DataInt)
		}
	}

	// buy shields
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-shieldsChan:
		if e.DataInt > 0 {
			a.Sendf("c%d\r", e.DataInt)
		}
	}

	a.Send("q/")

	return nil
}

package actions

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

type wsst struct {
	actuator    *actuator.Actuator
	done        chan struct{}
	shipCurrent *ship
	shipOther   *ship
}

func NewWSST(a *actuator.Actuator, shipOther int) Action {
	done := make(chan struct{})
	return &wsst{
		actuator: a,
		done:     done,
		shipOther: &ship{
			ID: shipOther,
		},
	}
}

func (w *wsst) Start(ctx context.Context) <-chan struct{} {
	go w.run(ctx)
	return w.done
}

func (w *wsst) portCanBeUsed(ctx context.Context, sector *persist.Sector) bool {
	// is this an xxB port?
	if sector.Equ == string(models.BUYING) {
		fmt.Printf("considering port %d\n", sector.ID)

		// any report within the last 2 minutes is recent enough
		report, err := w.actuator.GetPortReport(ctx, int(sector.ID), time.Minute*2)
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

func (w *wsst) updateOtherShipSector(ctx context.Context) {
	w.actuator.Send("xq")
	// wait for the parser
	select {
	case <-ctx.Done():
		return
	case e := <-w.actuator.Broker.WaitFor(ctx, events.AVAILABLESHIPS, fmt.Sprint(w.shipOther.ID)):
		w.shipOther.sector = e.DataInt
	}
}

func (w *wsst) genMoveOptions() actuator.MoveOptions {
	return actuator.MoveOptions{
		DropFigs:     1,
		EnemyFigsMax: (w.actuator.Data.Status.Figs + w.actuator.Data.Status.Shields) / 3,
		MinFigs:      100,
	}
}

func (w *wsst) run(ctx context.Context) {
	defer close(w.done)

	w.actuator.QuickStats(ctx)

	// figure out where the ships are and get them to the same sector
	currentSectorID := w.actuator.Data.Status.Sector
	w.shipCurrent = &ship{
		ID:     w.actuator.Data.Status.Ship,
		sector: currentSectorID,
	}

	w.updateOtherShipSector(ctx)

	if w.shipCurrent.sector != w.shipOther.sector {
		err := w.actuator.Move(ctx, w.shipOther.sector, w.genMoveOptions(), false)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
	}

	for {
		if ctx.Err() != nil {
			return
		}

		fmt.Println("START SST ROUND")

		currentSectorID = w.actuator.Data.Status.Sector
		w.shipCurrent.sector = currentSectorID
		w.shipOther.sector = currentSectorID

		// holo-scan
		w.actuator.Send("sh")
		// wait for the sectors to be parsed
		select {
		case <-ctx.Done():
			return
		case <-w.actuator.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(currentSectorID)):
		}

		err := w.findPorts(ctx)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		busted, err := w.sst(ctx)

		if err != nil {
			fmt.Printf("error during SST: %s\n", err.Error())
			return
		}

		if busted {
			err = w.actuator.Move(ctx, 1, w.genMoveOptions(), false)
			if err != nil {
				fmt.Printf("stopping WSST: %s\n", err.Error())
				return
			}

			w.actuator.Sendf("pta")

			holdsChan := w.actuator.Broker.WaitFor(ctx, events.HOLDSTOBUY, "")
			figsChan := w.actuator.Broker.WaitFor(ctx, events.FIGSTOBUY, "")
			shieldsChan := w.actuator.Broker.WaitFor(ctx, events.SHIELDSTOBUY, "")

			// buy holds
			select {
			case <-ctx.Done():
				return
			case e := <-holdsChan:
				w.actuator.Sendf("%d\ry", e.DataInt)
			}

			// buy figs
			select {
			case <-ctx.Done():
				return
			case e := <-figsChan:
				if e.DataInt > 0 {
					w.actuator.Sendf("b%d\r", e.DataInt)
				}
			}

			// buy shields
			select {
			case <-ctx.Done():
				return
			case e := <-shieldsChan:
				if e.DataInt > 0 {
					w.actuator.Sendf("c%d\r", e.DataInt)
				}
			}

			w.actuator.Send("qx\rq")
			// wait for the parser
			select {
			case <-ctx.Done():
				return
			case e := <-w.actuator.Broker.WaitFor(ctx, events.AVAILABLESHIPS, fmt.Sprint(w.shipOther.ID)):
				w.shipOther.sector = e.DataInt
			}

			err = w.actuator.Move(ctx, w.shipOther.sector, w.genMoveOptions(), false)
			if err != nil {
				fmt.Printf("stopping WSST: %s\n", err.Error())
				return
			}
		}

		// at an xxB?
		// if no, try to move to an adjacent xxB
		// if none available, pick a next sector and move (avoid 1-way warps)
		// if other ship is at max xport range, xport and move to this sector, then continue search.

		// once at xxB, xport and initiate search for an xxB within range

		// once both are under xxB
		// if holds have equ, sell it, steal back
		// else if needed, upgrade port, steal
		// xport and repeat

		// on bust:
		// goto 1 and buy holds/shields/figs
		// go to location of other ship
		// search for a new xxB
	}
}

func (w *wsst) changeShips(ctx context.Context) error {
	w.actuator.Sendf("x%d\rq", w.shipOther.ID)
	w.shipCurrent, w.shipOther = w.shipOther, w.shipCurrent

	// make sure the xport was successful
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.actuator.Broker.WaitFor(ctx, events.SHIPNOTAVAILABLE, ""):
		return fmt.Errorf("ship not available for xport")
	case <-w.actuator.Broker.WaitFor(ctx, events.AVAILABLESHIPS, fmt.Sprint(w.shipOther.ID)):
	}

	return nil
}

func (w *wsst) sst(ctx context.Context) (bool, error) {

	for i := 0; ; i++ {
		if i%5 == 0 {
			// periodically update stats to track exp
			w.actuator.Send("/")
		} else if w.actuator.Data.Status.Exp < 35*w.actuator.Data.Status.Holds {
			// except when exp is low, then track it more often
			w.actuator.Send("/")
		}

		if i == 0 {
			err := w.preparePort(ctx)
			if err != nil {
				return false, err
			}
		} else {
			w.sell(ctx)
		}

		busted, err := w.steal(ctx)
		if busted || err != nil {
			fmt.Printf("SST done after %d rounds\n", i)
			return busted, err
		}

		err = w.changeShips(ctx)
		if err != nil {
			return false, err
		}

		if i == 0 {
			err := w.preparePort(ctx)
			if err != nil {
				return false, err
			}
		} else {
			w.sell(ctx)
		}

		busted, err = w.steal(ctx)
		if busted || err != nil {
			fmt.Printf("SST done after %d rounds\n", i)
			return busted, err
		}

		err = w.changeShips(ctx)
		if err != nil {
			return false, err
		}
	}
}

func (w *wsst) preparePort(ctx context.Context) error {
	w.actuator.QuickStats(ctx)

	if w.actuator.Data.Status.Equ > 0 {
		err := w.sell(ctx)
		if err != nil {
			return err
		}
	}

	// jettison anything else we have
	w.actuator.Sendf("jy")
	return nil
}

func (w *wsst) sell(ctx context.Context) error {
	w.actuator.Send("pt")

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.SELLPROMPT):
		// sell all
		w.actuator.Send("\r")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.actuator.Broker.WaitFor(ctx, events.PORTNOTINTERESTED, ""):
		// TODO make sure we're not asked next to buy something else. For
		// example, got a "not interested" response negotiating to buy fuel, but
		// then get a prompt to buy org.
		// try again
		return w.sell(ctx)
	case <-w.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.COMMANDPROMPT):
		return nil
	case <-w.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.BUYPROMPT):
		// don't buy anything
		sector, _ := w.actuator.Data.GetSector(w.actuator.Data.Status.Sector)
		if sector.Port == nil || sector.Port.Report == nil {
			return fmt.Errorf("unexpected nill port report")
		}
		if sector.Port.Report.Fuel.Status == models.SELLING {
			w.actuator.Send("0\r")
		}
		if sector.Port.Report.Org.Status == models.SELLING {
			w.actuator.Send("0\r")
		}
	}

	return nil
}

func (w *wsst) steal(ctx context.Context) (bool, error) {
	holds := w.actuator.Data.Status.Holds

	holdsToSteal := min(holds, w.actuator.Data.Status.Exp/30)

	w.actuator.Send("pr\rs3")

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case e := <-w.actuator.Broker.WaitFor(ctx, events.PORTEQUTOSTEAL, ""):
		available := e.DataInt
		if available < holds {
			upgrade := int((holds - available) / 10)
			if (holds-available)%10 > 0 {
				upgrade += 1
			}
			w.actuator.Sendf("0\ro3%d\rq", upgrade)
			w.actuator.Send("pr\rs3")
		}
	}
	w.actuator.Sendf("%d\r", holdsToSteal)

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-w.actuator.Broker.WaitFor(ctx, events.STEALRESULT, string(events.CRIMESUCCESS)):
		return false, nil
	// sometimes this one gets obscured by a fig hit, so the next WaitFor
	// ensures we notice either way.
	case <-w.actuator.Broker.WaitFor(ctx, events.STEALRESULT, string(events.CRIMEBUSTED)):
		return true, nil
	case <-w.actuator.Broker.WaitFor(ctx, events.BUSTED, ""):
		return true, nil
	}
}

func (w *wsst) getSectorWithVisit(ctx context.Context, sectorID int) (*persist.Sector, error) {
	sector, ok := w.actuator.Data.Persist.SectorCache.Get(sectorID)
	if ok {
		return sector, nil
	}
	// visit the sector, holo-scan
	err := w.actuator.Move(ctx, sectorID, w.genMoveOptions(), false)
	if err != nil {
		return nil, err
	}
	w.actuator.Send("sh")

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.actuator.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sectorID)):
	}

	sector, ok = w.actuator.Data.Persist.SectorCache.Get(sectorID)
	if !ok {
		return nil, fmt.Errorf("failed to get sector details even after visiting it")
	}
	return sector, nil
}

func (w *wsst) checkDistance(ctx context.Context, distance, sectorA, sectorB int) bool {
	outbound, err := w.actuator.RouteFromTo(ctx, sectorA, sectorB)
	if err != nil {
		return false
	}
	if len(outbound) >= distance {
		return false
	}

	inbound, err := w.actuator.RouteFromTo(ctx, sectorB, sectorA)
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
	if sectorA != w.shipOther.sector {
		aToOtherShip, err := w.actuator.RouteFromTo(ctx, sectorA, w.shipOther.sector)
		if err != nil {
			return false
		}
		if len(aToOtherShip) > distance+1 {
			return false
		}
	}

	return true
}

func (w *wsst) findPorts(ctx context.Context) error {
	visited := map[int]struct{}{}

OUTER:
	for {
		fmt.Println("####################### START FIND PORTS ITERATION ##########################")
		start := w.actuator.Data.Status.Sector
		visited[start] = struct{}{}

		// holo-scan
		w.actuator.Send("sh")

		candidates, unexplored := w.findXXBs(ctx, start, 5, []int{})

		for _, candidate := range candidates {
			sector, err := w.getSectorWithVisit(ctx, candidate)
			if err != nil {
				return err
			}
			if w.portCanBeUsed(ctx, sector) {
				fmt.Printf("found suitable portA: %d\n", sector.ID)
				// look for a companion
				companions, cUnexplored := w.findXXBs(ctx, candidate, 5, []int{int(sector.ID)})
				fmt.Printf("%d potential companions\n", len(companions))
				for _, companion := range companions {
					fmt.Printf("considering companion %d\n", companion)
					cSector, err := w.getSectorWithVisit(ctx, companion)
					if err != nil {
						return err
					}
					if w.portCanBeUsed(ctx, cSector) && w.checkDistance(ctx, 5, candidate, companion) {
						fmt.Printf("found a pair: %d, %d\n", candidate, companion)

						return w.moveShipsIntoPosition(ctx, candidate, companion)
					}
				}
				// explore
				for _, uc := range cUnexplored {
					fmt.Printf("moving to unexplored sector %d\n", uc)
					err = w.actuator.Move(ctx, uc, w.genMoveOptions(), false)
					if err != nil {
						return err
					}
					cSector, ok := w.actuator.Data.Persist.SectorCache.Get(uc)
					if !ok {
						fmt.Println("cound not get current sector from cache")
						continue
					}
					if w.portCanBeUsed(ctx, cSector) && w.checkDistance(ctx, 5, candidate, uc) {
						return w.moveShipsIntoPosition(ctx, candidate, uc)
					}

				}
				// out of companions to check. Try another primary candidate.
				fmt.Printf("Giving up on primary candidate %d\n", sector.ID)
			}

		}
		if len(unexplored) > 0 {
			// move to a random unexplored sector and try again
			rand.Seed(time.Now().UnixNano())
			rand.Shuffle(len(unexplored), func(i, j int) { unexplored[i], unexplored[j] = unexplored[j], unexplored[i] })
			for _, u := range unexplored {
				returnRoute, err := w.actuator.RouteFromTo(ctx, u, w.shipCurrent.sector)
				if err != nil {
					return err
				}
				if len(returnRoute) <= 6 {
					err = w.actuator.Move(ctx, u, w.genMoveOptions(), false)
					if err != nil {
						return err
					}
					err = w.changeShips(ctx)
					if err != nil {
						return err
					}
					err = w.actuator.Move(ctx, u, w.genMoveOptions(), false)
					if err != nil {
						return err
					}
					continue OUTER
				}
			}
		}

		// in case we went exploring to get sector info
		fmt.Println("moving back to the other ship to start towing it")
		w.updateOtherShipSector(ctx)
		err := w.actuator.Move(ctx, w.shipOther.sector, w.genMoveOptions(), false)
		if err != nil {
			return err
		}

		current, _ := w.actuator.Data.GetSector(w.actuator.Data.Status.Sector)
		safeHops := []int{}
		for _, warp := range current.Warps {
			s, ok := w.actuator.Data.GetSector(warp)
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
			return fmt.Errorf("no safe moves")
		}

		unexplored = w.actuator.Data.Persist.WarpCache.TrimExplored(safeHops)

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

		w.actuator.Sendf("wn%d\r", w.shipOther.ID)
		err = w.actuator.Move(ctx, next, w.genMoveOptions(), false)
		if err != nil {
			return err
		}
		w.actuator.Send("w")
	}
}

func (w *wsst) moveShipsIntoPosition(ctx context.Context, a, b int) error {
	err := w.actuator.Move(ctx, a, w.genMoveOptions(), false)
	if err != nil {
		return err
	}
	// switch ships
	err = w.changeShips(ctx)
	if err != nil {
		return err
	}
	err = w.actuator.Move(ctx, b, w.genMoveOptions(), false)
	if err != nil {
		return err
	}

	return nil
}

func (w *wsst) findXXBs(ctx context.Context, start, distance int, exclude []int) ([]int, []int) {
	XXBs := []int{}
	unexplored := []int{}

	checked := map[int]struct{}{}

	toCheck := []int{start}

	eMap := map[int]struct{}{}
	for _, e := range exclude {
		eMap[e] = struct{}{}
	}

	for i := 0; i < distance; i++ {
		nextToCheck := []int{}
		for _, sector := range toCheck {
			fmt.Printf("checking sector %d\n", sector)
			// mark this sector as checked
			checked[sector] = struct{}{}

			// check if this sector is a match
			s, ok := w.actuator.Data.Persist.SectorCache.Get(sector)
			if !ok {
				fmt.Println("not in sectorcache")
				unexplored = append(unexplored, sector)
				continue
			}
			if s.Equ == persist.BUYING {
				_, ok := eMap[sector]
				if ok {
					fmt.Println("excluded")
				} else {
					fmt.Println("adding")
					XXBs = append(XXBs, sector)
				}
			}
			// determine which neighbors to check next
			warps, ok := w.actuator.Data.Persist.WarpCache.Get(sector)
			if !ok {
				// query warps and try again; maybe we've holo-scaned the
				// sector, but not visited, and thus don't have warp data yet.
				w.actuator.QueryWarps(ctx, sector, true)
				warps, ok = w.actuator.Data.Persist.WarpCache.Get(sector)
				if !ok {
					unexplored = append(unexplored, sector)
					continue
				}

			}
			for _, warp := range warps {
				_, alreadyChecked := checked[warp]
				if alreadyChecked {
					continue
				}

				nextToCheck = append(nextToCheck, warp)
			}
		}
		toCheck = nextToCheck

	}

	return XXBs, unexplored
}

type ship struct {
	ID     int
	sector int
}

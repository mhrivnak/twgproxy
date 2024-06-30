package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actions/sst"
	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

type wsst struct {
	actuator    *actuator.Actuator
	done        chan struct{}
	shipCurrent *ship
	shipOther   *ship
	xportRange  int
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

func (w *wsst) updateOtherShipSector(ctx context.Context) {
	w.actuator.Send("x\r\r")
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
		EnemyFigsMax: (w.actuator.Data.Status.Figs + w.actuator.Data.Status.Shields) / 4,
		MinFigs:      100,
		AutoAvoid:    true,
	}
}

func (w *wsst) run(ctx context.Context) {
	defer close(w.done)

	w.actuator.QuickStats(ctx)
	if w.actuator.Data.Status.StarDock == 0 {
		err := w.actuator.GetGameConfig(ctx)
		if err != nil {
			fmt.Println("error getting game config")
			return
		}
	}

	// figure out where the ships are and get them to the same sector
	currentSectorID := w.actuator.Data.Status.Sector
	w.shipCurrent = &ship{
		ID:     w.actuator.Data.Status.Ship,
		sector: currentSectorID,
	}

	w.updateOtherShipSector(ctx)

	// set the xport range
	w.xportRange = w.actuator.CurrentXportRange(ctx)

	// if exp isn't enough to rob all the holds, bust planets at SD first
	expShortfall := 30*w.actuator.Data.Status.Holds - w.actuator.Data.Status.Exp
	if expShortfall > 0 {
		fmt.Printf("need %d exp\n", expShortfall)
		w.actuator.GoToSD(ctx)
		w.actuator.BustPlanets(ctx, expShortfall)
	}

	mo := w.genMoveOptions().WithBuy(models.PRODUCTEQU)
	if w.shipCurrent.sector != w.shipOther.sector {
		err := w.actuator.Move(ctx, w.shipOther.sector, mo, false)
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

		err, sectA, sectB := sst.FindPorts(ctx, w.actuator, w.genMoveOptions(),
			w.xportRange, w.shipCurrent.ID, w.shipOther.sector, w.moveShips)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		w.moveShipsIntoPosition(ctx, sectA, sectB)

		sstRun := sst.New(w.actuator, w.shipCurrent.ID, w.shipOther.ID)
		err = sstRun.Run(ctx)

		if err != nil {
			fmt.Printf("error during SST: %s\n", err.Error())
			return
		}

		// update ship tracking in case we ended in the other one
		if sstRun.ShipCurrent() != w.shipCurrent.ID {
			w.shipCurrent, w.shipOther = w.shipOther, w.shipCurrent
		}

		if sstRun.Busted() {
			err = w.actuator.Move(ctx, 1, w.genMoveOptions(), false)
			if err != nil {
				fmt.Printf("stopping WSST: %s\n", err.Error())
				return
			}

			err = sst.Refurb(ctx, w.actuator)
			if err != nil {
				fmt.Printf("refurb error: %s\n", err.Error())
				return
			}

			w.actuator.Send("x\rq")
			// wait for the parser
			select {
			case <-ctx.Done():
				return
			case e := <-w.actuator.Broker.WaitFor(ctx, events.AVAILABLESHIPS, fmt.Sprint(w.shipOther.ID)):
				w.shipOther.sector = e.DataInt
			}

			// if exp isn't enough to rob all the holds, bust planets
			expShortfall := 30*w.actuator.Data.Status.Holds - w.actuator.Data.Status.Exp
			if expShortfall > 0 {
				fmt.Printf("need %d exp\n", expShortfall)
				w.actuator.GoToSD(ctx)
				w.actuator.BustPlanets(ctx, expShortfall)
			}

			err = w.actuator.Move(ctx, w.shipOther.sector, w.genMoveOptions().WithBuy(models.PRODUCTEQU), false)
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

func (w *wsst) moveShipsIntoPosition(ctx context.Context, a, b int) error {
	err := w.actuator.Move(ctx, a, w.genMoveOptions(), false)
	if err != nil {
		return err
	}
	// switch ships
	err = w.actuator.Transport(ctx, w.shipOther.ID)
	if err != nil {
		return err
	}
	w.shipCurrent, w.shipOther = w.shipOther, w.shipCurrent
	err = w.actuator.Move(ctx, b, w.genMoveOptions(), false)
	if err != nil {
		return err
	}

	return nil
}

func (w *wsst) moveShips(ctx context.Context, destination int) error {
	w.actuator.QuickStats(ctx)
	if w.shipCurrent.ID != w.actuator.Data.Status.Ship {
		w.shipCurrent, w.shipOther = w.shipOther, w.shipCurrent
	}

	w.actuator.Sendf("wn%d\r", w.shipOther.ID)
	err := w.actuator.Move(ctx, destination, w.genMoveOptions(), false)
	if err != nil {
		return err
	}
	w.actuator.Send("w")

	return nil
}

type ship struct {
	ID     int
	sector int
}

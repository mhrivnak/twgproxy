package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/mhrivnak/twgproxy/pkg/bot/actions/sst"
	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

type wsste struct {
	actuator          *actuator.Actuator
	done              chan struct{}
	shipPairCurrent   *shipPair
	shipPairOther     *shipPair
	xportRange        int
	holds             int
	escortMoveOptions actuator.MoveOptions
	traderMoveOptions actuator.MoveOptions
}

type shipPair struct {
	escort *ship
	trader *ship
}

func NewWSSTE(a *actuator.Actuator, traderCurrent, escortOther, traderOther int) Action {
	done := make(chan struct{})
	emo := actuator.MoveOptions{
		DropFigs:     1,
		EnemyFigsMax: 10000,
		MinFigs:      10000,
		BuyProduct:   models.PRODUCTFUEL,
	}
	tmo := actuator.MoveOptions{
		BuyProduct: models.PRODUCTEQU,
	}
	return &wsste{
		actuator: a,
		done:     done,
		shipPairCurrent: &shipPair{
			trader: &ship{
				ID:     traderCurrent,
				sector: a.Data.Status.Sector,
			},
		},
		shipPairOther: &shipPair{
			escort: &ship{
				ID: escortOther,
			},
			trader: &ship{
				ID: traderOther,
			},
		},
		escortMoveOptions: emo,
		traderMoveOptions: tmo,
	}
}

func (w *wsste) Start(ctx context.Context) <-chan struct{} {
	go w.run(ctx)
	return w.done
}

func (w *wsste) run(ctx context.Context) {
	defer close(w.done)

	w.actuator.QuickStats(ctx)
	if w.actuator.Data.Status.StarDock == 0 {
		err := w.actuator.GetGameConfig(ctx)
		if err != nil {
			fmt.Println("error getting game config")
			return
		}
	}

	// set current ship info
	currentSectorID := w.actuator.Data.Status.Sector
	fmt.Printf("current ship: %d\n", w.actuator.Data.Status.Ship)

	w.shipPairCurrent.escort = &ship{
		ID:     w.actuator.Data.Status.Ship,
		sector: currentSectorID,
	}

	// set trader ship info
	w.actuator.Transport(ctx, w.shipPairCurrent.trader.ID)
	w.actuator.QuickStats(ctx)

	// set the xport range and holds
	w.xportRange = w.actuator.CurrentXportRange(ctx)
	w.holds = w.actuator.Data.Status.Holds

	// back to the escort
	w.actuator.Transport(ctx, w.shipPairCurrent.escort.ID)

	// set other ship sector
	otherEscort, ok := w.actuator.Data.GetShip(w.shipPairOther.escort.ID)
	if !ok {
		fmt.Printf("could not find ship %d\n", w.shipPairOther.escort.ID)
		return
	}
	otherTrader, ok := w.actuator.Data.GetShip(w.shipPairOther.trader.ID)
	if !ok {
		fmt.Printf("could not find ship %d\n", w.shipPairOther.trader.ID)
		return
	}
	if otherEscort.Sector != otherTrader.Sector {
		fmt.Printf("other ships not in same sector\n")
		return
	}
	w.shipPairOther.escort.sector = otherEscort.Sector
	w.shipPairOther.trader.sector = otherTrader.Sector

	// if exp isn't enough to rob all the holds, bust planets at SD first
	expShortfall := 30*w.holds - w.actuator.Data.Status.Exp
	if expShortfall > 0 {
		fmt.Printf("need %d exp\n", expShortfall)
		ret := w.actuator.Data.Status.Sector
		w.actuator.Move(ctx, w.actuator.Data.Status.StarDock, w.escortMoveOptions, false)
		w.actuator.BustPlanets(ctx, expShortfall)
		w.actuator.Move(ctx, ret, w.escortMoveOptions, false)
	}

	if w.actuator.Data.Status.Sector != w.shipPairOther.escort.sector {
		w.actuator.MoveWith(ctx, w.shipPairOther.escort.sector, w.shipPairCurrent.trader.ID,
			w.escortMoveOptions, &w.traderMoveOptions)
	}

	for {
		if ctx.Err() != nil {
			return
		}

		// update ship sectors
		currentSectorID = w.actuator.Data.Status.Sector
		w.shipPairCurrent.escort.sector = currentSectorID
		w.shipPairCurrent.trader.sector = currentSectorID
		w.shipPairOther.escort.sector = currentSectorID
		w.shipPairOther.trader.sector = currentSectorID

		// holo-scan
		w.actuator.Send("sh")
		// wait for the sectors to be parsed
		select {
		case <-ctx.Done():
			return
		case <-w.actuator.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(currentSectorID)):
		}

		// find a port pair to use
		err, sectA, sectB := sst.FindPorts(ctx, w.actuator, w.escortMoveOptions,
			w.xportRange, w.shipPairCurrent.escort.ID, w.shipPairOther.escort.sector, w.moveShips)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		err = w.updateShipInfo(ctx)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		// make sure we're in the same sector as the trader
		if w.shipPairCurrent.escort.sector != w.shipPairCurrent.trader.sector {
			err = w.actuator.Move(ctx, w.shipPairCurrent.trader.sector, w.escortMoveOptions, false)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
		}

		// move one pair to sectA
		if w.shipPairCurrent.trader.sector != sectA {
			err = w.actuator.MoveWith(ctx, sectA, w.shipPairCurrent.trader.ID, w.escortMoveOptions, &w.traderMoveOptions)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			w.shipPairCurrent.escort.sector = sectA
			w.shipPairCurrent.trader.sector = sectA
		}
		w.deployDefenses(ctx)

		err = w.actuator.Transport(ctx, w.shipPairOther.escort.ID)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		w.shipPairCurrent, w.shipPairOther = w.shipPairOther, w.shipPairCurrent

		// move one pair to sectB
		if w.shipPairCurrent.trader.sector != sectB {
			err = w.actuator.MoveWith(ctx, sectB, w.shipPairCurrent.trader.ID, w.escortMoveOptions, &w.traderMoveOptions)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			w.shipPairCurrent.escort.sector = sectB
			w.shipPairCurrent.trader.sector = sectB
		}
		w.deployDefenses(ctx)

		// get in the trader
		err = w.actuator.Transport(ctx, w.shipPairCurrent.trader.ID)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		sstRun := sst.New(w.actuator, w.shipPairCurrent.trader.ID, w.shipPairOther.trader.ID)
		err = sstRun.Run(ctx)
		if err != nil {
			fmt.Printf("error during SST: %s\n", err.Error())
			return
		}

		// update ship tracking in case we ended in the other one
		if sstRun.ShipCurrent() == w.shipPairOther.trader.ID {
			w.shipPairCurrent, w.shipPairOther = w.shipPairOther, w.shipPairCurrent
		}

		// get in the other escort to retrieve defenses
		err = w.actuator.Transport(ctx, w.shipPairOther.escort.ID)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		w.retrieveDefenses(ctx)

		// get in the current escort to retrieve defenses
		err = w.actuator.Transport(ctx, w.shipPairCurrent.escort.ID)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		w.retrieveDefenses(ctx)

		if !sstRun.Busted() {
			return
		}

		err = w.actuator.MoveWith(ctx, 1, w.shipPairCurrent.trader.ID, w.escortMoveOptions, nil)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		// refurb the escort
		err = sst.Refurb(ctx, w.actuator)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		err = w.actuator.Transport(ctx, w.shipPairCurrent.trader.ID)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		w.actuator.QuickStats(ctx)

		// refurb the trader
		err = sst.Refurb(ctx, w.actuator)
		if err != nil {
			fmt.Println(err.Error())
			// best effort transport to the escort
			w.actuator.Transport(ctx, w.shipPairCurrent.escort.ID)
			return
		}

		err = w.actuator.Transport(ctx, w.shipPairCurrent.escort.ID)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		// work-around for issues with the current ship status not being
		// up-to-date at this point
		time.Sleep(time.Second)

		// if exp isn't enough
		expShortfall := 30*w.holds - w.actuator.Data.Status.Exp
		if expShortfall > 0 {
			fmt.Printf("need %d exp\n", expShortfall)
			err = w.actuator.MoveWith(ctx, w.actuator.Data.Status.StarDock, w.shipPairCurrent.trader.ID, w.escortMoveOptions, nil)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			err = w.actuator.BustPlanets(ctx, expShortfall)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
		}

		err = w.actuator.MoveWith(ctx, w.shipPairOther.escort.sector, w.shipPairCurrent.trader.ID, w.escortMoveOptions, &w.traderMoveOptions)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
	}
}

func (w *wsste) deployDefenses(ctx context.Context) {
	w.actuator.Send("f10000\rcdh1100\rc")
}

func (w *wsste) retrieveDefenses(ctx context.Context) {
	w.actuator.Send("f1\rcdh10\r")
}

func (w *wsste) updateShipInfo(ctx context.Context) error {
	w.actuator.QuickStats(ctx)
	if w.shipPairOther.escort.ID == w.actuator.Data.Status.Ship || w.shipPairOther.trader.ID == w.actuator.Data.Status.Ship {
		w.shipPairCurrent, w.shipPairOther = w.shipPairOther, w.shipPairCurrent
	}
	currentShip := w.actuator.Data.Status.Ship

	shipToSector := map[int]int{
		currentShip: w.actuator.Data.Status.Sector,
	}

	waits := map[int]<-chan *events.Event{
		w.shipPairCurrent.escort.ID: w.actuator.Broker.WaitFor(ctx, events.AVAILABLESHIPS, fmt.Sprint(w.shipPairCurrent.escort.ID)),
		w.shipPairCurrent.trader.ID: w.actuator.Broker.WaitFor(ctx, events.AVAILABLESHIPS, fmt.Sprint(w.shipPairCurrent.trader.ID)),
		w.shipPairOther.escort.ID:   w.actuator.Broker.WaitFor(ctx, events.AVAILABLESHIPS, fmt.Sprint(w.shipPairOther.escort.ID)),
		w.shipPairOther.trader.ID:   w.actuator.Broker.WaitFor(ctx, events.AVAILABLESHIPS, fmt.Sprint(w.shipPairOther.trader.ID)),
	}
	delete(waits, currentShip)

	w.actuator.Send("x\r\r")

	for shipID, wait := range waits {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-wait:
			shipToSector[shipID] = e.DataInt
		}
	}

	w.shipPairCurrent.escort.sector = shipToSector[w.shipPairCurrent.escort.ID]
	w.shipPairCurrent.trader.sector = shipToSector[w.shipPairCurrent.trader.ID]
	w.shipPairOther.escort.sector = shipToSector[w.shipPairOther.escort.ID]
	w.shipPairOther.trader.sector = shipToSector[w.shipPairOther.trader.ID]

	return nil
}

// moveShips moves all of the ships to one sector
func (w *wsste) moveShips(ctx context.Context, dest int) error {
	err := w.updateShipInfo(ctx)
	if err != nil {
		return err
	}

	err = w.moveCurrentShipsTo(ctx, dest)
	if err != nil {
		return err
	}

	err = w.actuator.Transport(ctx, w.shipPairOther.escort.ID)
	if err != nil {
		return err
	}
	w.shipPairCurrent, w.shipPairOther = w.shipPairOther, w.shipPairCurrent

	err = w.moveCurrentShipsTo(ctx, dest)
	if err != nil {
		return err
	}

	return nil
}

func (w *wsste) moveCurrentShipsTo(ctx context.Context, dest int) error {
	var err error

	// make sure we're in the escort
	if w.actuator.Data.Status.Ship != w.shipPairCurrent.escort.ID {
		err = w.actuator.Transport(ctx, w.shipPairCurrent.escort.ID)
		if err != nil {
			return err
		}
	}

	// make sure we're in the same sector as the trader
	if w.shipPairCurrent.escort.sector != w.shipPairCurrent.trader.sector {
		err = w.actuator.Move(ctx, w.shipPairCurrent.trader.sector, w.escortMoveOptions, false)
		if err != nil {
			return err
		}
	}

	if w.shipPairCurrent.trader.sector != dest {
		err = w.actuator.MoveWith(ctx, dest, w.shipPairCurrent.trader.ID, w.escortMoveOptions, &w.traderMoveOptions)
		if err != nil {
			return err
		}
	}

	return nil
}

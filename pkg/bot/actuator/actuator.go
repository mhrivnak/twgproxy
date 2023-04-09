package actuator

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func New(broker *events.Broker, data *models.Data, writer io.Writer) *Actuator {
	return &Actuator{
		Broker:        broker,
		Data:          data,
		commandWriter: writer,
	}
}

type Actuator struct {
	Broker        *events.Broker
	Data          *models.Data
	commandWriter io.Writer
}

func (a *Actuator) Send(command string) error {
	_, err := a.commandWriter.Write([]byte(command))
	return err
}

func (a *Actuator) Sendf(command string, args ...any) error {
	return a.Send(fmt.Sprintf(command, args...))
}

func (a *Actuator) Land(planetID int) {
	a.Send(fmt.Sprintf("l%d\r", planetID))
}

func (a *Actuator) MombotSend(ctx context.Context, command string) {
	a.Send(">")
	select {
	case <-ctx.Done():
		return
	case <-a.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.MOMBOTPROMPT):
		break
	}
	a.Send(command)
}

func (a *Actuator) QuickStats(ctx context.Context) {
	a.Send("/")

	select {
	case <-ctx.Done():
		return
	case <-a.Broker.WaitFor(ctx, events.QUICKSTATDISPLAY, ""):
	}
}

func (a *Actuator) RouteWalk(ctx context.Context, points []int, task func()) {
	a.QuickStats(ctx)

	completed := map[int]struct{}{}

	for _, point := range points {
		route, err := a.RouteTo(ctx, point)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		for i, sectorID := range route {
			if i > 0 {
				// move to the next sector
				err = a.MoveSafe(ctx, sectorID, true)
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

			// run the provided task
			task()

			// wait for a prompt before proceeding; sometimes TWX scripts
			// terminate before all of their commands are done
			a.Send("\r")
			select {
			case <-ctx.Done():
				return
			case <-a.Broker.WaitFor(ctx, events.PROMPTDISPLAY, ""):
			}
		}
	}
}

func (a *Actuator) MassUpgrade(ctx context.Context, block bool) error {
	a.Send("$ss2_massupgrade\rg")

	if block {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.Broker.WaitFor(ctx, events.TWXSCRIPTTERM, ""):
		}
	}

	return nil
}

func (a *Actuator) RouteTo(ctx context.Context, sector int) ([]int, error) {
	current, ok := a.Data.GetSector(a.Data.Status.Sector)
	if !ok {
		return nil, fmt.Errorf("current sector %d not found in cache", a.Data.Status.Sector)
	}
	for _, warp := range current.Warps {
		if warp == sector {
			// no need to plot a route if the destination is next door
			return []int{current.ID, sector}, nil
		}
	}

	// send commands
	a.Send(fmt.Sprintf("cf\r%d\rq", sector))

	// wait for events
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case e := <-a.Broker.WaitFor(ctx, events.ROUTEDISPLAY, ""):
		fmt.Printf("got route: %s\n", e.Data)
		return parseSectors(e.Data)
	}
}

type MoveOptions struct {
	DropFigs      int
	EnemyFigsMax  int
	EnemyMinesMax int
	MinFigs       int
}

func (a *Actuator) MoveSafe(ctx context.Context, dest int, block bool) error {
	return a.Move(ctx, dest, MoveOptions{}, block)
}

func (a *Actuator) Move(ctx context.Context, dest int, opts MoveOptions, block bool) error {
	// make sure we know what kind of long range scanner is available
	a.QuickStats(ctx)

	fmt.Printf("MOVE to %d\n", dest)
	sectors, err := a.RouteTo(ctx, dest)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	stop := make(chan interface{})
	defer close(stop)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-a.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.MINEDSECTORPROMPT):
				a.Send("\r")
			}
		}
	}()

	// ignore the first sector, which is the one we're in
	for _, sector := range sectors[1:] {
		attackCommand := ""
		if a.Data.Status.LRS == models.LRSHOLO {
			a.Send("sh")

			select {
			case <-a.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sector)):
				sInfo, ok := a.Data.GetSector(sector)
				if !ok {
					return fmt.Errorf("failed to get cached info on sector %d", sector)
				}
				switch {
				case !sInfo.FigsFriendly && sInfo.Figs > opts.EnemyFigsMax:
					return fmt.Errorf("too many enemy figs ahead")
				case !sInfo.MinesFriendly && sInfo.Mines > opts.EnemyMinesMax:
					return fmt.Errorf("too many enemy mines ahead")
				}
				if sInfo.Figs > 0 && !sInfo.FigsFriendly && sInfo.FigType != models.FigTypeOffensive {
					attackCommand = "a999\r"
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		a.Sendf("%d\r", sector)
		if attackCommand != "" {
			a.Send(attackCommand)
		}

		// wait for the next sector to display
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sector)):
		}

		sInfo, ok := a.Data.GetSector(sector)
		if !ok {
			return fmt.Errorf("failed to get cached info on sector %d", sector)
		}
		// should we drop figs and we have enough?
		if opts.DropFigs > 0 && !sInfo.IsFedSpace && a.Data.Status.Figs-opts.DropFigs >= opts.MinFigs {
			// does the sector need more figs?
			if sInfo.Figs < opts.DropFigs {
				a.Sendf("f%d\rcd", opts.DropFigs)
			}
		}
	}

	if block {
		for {
			select {
			case e := <-a.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.COMMANDPROMPT):
				if e.DataInt == dest {
					return nil
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return nil
}

func (a *Actuator) LandNewest(ctx context.Context) error {
	var planetIDs []int

	a.Send("l")
	select {
	case e := <-a.Broker.WaitFor(ctx, events.PLANETLANDINGDISPLAY, ""):
		planetIDs = e.DataSliceInt
	case <-ctx.Done():
		return ctx.Err()
	}

	sort.Ints(planetIDs)

	newest := planetIDs[len(planetIDs)-1]
	a.Send(fmt.Sprintf("%d\r", newest))

	return nil
}

func (a *Actuator) Rob(ctx context.Context) {
	a.Send("d/pr\rr")

	select {
	case <-ctx.Done():
		break
	case e := <-a.Broker.WaitFor(ctx, events.PORTROBCREDS, ""):
		creds := e.DataInt
		// Make sure the port has at least 1/3 the max that can be robbed, to
		// make the risk worthwhile.
		if creds < a.Data.Status.Exp {
			fmt.Println("not enough creds to rob")
			a.Send("0\r")
			a.Broker.Publish(&events.Event{
				Kind: events.ROBRESULT,
				ID:   string(events.CRIMEABORT),
			})
			return
		}
		credsToRob := int(float32(creds) * 1.11)
		maxToRob := 3 * a.Data.Status.Exp

		if credsToRob > maxToRob {
			credsToRob = maxToRob
		}

		a.Send(fmt.Sprintf("%d\r", credsToRob))
	}
}

func (a *Actuator) Express(destination int) {
	if a.Data.Status.TWarp == models.TWarpTypeNone {
		a.Send(fmt.Sprintf("%d\re", destination))
	} else {
		a.Send(fmt.Sprintf("%d\rne", destination))
	}
}

func (a *Actuator) CIMSectorUpdate(ctx context.Context) {
	a.Send("^iq")
}

// BuyGTorpsAndDetonators buys the max gtorps and detonators. Must be run from
// stardock sector. As an implementation detail, it declines to buy each item
// once so the event parser can observe the max that's possible to buy.
func (a *Actuator) BuyGTorpsAndDetonators(ctx context.Context) {
	a.Send("psha\r")

	select {
	case <-ctx.Done():
		return
	case e := <-a.Broker.WaitFor(ctx, events.DETONATORBUYMAX, ""):
		a.Send(fmt.Sprintf("a%d\r", e.DataInt))
	}

	a.Send("t\r")
	select {
	case <-ctx.Done():
		return
	case e := <-a.Broker.WaitFor(ctx, events.GTORPBUYMAX, ""):
		a.Send(fmt.Sprintf("t%d\r", e.DataInt))
	}
	a.Send("qq")
}

func (a *Actuator) GoToSD(ctx context.Context) error {
	for _, hop := range a.Data.Settings.HopsToSD {
		err := a.Twarp(ctx, hop.Sector)
		if err != nil {
			return err
		}
		a.Land(hop.Planet)
		a.Send("t\r\r1\rq")
	}
	a.MoveSafe(ctx, 8657, false)
	return nil
}

func (a *Actuator) Twarp(ctx context.Context, destination int) error {
	sector, ok := a.Data.GetSector(a.Data.Status.Sector)
	if !ok {
		return fmt.Errorf("current sector not in cache")
	}

	// nothing to do
	if sector.ID == destination {
		return nil
	}

	a.Send(fmt.Sprintf("%d\r", destination))
	// if it's adjacent, no need for twarp
	if sector.IsAdjacent(destination) {
		return nil
	}

	a.Send("y")

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.Broker.WaitFor(ctx, events.BLINDJUMP, ""):
		a.Send("n")
		return fmt.Errorf("aborting due to blind jump")
	case <-a.Broker.WaitFor(ctx, events.BLINDJUMP, ""):
		return fmt.Errorf("not enough fuel for the jump")
	case <-a.Broker.WaitFor(ctx, events.TWARPLOCKED, ""):
		a.Send("y")
	}
	return nil
}

func (a *Actuator) MombotPlanetSell(ctx context.Context, product models.ProductType) {
	a.MombotSend(ctx, fmt.Sprintf("neg %s\r", product))

	// wait for mombot to finish
	select {
	case <-ctx.Done():
		return
	case <-a.Broker.WaitFor(ctx, events.MBOTTRADEDONE, ""):
		return
	case <-a.Broker.WaitFor(ctx, events.MBOTNOTHINGTOSELL, ""):
		return
	}
}

func (a *Actuator) StripPlanet(ctx context.Context, fromID, toID int) error {
	wait := a.Broker.WaitFor(ctx, events.PLANETDISPLAY, "")
	a.Send(fmt.Sprintf("l%d\r", fromID))
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wait:
	case <-time.After(time.Second):
		// for some reason the planet display event is getting missed a lot.
		// fallback is to cause another planet display and just wait a second to
		// be confident it was parsed.
		fmt.Println("timeout")
		a.Send("\r")
		<-time.After(time.Second)
	}

	a.Data.PlanetLock.Lock()
	from, ok := a.Data.Planets[fromID]
	a.Data.PlanetLock.Unlock()
	if !ok {
		return fmt.Errorf("from planet not in cache")
	}

	holds := a.Data.Status.Holds

	for q := from.Ore; q > 0; q -= holds {
		if q < holds {
			a.Send(fmt.Sprintf("tnt1%d\rq", q))
		} else {
			a.Send("tnt1\rq")
		}
		a.Send(fmt.Sprintf("l%d\rtnl1\rql%d\r", toID, fromID))
	}

	for q := from.Org; q > 0; q -= holds {
		if q < holds {
			a.Send(fmt.Sprintf("tnt2%d\rq", q))
		} else {
			a.Send("tnt2\rq")
		}
		a.Send(fmt.Sprintf("l%d\rtnl2\rql%d\r", toID, fromID))
	}

	for q := from.Equ; q > 0; q -= holds {
		if q < holds {
			a.Send(fmt.Sprintf("tnt3%d\rq", q))
		} else {
			a.Send("tnt3\rq")
		}
		a.Send(fmt.Sprintf("l%d\rtnl3\rql%d\r", toID, fromID))
	}

	return nil
}

func parseSectors(route string) ([]int, error) {
	parts := strings.Split(route, " > ")
	sectors := make([]int, len(parts))
	for i := range parts {
		sector, err := strconv.Atoi(strings.Trim(parts[i], "()"))
		if err != nil {
			return nil, err
		}
		sectors[i] = sector
	}
	return sectors, nil
}

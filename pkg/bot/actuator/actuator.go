package actuator

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

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
				err = a.Move(ctx, sectorID, true)
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

func (a *Actuator) Move(ctx context.Context, dest int, block bool) error {
	// make sure we know what kind of long range scanner is available
	a.QuickStats(ctx)

	fmt.Printf("MOVE to %d\n", dest)
	sectors, err := a.RouteTo(ctx, dest)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	// ignore the first sector, which is the one we're in
	for _, sector := range sectors[1:] {
		if a.Data.Status.LRS == models.LRSHOLO {
			a.Send("sh")

			select {
			case <-a.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sector)):
				sInfo, ok := a.Data.GetSector(sector)
				if !ok {
					return fmt.Errorf("failed to get cached info on sector %d", sector)
				}
				if !sInfo.IsSafe() {
					return fmt.Errorf("unsafe sector ahead")
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		a.Send(fmt.Sprintf("%d\r", sector))
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
		if creds < 700000 {
			fmt.Println("not enough creds to rob")
			a.Send("0\r")
			a.Broker.Publish(&events.Event{
				Kind: events.ROBRESULT,
				ID:   string(events.ROBABORT),
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

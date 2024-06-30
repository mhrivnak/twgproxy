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
	"github.com/mhrivnak/twgproxy/pkg/models/persist"
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
	a.Sendf("l%d\r", planetID)
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

func (a *Actuator) QuickStatsSync(ctx context.Context, shipID int) {
	a.Send("/")

	select {
	case <-ctx.Done():
		return
	case <-a.Broker.WaitFor(ctx, events.QUICKSTATDISPLAY, fmt.Sprint(shipID)):
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
				err = a.MoveSafe(ctx, sectorID, false)
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

func (a *Actuator) RouteFromTo(ctx context.Context, from, to int) ([]int, error) {
	// send commands
	a.Send(fmt.Sprintf("cf%d\r%d\rq", from, to))

	key := fmt.Sprintf("%d:%d", from, to)

	// wait for events
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case e := <-a.Broker.WaitFor(ctx, events.ROUTEDISPLAY, key):
		fmt.Printf("got route: %s\n", e.Data)
		return parseSectors(e.Data)
	}
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

	return a.RouteFromTo(ctx, current.ID, sector)
}

type MoveOptions struct {
	DropFigs        int
	EnemyFigsMax    int
	EnemyMinesMax   int
	MinFigs         int
	SectorFunc      func(context.Context, int) error
	RefurbAndReturn bool
	AutoAvoid       bool
	BuyProduct      models.ProductType
}

func (m MoveOptions) WithoutRefurbAndReturn() MoveOptions {
	m.RefurbAndReturn = false
	return m
}

func (m MoveOptions) WithBuy(product models.ProductType) MoveOptions {
	m.BuyProduct = product
	return m
}

func (a *Actuator) MoveSafe(ctx context.Context, dest int, block bool) error {
	return a.Move(ctx, dest, MoveOptions{}, block)
}

func (a *Actuator) CurrentXportRange(ctx context.Context) int {
	ship, ok := a.Data.GetShip(a.Data.Status.Ship)
	if ok && ship.XportRange > 0 {
		return ship.XportRange
	}

	// get the current xport range
	// sending two enters works in fed space and elsewhere
	a.Send("x\r\r")
	select {
	case <-ctx.Done():
		return 0
	case e := <-a.Broker.WaitFor(ctx, events.XPORTRANGE, ""):
		return e.DataInt
	}
}

func (a *Actuator) Transport(ctx context.Context, shipID int) error {
	sector, ok := a.Data.GetSector(a.Data.Status.Sector)
	if !ok {
		return fmt.Errorf("current sector %d not found in cache", a.Data.Status.Sector)
	}
	if sector.IsFedSpace {
		// when in fed space, you get a warning that must be dismissed with an
		// extra press of ENTER
		a.Sendf("x\r%d\rq", shipID)
		return nil
	}
	a.Sendf("x%d\rq", shipID)

	// make sure the xport was successful
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.Broker.WaitFor(ctx, events.SHIPNOTAVAILABLE, ""):
		return fmt.Errorf("ship not available for xport")
	case <-a.Broker.WaitFor(ctx, events.AVAILABLESHIPS, fmt.Sprint(shipID)):
	}

	a.QuickStatsSync(ctx, shipID)
	return nil
}

// MoveWith moves the primary ship to the destination sector while periodically
// transporting back and express-warping the other ship to catch up.
//
// ctx: The context.Context object for the operation.
// dest: The destination sector to move to.
// otherShipID: The ID of the ship to bring along.
// opts: The MoveOptions to use.
// otherOpts: optional MoveOptions to use for the other ship.
// Returns an error if the move operation fails.
func (a *Actuator) MoveWith(ctx context.Context, dest, otherShipID int, opts MoveOptions, otherOpts *MoveOptions) error {
	fmt.Println("######################## MOVE WITH #########################")
	a.QuickStats(ctx)
	primaryShipID := a.Data.Status.Ship
	primaryRange := a.CurrentXportRange(ctx)

	fmt.Printf("primary ship: %d\n", primaryShipID)
	fmt.Printf("other ship: %d\n", otherShipID)
	fmt.Printf("primary range: %d\n", primaryRange)
	fmt.Printf("destination: %d\n", dest)

	sectors, err := a.RouteTo(ctx, dest)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	for i := 0; i < len(sectors)-1; {
		next, err := a.NextMoveByTransportRange(ctx, primaryRange, sectors[i:])
		if err != nil {
			return err
		}
		// 1-way warp that's too far to transport back, so must tow and go
		// through together
		if next == 0 {
			fmt.Println("towing for the next move")
			a.Sendf("wn%d\r", otherShipID)

			err = a.Move(ctx, sectors[i+1], opts, false)
			if err != nil {
				return err
			}
			// disengage tow
			a.Send("w")

			i += 1
			continue
		}

		err = a.Move(ctx, sectors[i+next], opts, false)
		if err != nil {
			return err
		}

		a.Transport(ctx, otherShipID)
		destinationWait := a.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sectors[i+next]))
		if otherOpts == nil {
			a.Sendf("%d\re", sectors[i+next])
		} else {
			a.Move(ctx, sectors[i+next], *otherOpts, false)
		}

		// wait until we arrive before transporting, so the "current sector"
		// status is updated
	OUTER:
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-destinationWait:
				break OUTER
			case <-a.Broker.WaitFor(ctx, events.NAVHAZ, ""):
				// respond to the "stop in this sector?" prompt
				a.Send("\r")
			}
		}
		a.Transport(ctx, primaryShipID)
		i += next
	}

	return nil
}

// NextMoveByTransportRange finds the next move based on the transport range being used to
// get back to the other ship.
//
// ctx: the context for the function
// xportRange: the transport range
// sectors: the list of sectors to consider
// Returns the index of the next sector to move to and any error encountered. If 0, there is no move
func (a *Actuator) NextMoveByTransportRange(ctx context.Context, xportRange int, sectors []int) (int, error) {
	start := sectors[0]
	farthest := xportRange
	if len(sectors)-1 < xportRange {
		farthest = len(sectors) - 1
	}

	for i := farthest; i > 0; i-- {
		sector := sectors[i]
		route, err := a.RouteFromTo(ctx, sector, start)
		if err != nil {
			return 0, err
		}
		if len(route)-1 <= xportRange {
			return i, nil
		}
	}
	fmt.Println("no move found; must tow!")
	return 0, nil
}

func (a *Actuator) Move(ctx context.Context, dest int, opts MoveOptions, block bool) error {
	// make sure we know what kind of long range scanner is available
	a.QuickStats(ctx)

	if a.Data.Status.Sector == dest {
		fmt.Printf("already in sector %d; no move needed\n", dest)
		return nil
	}

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
		offensiveEnemyFigs := false
		if a.Data.Status.LRS == models.LRSHOLO {
			a.Send("sh")

			select {
			case <-a.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sector)):
				sInfo, ok := a.Data.GetSector(sector)
				if !ok {
					return fmt.Errorf("failed to get cached info on sector %d", sector)
				}
				switch {
				case !sInfo.FigsFriendly && float64(sInfo.Figs)/1.1 > float64(min(opts.EnemyFigsMax, a.Data.Status.Figs)):
					if opts.AutoAvoid {
						a.Sendf("cv%d\rq", sector)
						return a.Move(ctx, dest, opts, block)
					}
					return fmt.Errorf("too many enemy figs ahead")
				case !sInfo.MinesFriendly && sInfo.Mines > opts.EnemyMinesMax:
					if opts.AutoAvoid {
						a.Sendf("cv%d\rq", sector)
						return a.Move(ctx, dest, opts, block)
					}
					return fmt.Errorf("too many enemy mines ahead")
				}
				if sInfo.Figs > 0 && !sInfo.FigsFriendly && sInfo.FigType != models.FigTypeOffensive {
					attackCommand = "a10000\r"
				}
				if sInfo.FigType == models.FigTypeOffensive && !sInfo.FigsFriendly {
					offensiveEnemyFigs = true
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// setup waits in advance so we don't miss the text
		sectorDisplayWait := a.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sector))
		var figsDestroyedWait <-chan (*events.Event)
		var shieldsAbsorbedWait <-chan (*events.Event)

		if offensiveEnemyFigs {
			figsDestroyedWait = a.Broker.WaitFor(ctx, events.FIGSDESTROYED, "")
			shieldsAbsorbedWait = a.Broker.WaitFor(ctx, events.SHIELDSABSORBEDATTACK, "")
		}

		// move to the next sector
		a.Sendf("%d\r", sector)
		if attackCommand != "" {
			a.Send(attackCommand)
		}

		// wait for the next sector to display
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sectorDisplayWait:
		}

		// wait for the message about how many figs were destroyed
		if offensiveEnemyFigs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-figsDestroyedWait:
			case <-shieldsAbsorbedWait:
			}
		}

		// if we may have lost figs, update info
		if offensiveEnemyFigs || attackCommand != "" {
			fmt.Println("getting quick stats in case we lost figs")
			a.Send("/")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-a.Broker.WaitFor(ctx, events.QUICKSTATDISPLAY, ""):
			}
		}

		sInfo, ok := a.Data.GetSector(sector)
		if !ok {
			return fmt.Errorf("failed to get cached info on sector %d", sector)
		}

		if opts.BuyProduct != models.PRODUCTNONE {
			productNumber := opts.BuyProduct.Num()
			if a.Data.Status.EmptyHolds() > 0 && sInfo.Port != nil && sInfo.Port.Type[productNumber-1] == 'S' {
				persistedPort, ok := a.Data.Persist.SectorCache.Get(sInfo.ID)
				if !ok {
					return fmt.Errorf("failed to get persisted info on sector %d", sector)
				}
				report, err := a.GetPortReport(ctx, sector, 60*time.Second)
				if err != nil {
					fmt.Printf("error getting port report: %s\n", err)
				} else {
					if report.ItemFromType(opts.BuyProduct).Trading > a.Data.Status.EmptyHolds() && persistedPort.Busted == nil {
						a.BuyProduct(ctx, opts.BuyProduct, sInfo.Port.Type)
						fmt.Println("done with call to buy product")
						a.Send("/")
					}
				}
			}
		}

		if opts.RefurbAndReturn && a.Data.Status.Figs < opts.MinFigs {
			// twarp to 1 if we have full fuel and are good
			if a.Data.Status.Alignment > 1000 && a.Data.Status.Fuel == a.Data.Status.Holds {
				a.Send("1\ryy")
			} else {
				err := a.Move(ctx, 1, opts.WithoutRefurbAndReturn(), false)
				if err != nil {
					return err
				}
			}
			err = a.Refurb(ctx)
			if err != nil {
				return err
			}
			err = a.Move(ctx, sector, opts, false)
			if err != nil {
				return err
			}
		}

		// should we drop figs and we have enough?
		if opts.DropFigs > 0 && !sInfo.IsFedSpace && a.Data.Status.Figs-opts.DropFigs >= opts.MinFigs {
			// does the sector need more figs?
			if sInfo.Figs < opts.DropFigs {
				a.Sendf("f%d\rcd", opts.DropFigs)
			}
		}

		if opts.SectorFunc != nil {
			err = opts.SectorFunc(ctx, sector)
			if err != nil {
				return err
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

func (a *Actuator) BuyProduct(ctx context.Context, product models.ProductType, portType string) {
	fmt.Printf("buying %s\n", product)

	sellsBefore := 0
	command := "pt"
	for i := 0; i < product.Num()-1; i++ {
		// TODO: If a port is selling a product but has 0, it won't present
		// a prompt to buy it.
		if portType[i] == 'S' {
			command += "0\r"
			sellsBefore += 1
		}
	}
	command += "\r"
	a.Send(command)

	// wait for the first statement with the port report
	<-a.Broker.WaitFor(ctx, events.YOUHAVECREDS, "")

	// each time the port offers to sell product, even if we decline, it will
	// give the YOUHAVECREDS line. So we need to wait for all of those lines to
	// know that this transaction is complete.
	for i := 0; i <= sellsBefore; i++ {
		fmt.Println("waiting for result of product purchase")
		select {
		case <-ctx.Done():
			return
		case <-a.Broker.WaitFor(ctx, events.PORTNOTINTERESTED, ""):
			// try again
			fmt.Println("product purchase did not go through; trying again.")
			// send two "0" ammounts in case the port also sells other products. If
			// not, these are harmless at the command prompt. Extra return
			// ensures that the sector displays. That is how we know we are done
			// with the prior port operation and can start a fresh attempt.
			a.Send("0\r0\r\r")
			<-a.Broker.WaitFor(ctx, events.SECTORDISPLAY, "")
			a.BuyProduct(ctx, product, portType)
			return
		case <-a.Broker.WaitFor(ctx, events.YOUHAVECREDS, ""):
		}
	}
	fmt.Println("product purchase was successful")
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

func (a *Actuator) BustPlanets(ctx context.Context, neededExp int) error {
	for e := 0; e < neededExp; e += 75 {
		a.Send("psha1\rt1\rqquyx\rc")
		err := a.LandNewest(ctx)
		if err != nil {
			return err
		}
		a.Send("zdy")
	}
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

func (a *Actuator) QueryWarps(ctx context.Context, sectorID int, block bool) {
	a.Sendf("ci%d\rq", sectorID)

	if block {
		select {
		case <-ctx.Done():
			return
		case <-a.Broker.WaitFor(ctx, events.SECTORWARPSDISPLAY, ""):
		case <-a.Broker.WaitFor(ctx, events.NOTVISITEDSECTORMSG, ""):
		}
	}
}

// BuyGTorpsAndDetonators buys the max gtorps and detonators. Must be run from
// stardock sector. As an implementation detail, it declines to buy each item
// once so the event parser can observe the max that's possible to buy.
// Also buys shields since they can be impacted by navhaz.
func (a *Actuator) BuyGTorpsAndDetonators(ctx context.Context) {
	a.Send("psha\r")

	// detonators
	select {
	case <-ctx.Done():
		return
	case e := <-a.Broker.WaitFor(ctx, events.DETONATORBUYMAX, ""):
		a.Send(fmt.Sprintf("a%d\r", e.DataInt))
	}

	// gtorps
	a.Send("t\r")
	select {
	case <-ctx.Done():
		return
	case e := <-a.Broker.WaitFor(ctx, events.GTORPBUYMAX, ""):
		a.Send(fmt.Sprintf("t%d\r", e.DataInt))
	}

	// shields
	a.Send("qsp")
	select {
	case <-ctx.Done():
		return
	case e := <-a.Broker.WaitFor(ctx, events.SHIELDSTOBUY, ""):
		a.Send(fmt.Sprintf("c%d\r", e.DataInt))
	}

	a.Send("qqq")
}

func (a *Actuator) GetGameConfig(ctx context.Context) error {
	a.Send("v")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.Broker.WaitFor(ctx, events.CONFIGDISPLAY, ""):
	}
	return nil
}

func (a *Actuator) GoToSD(ctx context.Context) error {
	// get stardock port if we don't already have it
	if a.Data.Status.StarDock == 0 {
		err := a.GetGameConfig(ctx)
		if err != nil {
			return err
		}
	}

	for _, hop := range a.Data.Settings.HopsToSD {
		err := a.Twarp(ctx, hop.Sector)
		if err != nil {
			return err
		}
		a.Land(hop.Planet)
		a.Send("t\r\r1\rq")
	}
	a.MoveSafe(ctx, a.Data.Status.StarDock, false)
	return nil
}

// GetPortReport returns nil, nil if a port report is not available for the sector.
func (a *Actuator) GetPortReport(ctx context.Context, sector int, maxAge time.Duration) (*models.PortReport, error) {
	if maxAge > 0 {
		report, ok := a.Data.GetPortReport(sector)
		if ok && time.Since(report.Time) < maxAge {
			return report, nil
		}
	}

	a.Sendf("cr%d\rq", sector)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.Broker.WaitFor(ctx, events.PORTREPORTDISPLAY, fmt.Sprint(sector)):
	case <-a.Broker.WaitFor(ctx, events.PORTNOINFO, ""):
		return nil, nil
	}

	report, ok := a.Data.GetPortReport(sector)
	if !ok {
		return nil, fmt.Errorf("unexpectedly didn't find port report")
	}

	return report, nil
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

func (a *Actuator) ClearVoid(sector int) {
	a.Sendf("cv0\ryn%d\rq", sector)
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

func (a *Actuator) GatherResource(ctx context.Context, product models.ProductType) error {
	var startID int
	for startID == 0 {
		wait := a.Broker.WaitFor(ctx, events.PLANETDISPLAY, "")
		a.Send("\r")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-wait:
			startID = e.DataInt
		case <-time.After(time.Second):
			// for some reason the planet display event is getting missed a lot.
			// fallback is to keep trying.
			fmt.Println("timeout waiting for planet display")
		}
	}

	a.Data.PlanetLock.Lock()
	start, ok := a.Data.Planets[startID]
	a.Data.PlanetLock.Unlock()
	if !ok {
		return fmt.Errorf("start planet not in cache")
	}

	holds := a.Data.Status.Holds

	startQuantity := start.ProductQuantity(product)
	startMax := start.ProductMax(product)

	var planetList []int

	// get the list of planets
	waitForPlanetList := a.Broker.WaitFor(ctx, events.PLANETLANDINGDISPLAY, "")
	a.Send("qlq\r")

	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-waitForPlanetList:
		planetList = e.DataSliceInt
	}

	for _, pID := range planetList {
		if pID == startID {
			continue
		}

		if startQuantity == startMax {
			a.Land(startID)
			return nil
		}

		var planet *models.Planet
		var ok bool

		for planet == nil {
			a.Land(pID)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-a.Broker.WaitFor(ctx, events.PLANETDISPLAY, fmt.Sprint(pID)):
				planet, ok = a.Data.GetPlanet(pID)
				if !ok {
					return fmt.Errorf("failed to get planet from data cache")
				}
				fmt.Printf("Got planet info for %d\n", pID)
			case <-time.NewTimer(time.Second).C:
				// occasionally the planet display event doesn't fire. Re-try.
				fmt.Printf("RETRY %d\n", pID)
				a.Send("q")
			}
		}

		toMove := min(planet.ProductQuantity(product), startMax-startQuantity)
		for q := toMove; q > 0; q -= holds {
			if q < holds {
				a.Sendf("tnt%d%d\rq", product.Num(), q)
			} else {
				a.Sendf("tnt%d\rq", product.Num())
			}
			a.Sendf("l%d\rtnl%d\rql%d\r", startID, product.Num(), pID)
		}
		startQuantity += toMove
		a.Send("q")
	}
	a.Land(startID)

	return nil
}

func (a *Actuator) RebalancePlanetPopulations(ctx context.Context) error {
	var planetList []int

	// get the list of planets
	waitForPlanetList := a.Broker.WaitFor(ctx, events.PLANETLANDINGDISPLAY, "")
	a.Send("lq\r")

	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-waitForPlanetList:
		planetList = e.DataSliceInt
	}

	for _, pID := range planetList {
		var planet *models.Planet
		var ok bool

		for planet == nil {
			a.Land(pID)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-a.Broker.WaitFor(ctx, events.PLANETDISPLAY, fmt.Sprint(pID)):
				planet, ok = a.Data.GetPlanet(pID)
				if !ok {
					return fmt.Errorf("failed to get planet from data cache")
				}
				fmt.Printf("Got planet info for %d\n", pID)
			case <-time.NewTimer(time.Second).C:
				// occasionally the planet display event doesn't fire. Re-try.
				fmt.Printf("RETRY %d\n", pID)
				a.Send("q")
			}
		}

		switch planet.Class {
		case "M":
			if planet.EquCols > 15000 {
				toMove := planet.EquCols - 14600
				a.Sendf("pn3%d\r1", toMove)
				planet.FuelCols += toMove
				planet.EquCols -= toMove
			}
			if planet.FuelCols > 15000 {
				toMove := planet.FuelCols - 14600
				a.Sendf("pn1%d\r2", toMove)
				planet.OrgCols += toMove
				planet.FuelCols -= toMove
			}
		case "O":
			if planet.OrgCols > 100000 {
				toMove := planet.OrgCols - 99000
				a.Sendf("pn2%d\r1", toMove)
				planet.FuelCols += toMove
				planet.OrgCols -= toMove
			}
		case "H":
			if planet.FuelCols > 50000 {
				toMove := planet.FuelCols - 49500
				a.Sendf("pn1%d\r3", toMove)
				planet.EquCols += toMove
				planet.FuelCols -= toMove
			}
		}
		a.Send("q")
	}

	return nil
}

func (a *Actuator) Refurb(ctx context.Context) error {
	a.Send("pt")
	shieldsChan := a.Broker.WaitFor(ctx, events.SHIELDSTOBUY, "")
	// buy shields
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-shieldsChan:
		if e.DataInt > 0 {
			a.Sendf("c%d\r", e.DataInt)
		} else {
			// re-display so a new FIGSTOBUY event displays
			a.Send("\r")
		}
	}

	figsChan := a.Broker.WaitFor(ctx, events.FIGSTOBUY, "")
	// buy figs
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-figsChan:
		if e.DataInt > 0 {
			a.Sendf("b%d\r", e.DataInt)
		}
	}

	a.Send("q/")

	return nil
}

func (a *Actuator) DisruptMines(ctx context.Context, sector int) error {
	a.QuickStats(ctx)

	// enter computer menu
	a.Send("c")

L:
	for i := 0; i < a.Data.Status.Disruptors; i++ {
		a.Sendf("wy%d\r", sector)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.Broker.WaitFor(ctx, events.MINESDESTROYED, ""):
			continue
		case <-a.Broker.WaitFor(ctx, events.MINESALLDESTROYED, ""):
			break L
		}
	}
	// exit computer menu
	a.Send("q")
	// update the number of disruptors
	a.QuickStats(ctx)
	return nil
}

// GetSectorWithVisit retrieves sector details with a visit and holo-scan if necessary
//
// ctx: context for the function
// sectorID: ID of the sector to retrieve
// moveOpts: options for moving to the sector
// (*persist.Sector, error): returns the sector details or an error
func (a *Actuator) GetSectorWithVisit(ctx context.Context, sectorID int, moveOpts MoveOptions) (*persist.Sector, error) {
	sector, ok := a.Data.Persist.SectorCache.Get(sectorID)
	if ok {
		return sector, nil
	}
	// visit the sector, holo-scan
	err := a.Move(ctx, sectorID, moveOpts, false)
	if err != nil {
		return nil, err
	}
	a.Send("sh")

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sectorID)):
	}

	sector, ok = a.Data.Persist.SectorCache.Get(sectorID)
	if !ok {
		return nil, fmt.Errorf("failed to get sector details even after visiting it")
	}
	return sector, nil
}

// findXXBs finds all sectors within a given distance of the starting sector that are
// marked as "BUYING" in the sector cache and are not excluded. It returns the
// sectors that meet the criteria and the sectors that were not explored.
//
// ctx: The context.Context object for the function.
// start: The starting sector.
// distance: The maximum distance from the starting sector to explore.
// exclude: The sectors to exclude from the search.
// []int, []int: The sectors that meet the criteria and the sectors that are unexplored.
func (a *Actuator) FindXXBPair(ctx context.Context, start, distance int, exclude []int) ([]int, []int) {
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
			s, ok := a.Data.Persist.SectorCache.Get(sector)
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
			warps, ok := a.Data.Persist.WarpCache.Get(sector)
			if !ok {
				// query warps and try again; maybe we've holo-scaned the
				// sector, but not visited, and thus don't have warp data yet.
				a.QueryWarps(ctx, sector, true)
				warps, ok = a.Data.Persist.WarpCache.Get(sector)
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

func min(nums ...int) int {
	x := nums[0]
	for i := range nums {
		if nums[i] < x {
			x = nums[i]
		}
	}
	return x
}

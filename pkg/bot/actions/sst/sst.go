package sst

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

type SST struct {
	actuator    *actuator.Actuator
	shipCurrent int
	shipOther   int
	busted      bool
}

func New(actuator *actuator.Actuator, shipCurrent, shipOther int) *SST {
	return &SST{
		actuator:    actuator,
		shipCurrent: shipCurrent,
		shipOther:   shipOther,
	}
}

func (s SST) ShipCurrent() int {
	return s.shipCurrent
}

func (s SST) Busted() bool {
	return s.busted
}

func (s *SST) Run(ctx context.Context) error {
	for i := 0; ; i++ {
		if i%5 == 0 {
			// periodically update stats to track exp
			s.actuator.Send("/")
		} else if s.actuator.Data.Status.Exp < 35*s.actuator.Data.Status.Holds {
			// except when exp is low, then track it more often
			s.actuator.Send("/")
		}

		if i <= 1 {
			err := s.preparePort(ctx)
			if err != nil {
				return err
			}
		} else {
			s.sell(ctx)
		}

		var err error
		s.busted, err = s.steal(ctx)
		if s.busted || err != nil {
			fmt.Printf("SST done after %d steals\n", i)
			return err
		}

		err = s.actuator.Transport(ctx, s.shipOther)
		if err != nil {
			return err
		}
		s.shipCurrent, s.shipOther = s.shipOther, s.shipCurrent
	}
}

func (s *SST) preparePort(ctx context.Context) error {
	s.actuator.QuickStats(ctx)

	if s.actuator.Data.Status.Equ > 0 {
		err := s.sell(ctx)
		if err != nil {
			return err
		}
	}

	// jettison anything else we have
	s.actuator.Sendf("jy")
	return nil
}

func (s *SST) sell(ctx context.Context) error {
	s.actuator.Send("pt")

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.SELLPROMPT):
		// sell all
		s.actuator.Send("\r")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.actuator.Broker.WaitFor(ctx, events.PORTNOTINTERESTED, ""):
		// TODO make sure we're not asked next to buy something else. For
		// example, got a "not interested" response negotiating to buy fuel, but
		// then get a prompt to buy org.
		// try again
		return s.sell(ctx)
	case <-s.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.COMMANDPROMPT):
		return nil
	case <-s.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.BUYPROMPT):
		// don't buy anything
		sector, _ := s.actuator.Data.GetSector(s.actuator.Data.Status.Sector)
		if sector.Port == nil || sector.Port.Report == nil {
			return fmt.Errorf("unexpected nil port report")
		}
		if sector.Port.Report.Fuel.Status == models.SELLING {
			s.actuator.Send("0\r")
		}
		if sector.Port.Report.Org.Status == models.SELLING {
			s.actuator.Send("0\r")
		}
	}

	return nil
}

func (s *SST) steal(ctx context.Context) (bool, error) {
	holds := s.actuator.Data.Status.Holds

	holdsToSteal := min(holds, s.actuator.Data.Status.Exp/30)

	s.actuator.Send("pr\rs3")

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case e := <-s.actuator.Broker.WaitFor(ctx, events.PORTEQUTOSTEAL, ""):
		available := e.DataInt
		// upgrade if necessary, when port regen means a bit of the equ we just
		// sold is now gone
		if available < holds {
			upgrade := int((holds - available) / 10)
			if (holds-available)%10 > 0 {
				upgrade += 1
			}
			s.actuator.Sendf("0\ro3%d\rq", upgrade)
			s.actuator.Send("pr\rs3")
		}
	}
	s.actuator.Sendf("%d\r", holdsToSteal)

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-s.actuator.Broker.WaitFor(ctx, events.STEALRESULT, string(events.CRIMESUCCESS)):
		return false, nil
	// sometimes this one gets obscured by a fig hit, so the next WaitFor
	// ensures we notice either way.
	case <-s.actuator.Broker.WaitFor(ctx, events.STEALRESULT, string(events.CRIMEBUSTED)):
		return true, nil
	case <-s.actuator.Broker.WaitFor(ctx, events.BUSTED, ""):
		return true, nil
	}
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

type pCreate struct {
	actuator *actuator.Actuator
	done     chan struct{}
	classes  map[string]int
}

func NewPCreate(classes map[string]int, actuator *actuator.Actuator) Action {
	return &pCreate{
		actuator: actuator,
		done:     make(chan struct{}),
		classes:  classes,
	}
}

func (p *pCreate) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *pCreate) run(ctx context.Context) {
	defer close(p.done)

	p.actuator.Send("/")
	select {
	case <-p.actuator.Broker.WaitFor(ctx, events.QUICKSTATDISPLAY, ""):
	case <-ctx.Done():
		fmt.Println(ctx.Err())
		return
	}

	for len(p.classes) > 0 {
		if p.actuator.Data.Status.GTorps == 0 || p.actuator.Data.Status.AtmDts == 0 {
			err := p.replenish(ctx)
			if err != nil {
				fmt.Printf("failed to replenish: %s\n", err.Error())
				return
			}
		}

		p.actuator.Send("uy")
		p.actuator.Data.Status.GTorps = p.actuator.Data.Status.GTorps - 1

		select {
		case e := <-p.actuator.Broker.WaitFor(ctx, events.PLANETCREATE, ""):
			count, ok := p.classes[e.ID]
			if !ok {
				// destroy planet
				p.actuator.Send("x\rc")
				err := p.actuator.LandNewest(ctx)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				p.actuator.Send("zdy")
				p.actuator.Data.Status.AtmDts = p.actuator.Data.Status.AtmDts - 1
				continue
			}

			p.actuator.Send("x\rc")

			if count > 1 {
				p.classes[e.ID] = count - 1
			} else {
				delete(p.classes, e.ID)
			}
		case <-ctx.Done():
			fmt.Println(ctx.Err())
			return
		}
	}
}

func (p *pCreate) replenish(ctx context.Context) error {
	sector := p.actuator.Data.Status.Sector
	p.actuator.GoToSD(ctx)
	p.actuator.BuyGTorpsAndDetonators(ctx)
	// request quick stats so the status gets updated with replenished values
	p.actuator.Send("/")
	return p.actuator.Twarp(ctx, sector)
}

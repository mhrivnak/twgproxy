package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

type pStripBulk struct {
	actuator *actuator.Actuator
	done     chan struct{}
	toID     int
}

func NewPStripBulk(toID int, actuator *actuator.Actuator) Action {
	return &pStripBulk{
		actuator: actuator,
		done:     make(chan struct{}),
		toID:     toID,
	}
}

func (p *pStripBulk) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *pStripBulk) run(ctx context.Context) {
	defer close(p.done)

	p.actuator.Send("/")
	select {
	case <-p.actuator.Broker.WaitFor(ctx, events.QUICKSTATDISPLAY, ""):
	case <-ctx.Done():
		fmt.Println(ctx.Err())
		return
	}

	for {
		if p.actuator.Data.Status.GTorps == 0 {
			return
		}

		p.actuator.Send("uy")
		p.actuator.Data.Status.GTorps = p.actuator.Data.Status.GTorps - 1

		select {
		case <-p.actuator.Broker.WaitFor(ctx, events.PLANETCREATE, ""):
			p.actuator.Send("x\rc")
			err := p.actuator.LandNewest(ctx)
			if err != nil {
				fmt.Println(err.Error())
				return
			}

			select {
			case e := <-p.actuator.Broker.WaitFor(ctx, events.PLANETDISPLAY, ""):
				fromID := e.DataInt
				p.actuator.Send("q")
				<-time.After(time.Second)
				err := p.actuator.StripPlanet(ctx, fromID, p.toID)
				if err != nil {
					fmt.Println(ctx.Err())
					return
				}
			case <-ctx.Done():
				fmt.Println(ctx.Err())
				return
			}

			if p.actuator.Data.Status.GTorps == 0 || p.actuator.Data.Status.AtmDts == 0 {
				return
			}
			p.actuator.Send("zdy")
			p.actuator.Data.Status.AtmDts = p.actuator.Data.Status.AtmDts - 1
		case <-ctx.Done():
			fmt.Println(ctx.Err())
			return
		}

		// hopefully helps instability; seems like we overwhelm swath and maybe
		// also this app.
		<-time.After(4 * time.Second)
	}
}

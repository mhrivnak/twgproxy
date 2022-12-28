package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

type ptrade struct {
	actuator *actuator.Actuator
	planetID int
	product  ProductType
	done     chan struct{}
}

func NewPTrade(planetID int, product ProductType, actuator *actuator.Actuator) Action {
	done := make(chan struct{})
	return &ptrade{
		actuator: actuator,
		planetID: planetID,
		product:  product,
		done:     done,
	}
}

func (p *ptrade) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *ptrade) run(ctx context.Context) {
	defer close(p.done)

	// ensure we're at the planet prompt
	p.actuator.Send("\r")
	select {
	case <-ctx.Done():
		return
	case e := <-p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, ""):
		switch e.ID {
		case events.COMMANDPROMPT:
			wait := p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.PLANETPROMPT)
			p.actuator.Land(p.planetID)
			select {
			case <-ctx.Done():
				return
			case <-wait:
			}
		case events.PLANETPROMPT:
		case events.CITADELPROMPT:
			p.actuator.Send("q")
		}
	}

	// make sure we're on the right planet
	wait := p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.PLANETPROMPT)
	p.actuator.Send("\r")
	select {
	case <-ctx.Done():
		return
	case e := <-p.actuator.Broker.WaitFor(ctx, events.PLANETDISPLAY, ""):
		if p.planetID != 0 && p.planetID != e.DataInt {
			p.actuator.Send("q")
			p.actuator.Land(p.planetID)
		} else if p.planetID == 0 {
			p.planetID = e.DataInt
		}
	}

	// wait for prompt before invoking mombot
	<-wait

	p.actuator.MombotSend(ctx, fmt.Sprintf("neg %s\r", ProductCharFromType(p.product)))

	// wait for mombot to finish
	select {
	case <-ctx.Done():
		return
	case <-p.actuator.Broker.WaitFor(ctx, events.MBOTTRADEDONE, ""):
		return
	case <-p.actuator.Broker.WaitFor(ctx, events.MBOTNOTHINGTOSELL, ""):
		return
	}
}

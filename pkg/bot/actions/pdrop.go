package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

type pdrop struct {
	actuator *actuator.Actuator
	done     chan struct{}
}

func NewPDrop(actuator *actuator.Actuator) Action {
	done := make(chan struct{})
	return &pdrop{
		actuator: actuator,
		done:     done,
	}
}

func (p *pdrop) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *pdrop) run(ctx context.Context) {
	defer close(p.done)

	fmt.Println("Waiting to drop planet...")

	select {
	case <-ctx.Done():
		return
	case e := <-p.actuator.Broker.WaitFor(ctx, events.FIGHIT, ""):
		p.actuator.Send(fmt.Sprintf("p%d\ry", e.DataInt))
	}

	fmt.Println("done with fighit command")
}

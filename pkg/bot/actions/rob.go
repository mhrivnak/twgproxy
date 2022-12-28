package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

type rob struct {
	actuator *actuator.Actuator
	done     chan struct{}
}

func NewRob(actuator *actuator.Actuator) Action {
	done := make(chan struct{})
	return &rob{
		actuator: actuator,
		done:     done,
	}
}

func (r *rob) Start(ctx context.Context) <-chan struct{} {
	go r.run(ctx)
	return r.done
}

func (r *rob) run(ctx context.Context) {
	defer close(r.done)

	r.actuator.Send("d/pr\rr")

	select {
	case <-ctx.Done():
		break
	case e := <-r.actuator.Broker.WaitFor(ctx, events.PORTROBCREDS, ""):
		creds := e.DataInt
		if creds < 700000 {
			fmt.Println("not enough creds to rob")
			r.actuator.Send("0\r")
			return
		}
		credsToRob := int(float32(creds) * 1.11)
		maxToRob := 3 * r.actuator.Data.Status.Exp

		if credsToRob > maxToRob {
			credsToRob = maxToRob
		}

		r.actuator.Send(fmt.Sprintf("%d\r", credsToRob))
	}

}

package actions

import (
	"context"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

type robPair struct {
	actuator  *actuator.Actuator
	done      chan struct{}
	otherPort int
}

func NewRobPair(otherPort int, actuator *actuator.Actuator) Action {
	done := make(chan struct{})
	return &robPair{
		actuator:  actuator,
		done:      done,
		otherPort: otherPort,
	}
}

func (r *robPair) Start(ctx context.Context) <-chan struct{} {
	go r.run(ctx)
	return r.done
}

func (r *robPair) run(ctx context.Context) {
	defer close(r.done)
	startPort := r.actuator.Data.Status.Sector

	abortWait := r.actuator.Broker.WaitFor(ctx, events.ROBRESULT, string(events.ROBABORT))
	bustedWait := r.actuator.Broker.WaitFor(ctx, events.ROBRESULT, string(events.ROBBUSTED))

	for i := 0; ; i++ {
		r.actuator.Rob(ctx)

		select {
		case <-abortWait:
			return
		case <-bustedWait:
			return
		case <-r.actuator.Broker.WaitFor(ctx, events.ROBRESULT, string(events.ROBSUCCESS)):
			if i%2 == 0 {
				r.actuator.MoveSafe(ctx, r.otherPort, true)
			} else {
				r.actuator.MoveSafe(ctx, startPort, true)
			}
		case <-ctx.Done():
			return
		}

	}

}

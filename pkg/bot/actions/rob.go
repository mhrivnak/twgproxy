package actions

import (
	"context"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
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

	r.actuator.Rob(ctx)
}

package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
)

type move struct {
	actuator *actuator.Actuator
	dest     int
	done     chan struct{}
}

func NewMove(dest int, actuator *actuator.Actuator) Action {
	done := make(chan struct{})
	return &move{
		actuator: actuator,
		dest:     dest,
		done:     done,
	}
}

func (m *move) Start(ctx context.Context) <-chan struct{} {
	go m.run(ctx)
	return m.done
}

func (m *move) run(ctx context.Context) {
	defer close(m.done)

	err := m.actuator.Move(ctx, m.dest, false)
	if err != nil {
		fmt.Println(err.Error())
	}
}

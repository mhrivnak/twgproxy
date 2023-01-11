package actions

import (
	"context"

	"github.com/mhrivnak/twgproxy/pkg/bot/actions/tools"
	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
)

type pUpgrade struct {
	actuator *actuator.Actuator
	done     chan struct{}
	points   []int
}

func NewPUpgrade(points string, actuator *actuator.Actuator) (Action, error) {
	done := make(chan struct{})
	parsedPoints, err := tools.ParsePoints(points)

	return &pUpgrade{
		actuator: actuator,
		done:     done,
		points:   parsedPoints,
	}, err
}

func (p *pUpgrade) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *pUpgrade) run(ctx context.Context) {
	defer close(p.done)

	p.actuator.RouteWalk(ctx, p.points, func() {
		p.actuator.MassUpgrade(ctx, true)
	})
}

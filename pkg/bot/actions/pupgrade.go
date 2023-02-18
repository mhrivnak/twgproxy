package actions

import (
	"context"

	"github.com/mhrivnak/twgproxy/pkg/bot/actions/tools"
	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
)

func NewPUpgrade(points string, actuator *actuator.Actuator) (Action, error) {
	parsedPoints, err := tools.ParsePoints(points)
	if err != nil {
		return nil, err
	}

	return Wrap(func(ctx context.Context) {
		actuator.RouteWalk(ctx, parsedPoints, func() {
			actuator.MassUpgrade(ctx, true)
		})
	}), nil
}

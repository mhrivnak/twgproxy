package actions

import (
	"context"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
)

func NewPStrip(fromID, toID int, actuator *actuator.Actuator) Action {
	return WrapErr(func(ctx context.Context) error {
		return actuator.StripPlanet(ctx, fromID, toID)
	})
}

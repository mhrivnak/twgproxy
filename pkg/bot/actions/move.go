package actions

import (
	"context"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
)

func NewMove(dest int, opts actuator.MoveOptions, actuator *actuator.Actuator) Action {
	return WrapErr(func(ctx context.Context) error {
		return actuator.Move(ctx, dest, opts, false)
	})
}

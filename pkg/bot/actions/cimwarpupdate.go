package actions

import (
	"context"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
)

func NewCIMWarpUpdate(a *actuator.Actuator) Action {
	return Wrap(func(ctx context.Context) {
		a.CIMSectorUpdate(ctx)
	})
}

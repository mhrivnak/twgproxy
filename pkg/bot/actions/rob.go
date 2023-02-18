package actions

import (
	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
)

func NewRob(actuator *actuator.Actuator) Action {
	return Wrap(actuator.Rob)
}

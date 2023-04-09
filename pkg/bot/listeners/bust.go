package listeners

import (
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

func NewBustHandler(a *actuator.Actuator) func(*events.Event) {
	return func(e *events.Event) {
		sectorID := e.DataInt
		if sectorID == 0 {
			fmt.Println("ERROR: got bust event without sector specified")
			return
		}

		go a.Data.Persist.SectorCache.UpdateBust(sectorID)
	}

}

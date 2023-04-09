package listeners

import (
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models/persist"
)

func NewSectorHandler(a *actuator.Actuator) func(*events.Event) {
	return func(e *events.Event) {
		sectorID := e.DataInt
		if sectorID == 0 {
			fmt.Println("ERROR: got sector event without sector specified")
			return
		}

		s, ok := a.Data.GetSector(sectorID)
		if !ok {
			return
		}
		if s.Warps != nil && len(s.Warps) > 0 {
			go a.Data.Persist.WarpCache.AddIfNeeded(s.ID, s.Warps)
		}

		sector := persist.Sector{
			ID: uint(sectorID),
		}

		if s.Port != nil && len(s.Port.Type) == 3 {
			switch s.Port.Type[0] {
			case 'B':
				sector.Fuel = persist.BUYING
			case 'S':
				sector.Fuel = persist.SELLING
			default:
				fmt.Printf("got unexpected port product type char %s\n", string(s.Port.Type[0]))
				return
			}

			switch s.Port.Type[1] {
			case 'B':
				sector.Org = persist.BUYING
			case 'S':
				sector.Org = persist.SELLING
			default:
				fmt.Printf("got unexpected port product type char %s\n", string(s.Port.Type[1]))
				return
			}

			switch s.Port.Type[2] {
			case 'B':
				sector.Equ = persist.BUYING
			case 'S':
				sector.Equ = persist.SELLING
			default:
				fmt.Printf("got unexpected port product type char %s\n", string(s.Port.Type[2]))
				return
			}
		}

		go a.Data.Persist.SectorCache.UpdateIfNeeded(&sector)
	}

}

package actions

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

type surround struct {
	actuator *actuator.Actuator
	done     chan struct{}
	figs     int
}

func NewSurround(figs int, actuator *actuator.Actuator) Action {
	return &surround{
		actuator: actuator,
		done:     make(chan struct{}),
		figs:     figs,
	}
}

func (s *surround) Start(ctx context.Context) <-chan struct{} {
	go s.run(ctx)
	return s.done
}

func (s *surround) run(ctx context.Context) {
	defer close(s.done)
	start := time.Now()

	// scan to make sure it's safe and get warp counts
	s.actuator.Send("shsd")

	select {
	case <-s.actuator.Broker.WaitFor(ctx, events.DENSITYDISPLAY, ""):
	case <-ctx.Done():
		fmt.Println(ctx.Err())
		return
	}

	sector, ok := s.actuator.Data.GetSector(s.actuator.Data.Status.Sector)
	if !ok {
		fmt.Println("current sector not in cache")
		return
	}

	needFigs := []models.Sector{}

	for _, n := range sector.Warps {
		neighbor, ok := s.actuator.Data.GetSector(n)
		if !ok {
			fmt.Printf("neighbor %d not in cache\n", n)
			return
		}
		if !neighbor.IsSafe() {
			fmt.Printf("neighbor %d not safe\n", n)
			return
		}
		if neighbor.Figs < s.figs {
			needFigs = append(needFigs, *neighbor)
		}
	}

	// sort so we visit the sectors with the most warps first. That gives the
	// opponent being surrounded fewer options for where to run.
	sort.Slice(needFigs, func(i, j int) bool {
		return needFigs[i].WarpCount > needFigs[j].WarpCount
	})

	for i, n := range needFigs {
		current, ok := s.actuator.Data.GetSector(s.actuator.Data.Status.Sector)
		if !ok {
			fmt.Println("sector not in cache")
			return
		}
		directReturn := false

		for _, warp := range current.Warps {
			if warp == sector.ID {
				directReturn = true
				break
			}
		}

		switch {
		case i == 0:
			s.actuator.Send(fmt.Sprintf("%d\r", n.ID))
		case directReturn:
			s.actuator.Express(n.ID)
		default:
			s.actuator.Move(ctx, n.ID, false)
		}

		select {
		case <-s.actuator.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(n.ID)):
		case <-s.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.STOPINSECTORPROMPT):
			s.actuator.Send("\r")
		case <-ctx.Done():
			fmt.Println(ctx.Err())
			return
		}
		s.actuator.Send(fmt.Sprintf("f%d\rcd", s.figs))
	}
	s.actuator.Send(fmt.Sprintf("%d\r", sector.ID))

	fmt.Printf("Surround took %v\n", time.Since(start))
}

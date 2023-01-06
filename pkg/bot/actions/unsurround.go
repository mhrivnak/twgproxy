package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

type unsurround struct {
	actuator *actuator.Actuator
	done     chan struct{}
}

func NewUnsurround(actuator *actuator.Actuator) Action {
	return &unsurround{
		actuator: actuator,
		done:     make(chan struct{}),
	}
}

func (s *unsurround) Start(ctx context.Context) <-chan struct{} {
	go s.run(ctx)
	return s.done
}

func (s *unsurround) run(ctx context.Context) {
	defer close(s.done)
	start := time.Now()

	// holo-scan to make sure it's safe
	s.actuator.Send("sh")

	select {
	case <-s.actuator.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(s.actuator.Data.Status.Sector)):
	case <-ctx.Done():
		fmt.Println(ctx.Err())
		return
	}

	sector, ok := s.actuator.Data.Sectors[s.actuator.Data.Status.Sector]
	if !ok {
		fmt.Println("current sector not in cache")
		return
	}
	fmt.Printf("START: %d\n", sector.ID)

	for _, n := range sector.Warps {
		neighbor, ok := s.actuator.Data.Sectors[n]
		if !ok {
			fmt.Printf("neighbor %d not in cache\n", n)
			return
		}
		if !neighbor.IsSafe() {
			fmt.Printf("neighbor %d not safe\n", n)
			return
		}
	}

	for _, n := range sector.Warps {
		s.actuator.Move(ctx, n, true)
		s.actuator.Send("f0\r")
	}
	s.actuator.Move(ctx, sector.ID, false)

	fmt.Printf("Unsurround took %v\n", time.Since(start))
}

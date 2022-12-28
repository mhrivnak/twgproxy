package actions

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

type move struct {
	actuator *actuator.Actuator
	dest     int
	done     chan struct{}
}

func NewMove(dest int, actuator *actuator.Actuator) Action {
	done := make(chan struct{})
	return &move{
		actuator: actuator,
		dest:     dest,
		done:     done,
	}
}

func (m *move) Start(ctx context.Context) <-chan struct{} {
	go m.run(ctx)
	return m.done
}

func (m *move) run(ctx context.Context) {
	defer close(m.done)
	sectors := []int{}

	// send commands
	m.actuator.Send(fmt.Sprintf("cf\r%d\rq", m.dest))

	// wait for events
	select {
	case e := <-m.actuator.Broker.WaitFor(ctx, events.ROUTEDISPLAY, ""):
		fmt.Printf("got route: %s\n", e.Data)
		var err error
		sectors, err = parseSectors(e.Data)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

	case <-ctx.Done():
		break
	}

	// ignore the first sector, which is the one we're in
	for _, sector := range sectors[1:] {
		m.actuator.Send("sh")

		select {
		case <-m.actuator.Broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sector)):
			sInfo, ok := m.actuator.Data.Sectors[sector]
			if !ok {
				fmt.Printf("failed to get cached info on sector %d\n", sector)
				return
			}
			if !sInfo.IsSafe() {
				fmt.Println("unsafe sector ahead")
				return
			}
			m.actuator.Send(fmt.Sprintf("%d\r", sector))
		case <-ctx.Done():
			return
		}
	}
}

func parseSectors(route string) ([]int, error) {
	parts := strings.Split(route, " > ")
	sectors := make([]int, len(parts))
	for i := range parts {
		sector, err := strconv.Atoi(parts[i])
		if err != nil {
			return nil, err
		}
		sectors[i] = sector
	}
	return sectors, nil
}

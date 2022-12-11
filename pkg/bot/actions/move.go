package actions

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

type move struct {
	dest   int
	broker *events.Broker
	data   *models.Data
	done   chan struct{}
	send   func(string) error
}

func NewMove(dest int, broker *events.Broker, data *models.Data, send func(string) error) Action {
	done := make(chan struct{})
	return &move{
		broker: broker,
		data:   data,
		dest:   dest,
		done:   done,
		send:   send,
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
	m.send(fmt.Sprintf("cf\r%d\rq", m.dest))

	// wait for events
	select {
	case e := <-m.broker.WaitFor(ctx, events.ROUTEDISPLAY, ""):
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
		m.send("sh")

		select {
		case <-m.broker.WaitFor(ctx, events.SECTORDISPLAY, fmt.Sprint(sector)):
			sInfo, ok := m.data.Sectors[sector]
			if !ok {
				fmt.Printf("failed to get cached info on sector %d\n", sector)
				return
			}
			if !sInfo.IsSafe() {
				fmt.Println("unsafe sector ahead")
				return
			}
			m.send(fmt.Sprintf("%d\r", sector))
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

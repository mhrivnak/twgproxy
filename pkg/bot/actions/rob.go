package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

type rob struct {
	broker *events.Broker
	data   *models.Data
	done   chan struct{}
	send   func(string) error
}

func NewRob(broker *events.Broker, data *models.Data, send func(string) error) Action {
	done := make(chan struct{})
	return &rob{
		broker: broker,
		data:   data,
		done:   done,
		send:   send,
	}
}

func (r *rob) Start(ctx context.Context) <-chan struct{} {
	go r.run(ctx)
	return r.done
}

func (r *rob) run(ctx context.Context) {
	defer close(r.done)

	r.send("d/pr\rr")

	select {
	case <-ctx.Done():
		break
	case e := <-r.broker.WaitFor(ctx, events.PORTROBCREDS, ""):
		creds := e.DataInt
		if creds < 700000 {
			fmt.Println("not enough creds to rob")
			r.send("0\r")
			return
		}
		credsToRob := int(float32(creds) * 1.11)
		maxToRob := 3 * r.data.Status.Exp

		if credsToRob > maxToRob {
			credsToRob = maxToRob
		}

		r.send(fmt.Sprintf("%d\r", credsToRob))
	}

}

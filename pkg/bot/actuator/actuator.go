package actuator

import (
	"context"
	"fmt"
	"io"

	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

func New(broker *events.Broker, data *models.Data, writer io.Writer) *Actuator {
	return &Actuator{
		Broker:        broker,
		Data:          data,
		commandWriter: writer,
	}
}

type Actuator struct {
	Broker        *events.Broker
	Data          *models.Data
	commandWriter io.Writer
}

func (a *Actuator) Send(command string) error {
	_, err := a.commandWriter.Write([]byte(command))
	return err
}

func (a *Actuator) Land(planetID int) {
	a.Send(fmt.Sprintf("l%d\r", planetID))
}

func (a *Actuator) MombotSend(ctx context.Context, command string) {
	a.Send(">")
	select {
	case <-ctx.Done():
		return
	case <-a.Broker.WaitFor(ctx, events.PROMPTDISPLAY, events.MOMBOTPROMPT):
		break
	}
	a.Send(command)
}

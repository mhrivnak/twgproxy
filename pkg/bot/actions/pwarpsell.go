package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actions/tools"
	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
	"github.com/mhrivnak/twgproxy/pkg/models"
)

type pWarpSell struct {
	actuator *actuator.Actuator
	points   []int
	done     chan struct{}
}

func NewPWarpSell(points string, actuator *actuator.Actuator) (Action, error) {
	done := make(chan struct{})

	parsedPoints, err := tools.ParsePoints(points)

	return &pWarpSell{
		actuator: actuator,
		done:     done,
		points:   parsedPoints,
	}, err
}

func (p *pWarpSell) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *pWarpSell) run(ctx context.Context) {
	defer close(p.done)

	var org int
	var equ int

	wait := p.actuator.Broker.WaitFor(ctx, events.PLANETDISPLAY, "")
	p.actuator.Send("d")
	select {
	case <-ctx.Done():
		fmt.Println(ctx.Err())
		return
	case e := <-wait:
		planetID := e.DataInt
		p.actuator.Data.PlanetLock.Lock()
		planet, ok := p.actuator.Data.Planets[planetID]
		p.actuator.Data.PlanetLock.Unlock()
		if !ok {
			fmt.Println("planet not in cache")
			return
		}
		org = planet.Org
		equ = planet.Equ
	}

	for _, point := range p.points {
		if org < 32760 || equ < 32760 {
			fmt.Println("done; planet is low on product")
			return
		}

		p.actuator.Send(fmt.Sprintf("cp%d\ryq", point))

		// wait for the warp to complete
		select {
		case <-ctx.Done():
			return
		case <-p.actuator.Broker.WaitFor(ctx, events.PLANETWARPCOMPLETE, ""):
		}

		p.actuator.MombotPlanetSell(ctx, models.ORG)
		org -= 32760
		p.actuator.MombotPlanetSell(ctx, models.EQU)
		equ -= 32760
	}
}

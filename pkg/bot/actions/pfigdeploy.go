package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
	"github.com/mhrivnak/twgproxy/pkg/bot/events"
)

type pFigDeploy struct {
	actuator *actuator.Actuator
	done     chan struct{}
	figs     int
}

func NewPFigDeploy(figs int, actuator *actuator.Actuator) Action {
	return &pFigDeploy{
		actuator: actuator,
		done:     make(chan struct{}),
		figs:     figs,
	}
}

func (p *pFigDeploy) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *pFigDeploy) run(ctx context.Context) {
	defer close(p.done)
	var planetID int

	p.actuator.Send("\r")
loop:
	for {
		select {
		case e := <-p.actuator.Broker.WaitFor(ctx, events.PLANETDISPLAY, ""):
			planetID = e.DataInt
			break loop
		case e := <-p.actuator.Broker.WaitFor(ctx, events.PROMPTDISPLAY, ""):
			if e.ID != events.PLANETPROMPT {
				fmt.Println("must be at planet prompt to run fig deploy")
				return
			}
		case <-ctx.Done():
			fmt.Println(ctx.Err())
			return
		}
	}

	planet, ok := p.actuator.Data.Planets[planetID]
	if !ok {
		fmt.Printf("Planet ID %d not in cache\n", planetID)
		return
	}

	// load up with figs and get quick stats so we know the capacity
	p.actuator.Send("m\r\r\r/")

	for {
		var available int
		p.actuator.Send("qf")
		select {
		case e := <-p.actuator.Broker.WaitFor(ctx, events.FIGDEPLOY, ""):
			available = e.DataInt
		case <-ctx.Done():
			fmt.Println(ctx.Err())
			return
		}

		if available-p.actuator.Data.Status.Figs > p.figs {
			// this is an error. This command isn't designed to un-deploy figs
			p.actuator.Send(fmt.Sprintf("%d\rcd", available-p.actuator.Data.Status.Figs))
			p.actuator.Land(planet.ID)
			fmt.Println("Error: sector has more figs than desired.")
			return
		}
		p.actuator.Send(fmt.Sprintf("%d\rcd", min(available, p.figs)))

		p.actuator.Land(planet.ID)

		if available >= p.figs {
			// load the ship up with figs
			p.actuator.Send("m\r\r\r")
			return
		}
	}
}

func min(nums ...int) int {
	min := nums[0]

	for _, i := range nums {
		if min > i {
			min = i
		}
	}

	return min
}

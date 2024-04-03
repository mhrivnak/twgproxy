package actions

import (
	"context"
	"fmt"

	"github.com/mhrivnak/twgproxy/pkg/bot/actuator"
)

// explore moves to the starting sector then each sector number sequentially
// while holo-scanning.
type explore struct {
	actuator    *actuator.Actuator
	done        chan struct{}
	startSector int
}

func NewExplore(startSector int, actuator *actuator.Actuator) Action {
	done := make(chan struct{})
	return &explore{
		actuator:    actuator,
		done:        done,
		startSector: startSector,
	}
}

func (p *explore) Start(ctx context.Context) <-chan struct{} {
	go p.run(ctx)
	return p.done
}

func (p *explore) run(ctx context.Context) {
	defer close(p.done)

	for i := p.startSector; i <= 30000; i++ {
		fmt.Printf("exploring to sector %d\n", i)
		opts := actuator.MoveOptions{
			DropFigs:        1,
			MinFigs:         49000,
			EnemyFigsMax:    50000,
			EnemyMinesMax:   50,
			RefurbAndReturn: true,
			BuyFuel:         true,
		}
		err := p.actuator.Move(ctx, i, opts, false)
		if err != nil {
			fmt.Printf("error: %s\n", err.Error())
			return
		}
	}

	fmt.Println("done exploring")
}

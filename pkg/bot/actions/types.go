package actions

import (
	"context"
	"fmt"
)

type Action interface {
	Start(context.Context) <-chan struct{}
}

func Wrap(runme func(context.Context)) Action {
	return &wrapper{runme}
}

func WrapErr(runme func(context.Context) error) Action {
	return &wrapper{func(ctx context.Context) {
		err := runme(ctx)
		if err != nil {
			fmt.Println(err.Error())
		}
	}}
}

type wrapper struct {
	runme func(context.Context)
}

func (w *wrapper) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go w.run(ctx, done)
	return done
}

func (w *wrapper) run(ctx context.Context, done chan struct{}) {
	w.runme(ctx)
	close(done)
}

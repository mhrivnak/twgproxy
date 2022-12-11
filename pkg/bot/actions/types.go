package actions

import "context"

type Action interface {
	Start(context.Context) <-chan struct{}
}

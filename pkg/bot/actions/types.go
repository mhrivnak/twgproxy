package actions

import "context"

const (
	FUEL ProductType = "fuel ore"
	ORG  ProductType = "organics"
	EQU  ProductType = "equipment"
)

type ProductType string

type Action interface {
	Start(context.Context) <-chan struct{}
}

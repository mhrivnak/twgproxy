package actions

import (
	"context"
	"fmt"
)

const (
	FUEL ProductType = "fuel ore"
	ORG  ProductType = "organics"
	EQU  ProductType = "equipment"
	NONE ProductType = "none"
)

type ProductType string

type Action interface {
	Start(context.Context) <-chan struct{}
}

func ProductCharFromType(pType ProductType) string {
	switch pType {
	case FUEL:
		return "f"
	case ORG:
		return "o"
	case EQU:
		return "e"
	}
	return ""
}

func ProductTypeFromChar(c string) (ProductType, error) {
	switch c {
	case "f":
		return FUEL, nil
	case "o":
		return ORG, nil
	case "e":
		return EQU, nil
	}
	return NONE, fmt.Errorf("invalid product type %s", c)
}

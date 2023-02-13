package models

import "fmt"

const (
	FUEL ProductType = "f"
	ORG  ProductType = "o"
	EQU  ProductType = "e"
	NONE ProductType = "none"
)

type ProductType string

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

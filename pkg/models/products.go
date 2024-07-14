package models

import "fmt"

const (
	PRODUCTFUEL ProductType = "f"
	PRODUCTORG  ProductType = "o"
	PRODUCTEQU  ProductType = "e"
	PRODUCTNONE ProductType = ""
)

type ProductType string

func ProductTypeFromChar(c string) (ProductType, error) {
	switch c {
	case "f":
		return PRODUCTFUEL, nil
	case "o":
		return PRODUCTORG, nil
	case "e":
		return PRODUCTEQU, nil
	}
	return PRODUCTNONE, fmt.Errorf("invalid product type %s", c)
}

func (t ProductType) Num() int {
	switch t {
	case PRODUCTFUEL:
		return 1
	case PRODUCTORG:
		return 2
	case PRODUCTEQU:
		return 3
	}
	return -1
}

func ProductTypeFromNum(n int) (ProductType, error) {
	switch n {
	case 1:
		return PRODUCTFUEL, nil
	case 2:
		return PRODUCTORG, nil
	case 3:
		return PRODUCTEQU, nil
	}
	return PRODUCTNONE, fmt.Errorf("invalid product type %d", n)
}

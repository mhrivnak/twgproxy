package models

type Planet struct {
	ID       int
	Name     string
	Sector   int
	Class    string
	Level    int
	Ore      int
	Org      int
	Equ      int
	OreMax   int
	OrgMax   int
	EquMax   int
	FuelCols int
	OrgCols  int
	EquCols  int
	Figs     int
	Summary  *PlanetCorpSummary
}

// Numbers from the corporate planet list, which aren't precise
type PlanetCorpSummary struct {
	Ore  int
	Org  int
	Equ  int
	Figs int
}

func (p *Planet) ProductQuantity(product ProductType) int {
	switch product {
	case FUEL:
		return p.Ore
	case ORG:
		return p.Org
	case EQU:
		return p.Equ
	}
	return -1
}

func (p *Planet) ProductMax(product ProductType) int {
	switch product {
	case FUEL:
		return p.OreMax
	case ORG:
		return p.OrgMax
	case EQU:
		return p.EquMax
	}
	return -1
}

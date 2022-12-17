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

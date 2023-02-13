package models

type Sector struct {
	ID            int
	Figs          int
	FigsType      string
	FigsFriendly  bool
	Mines         int
	MinesFriendly bool
	Port          *Port
	Warps         []int
	WarpCount     int
	Density       int
}

type Port struct {
	Type   string
	Creds  int
	Report *PortReport
}

type PortReport struct {
	Fuel PortItem
	Org  PortItem
	Equ  PortItem
}

type PortItem struct {
	Status  PortItemStatus
	Trading int
	Percent int
}

type PortItemStatus string

const BUYING PortItemStatus = "buying"
const SELLING PortItemStatus = "selling"

func (s *Sector) IsSafe() bool {
	switch {
	case s.Figs > 0 && !s.FigsFriendly:
		return false
	case s.Mines > 0 && !s.MinesFriendly:
		return false
	}
	return true
}

func (s *Sector) IsAdjacent(other int) bool {
	for _, warp := range s.Warps {
		if other == warp {
			return true
		}
	}
	return false
}

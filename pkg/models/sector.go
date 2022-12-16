package models

type Sector struct {
	ID            int
	Figs          int
	FigsType      string
	FigsFriendly  bool
	Mines         int
	MinesFriendly bool
	Port          *Port
}

type Port struct {
	Type  string
	Creds int
}

func (s *Sector) IsSafe() bool {
	switch {
	case s.Figs > 0 && !s.FigsFriendly:
		return false
	case s.Mines > 0 && !s.MinesFriendly:
		return false
	}
	return true
}

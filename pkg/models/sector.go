package models

import (
	"fmt"
	"strings"
)

type Sector struct {
	ID            int
	Figs          int
	FigsFriendly  bool
	FigType       FigType
	Mines         int
	MinesFriendly bool
	Port          *Port
	Warps         []int
	WarpCount     int
	Density       int
	Traders       []Trader
	IsFedSpace    bool
}

type FigType string

const (
	FigTypeToll      FigType = "Toll"
	FigTypeOffensive FigType = "Offensive"
	FigTypeDefensive FigType = "Defensive"
)

func FigTypeFromString(figtype string) (FigType, error) {
	switch figtype {
	case string(FigTypeToll):
		return FigTypeToll, nil
	case string(FigTypeOffensive):
		return FigTypeOffensive, nil
	case string(FigTypeDefensive):
		return FigTypeDefensive, nil
	}
	return "", fmt.Errorf("unknown fig type: %s", figtype)
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

type Trader struct {
	Name     string
	ShipType ShipType
	Figs     int
	Type     TraderType
}

type TraderType string

const (
	TraderTypeNormal   TraderType = "Normal"
	TraderTypeAlien    TraderType = "Alien"
	TraderTypeGrey     TraderType = "Grey"
	TraderTypeFerrengi TraderType = "Ferrengi"
)

type ShipType string

const (
	ShipTypeDreadnought    ShipType = "Ferrengi Dreadnought"
	ShipTypeAssaultTrader  ShipType = "Ferrengi Assault Trader"
	ShipTypeBattleCruiser  ShipType = "Ferrengi Battle Cruiser"
	ShipTypeBattleShip     ShipType = "BattleShip"
	ShipTypeMissileFrigate ShipType = "Missile Frigate"
	ShipTypeUnkown         ShipType = "Unknown"
)

var KnownTraderTitles []string = []string{
	// Evil
	"Nuisance 1st Class",
	"Nuisance 2nd Class",
	"Nuisance 3rd Class",
	"Menace 1st Class",
	"Menace 2nd Class",
	"Menace 3rd Class",
	"Smuggler 1st Class",
	"Smuggler 2nd Class",
	"Smuggler 3rd Class",
	"Smuggler Savant",
	"Robber",
	"Terrorist",
	"Pirate",
	"Infamous Pirate",
	"Notorious Pirate",
	"Dread Pirate",
	"Galactic Scourge",
	"Enemy of the State",
	"Enemy of the People",
	"Enemy of Humankind",
	"Heinous Overlord",
	// Good
	"Private",
	"Private 1st Class",
	"Lance Corporal",
	"Corporal",
	"Sergeant",
	"Staff Sergeant",
	"Gunnery Sergeant",
	"1st Sergeant",
	"Sergeant Major",
	"Warrant Officer",
	"Chief Warrant Officer",
	"Ensign",
	"Lieutenant J.G.",
	"Lieutenant",
	"Lieutenant Commander",
	"Commander",
	"Captain",
	"Commodore",
	"Rear Admiral",
	"Vice Admiral",
	"Admiral",
	"Fleet Admiral",
	// Ferrengi
	"Trader",
	"Capitalist",
	"Entrepreneur",
	"Merchant Apprentice",
	"Merchant",
	"Grand Merchant",
	"Executive Merchant",
}

func StripTitleFromName(name string) string {
	for _, title := range KnownTraderTitles {
		if strings.HasPrefix(name, title) {
			return strings.TrimSpace(strings.TrimPrefix(name, title))
		}
	}
	return name
}

func ShipTypeFromString(name string) ShipType {
	for _, st := range []ShipType{
		ShipTypeAssaultTrader,
		ShipTypeBattleCruiser,
		ShipTypeBattleShip,
		ShipTypeDreadnought,
		ShipTypeMissileFrigate,
	} {
		if strings.HasSuffix(name, string(st)) {
			return st
		}
	}
	return ShipTypeUnkown
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

func (s *Sector) IsAdjacent(other int) bool {
	for _, warp := range s.Warps {
		if other == warp {
			return true
		}
	}
	return false
}

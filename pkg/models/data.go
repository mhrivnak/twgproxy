package models

import "sync"

type LRSType string

const (
	LRSHOLO    LRSType = "holo scanner"
	LRSDENSITY LRSType = "density scanner"
	LRSNONE    LRSType = "no scanner"
)

type Data struct {
	Planets    map[int]*Planet
	Sectors    map[int]*Sector
	Status     Status
	PlanetLock sync.Mutex
	SectorLock sync.Mutex
}

func NewData() *Data {
	return &Data{
		Planets: map[int]*Planet{},
		Sectors: map[int]*Sector{},
	}
}

type Status struct {
	Creds   int
	Exp     int
	Figs    int
	Holds   int
	Sector  int
	Shields int
	GTorps  int
	AtmDts  int
	LRS     LRSType
}

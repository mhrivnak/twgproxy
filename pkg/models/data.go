package models

import (
	"sync"

	"gorm.io/gorm"

	"github.com/mhrivnak/twgproxy/pkg/models/persist"
)

type LRSType string

const (
	LRSHOLO    LRSType = "holo scanner"
	LRSDENSITY LRSType = "density scanner"
	LRSNONE    LRSType = "no scanner"
)

type TWarpType string

const (
	TWarpTypeNone TWarpType = "none"
	TWarpType1    TWarpType = "type 1"
	TWarpType2    TWarpType = "type 2"
)

type Data struct {
	Planets    map[int]*Planet
	Sectors    map[int]*Sector
	Settings   Settings
	Status     Status
	PlanetLock sync.Mutex
	SectorLock sync.Mutex

	Persist Persist
}

// GetSector returns a pointer to the requested Sector or nil, and a bool that
// indicates if the Sector was found.
func (d *Data) GetSector(sector int) (*Sector, bool) {
	d.SectorLock.Lock()
	defer d.SectorLock.Unlock()

	s, ok := d.Sectors[sector]
	return s, ok
}

func NewData(db *gorm.DB) *Data {
	return &Data{
		Planets: map[int]*Planet{},
		Sectors: map[int]*Sector{},
		Persist: Persist{
			SectorCache: persist.NewSectorCache(db),
			WarpCache:   persist.NewWarpCache(db),
		},
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
	TWarp   TWarpType
}

type Persist struct {
	SectorCache *persist.SectorCache
	WarpCache   *persist.WarpCache
}

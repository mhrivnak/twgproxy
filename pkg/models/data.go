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
	Planets        map[int]*Planet
	Sectors        map[int]*Sector
	Ships          map[int]*Ship
	PortReports    map[int]*PortReport
	Settings       Settings
	Status         Status
	PlanetLock     sync.Mutex
	SectorLock     sync.Mutex
	ShipLock       sync.Mutex
	PortReportLock sync.Mutex

	Persist Persist
}

func (d *Data) GetPortReport(sector int) (*PortReport, bool) {
	d.PortReportLock.Lock()
	defer d.PortReportLock.Unlock()

	s, ok := d.PortReports[sector]
	return s, ok
}

// GetSector returns a pointer to the requested Sector or nil, and a bool that
// indicates if the Sector was found.
func (d *Data) GetSector(sector int) (*Sector, bool) {
	d.SectorLock.Lock()
	defer d.SectorLock.Unlock()

	s, ok := d.Sectors[sector]
	return s, ok
}

func (d *Data) GetShip(id int) (*Ship, bool) {
	d.ShipLock.Lock()
	defer d.ShipLock.Unlock()

	s, ok := d.Ships[id]
	return s, ok
}

func (d *Data) GetPlanet(id int) (*Planet, bool) {
	d.PlanetLock.Lock()
	defer d.PlanetLock.Unlock()

	s, ok := d.Planets[id]
	return s, ok
}

func (d *Data) PutShip(s *Ship) {
	d.ShipLock.Lock()
	defer d.ShipLock.Unlock()

	d.Ships[s.ID] = s
}

func NewData(db *gorm.DB) *Data {
	return &Data{
		Planets:     map[int]*Planet{},
		Sectors:     map[int]*Sector{},
		Ships:       map[int]*Ship{},
		PortReports: map[int]*PortReport{},
		Persist: Persist{
			SectorCache: persist.NewSectorCache(db),
			WarpCache:   persist.NewWarpCache(db),
		},
	}
}

type Status struct {
	Creds    int
	Exp      int
	Figs     int
	Holds    int
	Fuel     int
	Org      int
	Equ      int
	Sector   int
	Ship     int
	Shields  int
	GTorps   int
	AtmDts   int
	LRS      LRSType
	TWarp    TWarpType
	StarDock int
}

type Persist struct {
	SectorCache *persist.SectorCache
	WarpCache   *persist.WarpCache
}

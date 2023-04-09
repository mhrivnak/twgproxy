package persist

import (
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	BUYING  = "buying"
	SELLING = "selling"
)

type Sector struct {
	gorm.Model
	ID uint

	Busted *time.Time

	Fuel string
	Org  string
	Equ  string
}

func (s *Sector) IsObsolete(candidate *Sector) bool {
	switch {
	case s.Fuel != candidate.Fuel:
		return true
	case s.Org != candidate.Org:
		return true
	case s.Equ != candidate.Equ:
		return true
	}
	return false
}

type SectorCache struct {
	sync.Mutex

	Sectors map[int]*Sector
	db      *gorm.DB
}

func NewSectorCache(db *gorm.DB) *SectorCache {
	sc := SectorCache{
		db:      db,
		Sectors: make(map[int]*Sector),
	}
	var sectors []Sector
	result := db.Find(&sectors)
	if result.Error != nil {
		panic(result.Error)
	}

	for i := range sectors {
		s := &sectors[i]
		sc.Sectors[int(s.ID)] = s
	}

	return &sc
}

func (c *SectorCache) Get(id int) (*Sector, bool) {
	c.Lock()
	defer c.Unlock()
	s, ok := c.Sectors[id]
	return s, ok
}

func (c *SectorCache) UpdateIfNeeded(sector *Sector) {
	c.Lock()
	defer c.Unlock()

	found, ok := c.Sectors[int(sector.ID)]

	if !ok || found.IsObsolete(sector) {
		// preserve bust records
		if found != nil && found.Busted != nil {
			sector.Busted = found.Busted
		}

		result := c.db.Save(sector)
		if result.Error != nil {
			fmt.Printf("failed to save sector: %s\n", result.Error.Error())
			return
		}
		c.Sectors[int(sector.ID)] = sector
	}
}

func (c *SectorCache) UpdateBust(sectorID int) {
	c.Lock()
	defer c.Unlock()

	sector, ok := c.Sectors[sectorID]
	if !ok {
		fmt.Println("ERROR: busted sector not found in cache")
		return
	}

	now := time.Now()
	sector.Busted = &now
	result := c.db.Save(sector)
	if result.Error != nil {
		fmt.Printf("failed to save sector: %s\n", result.Error.Error())
		return
	}
	c.Sectors[int(sector.ID)] = sector
}

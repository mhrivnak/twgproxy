package persist

import (
	"sync"

	"gorm.io/gorm"
)

type Warp struct {
	gorm.Model
	ID uint

	From uint `gorm:"index"`
	To   uint
}

type WarpCache struct {
	sync.Mutex

	Warps map[int][]int
	db    *gorm.DB
}

func NewWarpCache(db *gorm.DB) *WarpCache {
	wc := WarpCache{
		db:    db,
		Warps: map[int][]int{},
	}

	var warps []Warp
	result := db.Find(&warps)
	if result.Error != nil {
		panic(result.Error)
	}

	for i := range warps {
		from := int(warps[i].From)
		to := int(warps[i].To)

		wc.add(from, to)
	}

	return &wc
}

func (c *WarpCache) Get(from int) ([]int, bool) {
	c.Lock()
	defer c.Unlock()
	warps, ok := c.Warps[from]
	return warps, ok
}

func (c *WarpCache) Exists(from, to int) bool {
	warps, ok := c.Get(from)
	if !ok {
		return false
	}
	for _, sectorID := range warps {
		if to == sectorID {
			return true
		}
	}
	return false
}

func (c *WarpCache) add(from, to int) {
	_, ok := c.Warps[from]
	if !ok {
		c.Warps[from] = []int{}
	}

	c.Warps[from] = append(c.Warps[from], to)
}

func (c *WarpCache) AddIfNeeded(from int, destinations []int) {
	c.Lock()
	defer c.Unlock()
	_, ok := c.Warps[from]
	if ok {
		return
	}

	for _, to := range destinations {
		c.add(from, to)
		warp := Warp{
			From: uint(from),
			To:   uint(to),
		}
		c.db.Save(&warp)
	}
}

// TrimExplored takes a slice of sectors and returns the ones for which we don't
// have warp data, suggesting they are unexplored.
func (c *WarpCache) TrimExplored(sectors []int) []int {
	ret := []int{}

	c.Lock()
	defer c.Unlock()
	for _, s := range sectors {
		_, ok := c.Warps[s]
		if !ok {
			ret = append(ret, s)
		}
	}
	return ret
}

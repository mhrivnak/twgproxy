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

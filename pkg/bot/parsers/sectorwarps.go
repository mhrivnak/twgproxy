package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/models"
)

func NewSectorWarpsParser(data *models.Data) Parser {
	return &ParseSectorWarps{
		data: data,
	}
}

type ParseSectorWarps struct {
	done bool
	data *models.Data
}

var warpQueryInfo *regexp.Regexp = regexp.MustCompile(`Sector ([0-9]+) has warps to sector\(s\) :  ([0-9 -]+)`)

func (p *ParseSectorWarps) Parse(line string) error {
	if !strings.Contains(line, "has warps to sector(s) :") {
		return nil
	}

	p.done = true

	parts := warpQueryInfo.FindStringSubmatch(line)
	if len(parts) != 3 {
		return fmt.Errorf("failed to parse sector warp query results")
	}

	from, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}

	to := []int{}
	sectors := strings.Split(parts[2], " - ")
	for _, sector := range sectors {
		warp, err := strconv.Atoi(strings.TrimSpace(sector))
		if err != nil {
			fmt.Printf("failed to parse warp %s: %s", sector, err.Error())
			return err
		}
		to = append(to, warp)
	}

	fmt.Printf("%d: %v\n", from, to)

	p.data.Persist.WarpCache.AddIfNeeded(from, to)

	// add warps to the in-memory data store
	s, ok := p.data.GetSector(from)
	if ok {
		s.Warps = to
	}

	return nil
}

func (p *ParseSectorWarps) Done() bool {
	return p.done
}

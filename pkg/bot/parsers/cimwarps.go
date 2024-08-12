package parsers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mhrivnak/twgproxy/pkg/models/persist"
)

func NewCIMWarpsParser(warpCache *persist.WarpCache) Parser {
	return &parseCIMWarps{warpCache: warpCache}
}

type parseCIMWarps struct {
	done       bool
	warpCache  *persist.WarpCache
	addedCount int
}

func (p *parseCIMWarps) Parse(line string) error {
	if strings.Contains(line, "ENDINTERROG") {
		p.done = true
		return nil
	}

	if strings.HasPrefix(line, ":") {
		return nil
	}
	if strings.HasPrefix(line, "Command") {
		return nil
	}

	if strings.TrimSpace(line) == "" {
		return nil
	}

	from, to, err := parseCIMWarpLine(line)
	if err != nil {
		return err
	}

	if p.warpCache.AddIfNeeded(from, to) {
		p.addedCount += 1
	}

	return nil
}

func (p *parseCIMWarps) Done() bool {
	if p.done {
		fmt.Printf("CIM parser added warps for %d sectors\n", p.addedCount)
	}
	return p.done
}

func parseCIMWarpLine(line string) (int, []int, error) {
	line = strings.TrimSpace(line)
	parts := strings.Split(line, " ")

	from, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, []int{}, err
	}
	to := []int{}

	for _, part := range parts[1:] {
		if part != "" {
			sector, err := strconv.Atoi(part)
			if err != nil {
				return 0, []int{}, err
			}
			to = append(to, sector)
		}
	}

	return from, to, nil
}

package tools

import (
	"strconv"
	"strings"
)

func ParsePoints(points string) ([]int, error) {
	parts := strings.Split(points, ",")
	sectors := make([]int, len(parts))
	for i := range parts {
		sector, err := strconv.Atoi(parts[i])
		if err != nil {
			return nil, err
		}
		sectors[i] = sector
	}
	return sectors, nil
}

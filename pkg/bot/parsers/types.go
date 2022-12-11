package parsers

import "strings"

const (
	ROUTEINFO  = "route info"
	SECTORINFO = "sector info"
	PLANETINFO = "planet info"
)

type Parser interface {
	Parse(string) error
	Done() bool
}

func removeCommas(num string) string {
	return strings.ReplaceAll(num, ",", "")
}

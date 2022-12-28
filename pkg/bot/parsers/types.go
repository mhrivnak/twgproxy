package parsers

import "strings"

const (
	ROUTEINFO   = "route info"
	SECTORINFO  = "sector info"
	PLANETINFO  = "planet info"
	PORTROBINFO = "port rob info"
	QUICKSTATS  = "quick stats"
	CORPPLANETS = "corp planet display"
	PORTREPORT  = "port report"
)

type Parser interface {
	Parse(string) error
	Done() bool
}

func removeCommas(num string) string {
	return strings.ReplaceAll(num, ",", "")
}

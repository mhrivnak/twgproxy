package parsers

import "strings"

const (
	CORPPLANETS = "corp planet display"
	DENSITYSCAN = "density scan"
	PLANETINFO  = "planet info"
	PORTREPORT  = "port report"
	PORTROBINFO = "port rob info"
	QUICKSTATS  = "quick stats"
	ROUTEINFO   = "route info"
	SECTORINFO  = "sector info"
)

type Parser interface {
	Parse(string) error
	Done() bool
}

func removeCommas(num string) string {
	return strings.ReplaceAll(num, ",", "")
}

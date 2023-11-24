package parsers

import "strings"

const (
	AVAILABLESHIPS = "available ship scan"
	BUYDETONATORS  = "buy detonators"
	BUYGTORPS      = "buy gtorps"
	CIMWARPS       = "cim warps"
	CONFIGDISPLAY  = "config display"
	CORPPLANETS    = "corp planet display"
	DENSITYSCAN    = "density scan"
	FIGDEPLOY      = "fig deploy"
	PLANETCREATE   = "planet create"
	PLANETINFO     = "planet info"
	PLANETLANDING  = "planet landing"
	PORTEQUTOSTEAL = "port equ to steal"
	PORTREPORT     = "port report"
	PORTROBINFO    = "port rob info"
	ROBRESULT      = "rob result"
	QUICKSTATS     = "quick stats"
	ROUTEINFO      = "route info"
	SECTORINFO     = "sector info"
	SECTORWARPS    = "sector warps"
)

type Parser interface {
	Parse(string) error
	Done() bool
}

func removeCommas(num string) string {
	return strings.ReplaceAll(num, ",", "")
}

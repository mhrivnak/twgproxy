package parsers

import "strings"

const (
	CORPPLANETS   = "corp planet display"
	DENSITYSCAN   = "density scan"
	FIGDEPLOY     = "fig deploy"
	PLANETCREATE  = "planet create"
	PLANETINFO    = "planet info"
	PLANETLANDING = "planet landing"
	PORTREPORT    = "port report"
	PORTROBINFO   = "port rob info"
	ROBRESULT     = "rob result"
	QUICKSTATS    = "quick stats"
	ROUTEINFO     = "route info"
	SECTORINFO    = "sector info"
)

type Parser interface {
	Parse(string) error
	Done() bool
}

func removeCommas(num string) string {
	return strings.ReplaceAll(num, ",", "")
}

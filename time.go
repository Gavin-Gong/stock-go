package stockgo

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/internal/markettime"
)

// MarketTZ identifies one of the three supported wall-clock time zones.
type MarketTZ = string

const (
	MarketTZCN MarketTZ = markettime.CN
	MarketTZHK MarketTZ = markettime.HK
	MarketTZUS MarketTZ = markettime.US
)

type TimeMeta struct {
	Timestamp *int64   `json:"timestamp"`
	TZ        MarketTZ `json:"tz"`
}

var marketWallPattern = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2})(?::(\d{2}))?)?$`)
var marketCompactPattern = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})(?:(\d{2})(\d{2})(\d{2}))?$`)

// ParseMarketTime returns UTC milliseconds, or nil on invalid input. DST folds
// choose the earlier occurrence; spring gaps roll forward by the gap.
// Numeric calendar overflow rolls over as in JavaScript Date.UTC.
func ParseMarketTime(local string, tz MarketTZ) *int64 {
	local = strings.TrimSpace(local)
	parts := marketWallPattern.FindStringSubmatch(local)
	if parts == nil {
		parts = marketCompactPattern.FindStringSubmatch(local)
	}
	if parts == nil {
		return nil
	}
	n := make([]int, 6)
	for i := range n {
		n[i], _ = strconv.Atoi(parts[i+1])
	}
	if n[0] < 100 {
		n[0] += 1900
	} // JavaScript Date.UTC treats years 0..99 as 1900..1999.
	wall := time.Date(n[0], time.Month(n[1]), n[2], n[3], n[4], n[5], 0, time.UTC)
	return markettime.Parse(wall.Format("2006-01-02 15:04:05"), tz)
}
func BuildTimeMeta(local string, tz MarketTZ) TimeMeta {
	return TimeMeta{ParseMarketTime(local, tz), tz}
}

var marketClockPattern = regexp.MustCompile(`^\d{2}:\d{2}(?::\d{2})?$`)

func BuildTimeMetaFromDateAndTime(date, clock string, tz MarketTZ) TimeMeta {
	date, clock = strings.TrimSpace(date), strings.TrimSpace(clock)
	if !marketClockPattern.MatchString(clock) || (len(date) != 8 && len(date) != 10) {
		return TimeMeta{nil, tz}
	}
	if len(date) == 8 {
		date = date[:4] + "-" + date[4:6] + "-" + date[6:]
	}
	return BuildTimeMeta(date+" "+clock, tz)
}

// FormatInTZ formats UTC milliseconds as YYYY-MM-DD HH:mm; nil or invalid zones return "".
func FormatInTZ(epoch *int64, tz MarketTZ) string {
	if epoch == nil || *epoch < -8640000000000000 || *epoch > 8640000000000000 {
		return ""
	}
	if tz != MarketTZCN && tz != MarketTZHK && tz != MarketTZUS {
		return ""
	}
	loc, e := time.LoadLocation(tz)
	if e != nil {
		return ""
	}
	return time.UnixMilli(*epoch).In(loc).Format("2006-01-02 15:04")
}

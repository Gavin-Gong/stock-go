// Package markettime converts provider wall times into UTC milliseconds.
package markettime

import (
	"strings"
	"time"
	_ "time/tzdata" // Keep DST support when deployed without system zoneinfo.
)

const (
	CN = "Asia/Shanghai"
	HK = "Asia/Hong_Kong"
	US = "America/New_York"
)

var locations = func() map[string]*time.Location {
	m := make(map[string]*time.Location)
	for _, name := range []string{CN, HK, US} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			panic(err)
		}
		m[name] = loc
	}
	return m
}()

// Parse returns nil for invalid input. DST overlaps choose the earlier occurrence;
// nonexistent spring times roll forward by the gap, matching stock-sdk v2.
func Parse(local, zone string) *int64 {
	loc := locations[zone]
	if loc == nil {
		return nil
	}
	local = strings.TrimSpace(local)
	for _, layout := range []string{"20060102150405", "20060102", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02T15:04", "2006/01/02 15:04:05", "2006/01/02 15:04", "2006-01-02"} {
		wall, err := time.Parse(layout, local)
		if err != nil || wall.Format(layout) != local {
			continue
		}
		// Sample both sides of the transition to enumerate all plausible offsets.
		offsets := map[int]bool{}
		for _, delta := range []time.Duration{-48 * time.Hour, 0, 48 * time.Hour} {
			_, offset := wall.Add(delta).In(loc).Zone()
			offsets[offset] = true
		}
		var exact, forward time.Time
		var forwardDelta time.Duration
		for offset := range offsets {
			candidate := wall.Add(-time.Duration(offset) * time.Second)
			displayed, err := time.Parse(layout, candidate.In(loc).Format(layout))
			if err != nil {
				continue
			}
			delta := displayed.Sub(wall)
			if delta == 0 && (exact.IsZero() || candidate.Before(exact)) {
				exact = candidate
			}
			if delta > 0 && delta <= 3*time.Hour && (forward.IsZero() || delta < forwardDelta) {
				forward = candidate
				forwardDelta = delta
			}
		}
		if !exact.IsZero() {
			ms := exact.UnixMilli()
			return &ms
		}
		if !forward.IsZero() {
			ms := forward.UnixMilli()
			return &ms
		}
	}
	return nil
}
func DateAndTime(date, clock, zone string) *int64 {
	if len(date) == 8 {
		date = date[:4] + "-" + date[4:6] + "-" + date[6:]
	}
	return Parse(date+" "+clock, zone)
}

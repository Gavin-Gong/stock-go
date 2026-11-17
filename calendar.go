package stockgo

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/symbols"
)

type MarketStatus string

const (
	PreMarket    MarketStatus = "pre_market"
	MarketOpen   MarketStatus = "open"
	LunchBreak   MarketStatus = "lunch_break"
	AfterHours   MarketStatus = "after_hours"
	MarketClosed MarketStatus = "closed"
)

type CalendarService struct{ codes *CodeService }

func calendarDate(input []string) (string, error) {
	if len(input) > 1 {
		return "", argument("at most one date is allowed")
	}
	if len(input) == 0 || input[0] == "" {
		loc, _ := time.LoadLocation(marketZone(symbols.CN))
		return time.Now().In(loc).Format("2006-01-02"), nil
	}
	raw := strings.TrimSpace(input[0])
	if len(raw) == 8 || len(raw) == 10 {
		return dateISO(raw)
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		loc, _ := time.LoadLocation(marketZone(symbols.CN))
		return t.In(loc).Format("2006-01-02"), nil
	}
	return "", argument("date must be YYYYMMDD, YYYY-MM-DD or RFC3339")
}
func (s *CalendarService) IsTradingDay(ctx context.Context, date ...string) (bool, error) {
	target, err := calendarDate(date)
	if err != nil {
		return false, err
	}
	calendar, err := s.codes.calendar(ctx)
	if err != nil {
		return false, err
	}
	i := sort.SearchStrings(calendar, target)
	return i < len(calendar) && calendar[i] == target, nil
}
func (s *CalendarService) NextTradingDay(ctx context.Context, date ...string) (string, error) {
	target, err := calendarDate(date)
	if err != nil {
		return "", err
	}
	calendar, err := s.codes.calendar(ctx)
	if err != nil {
		return "", err
	}
	i := sort.SearchStrings(calendar, target)
	if i < len(calendar) && calendar[i] == target {
		i++
	}
	if i >= len(calendar) {
		return "", argument("no trading day after " + target + " in calendar")
	}
	return calendar[i], nil
}
func (s *CalendarService) PrevTradingDay(ctx context.Context, date ...string) (string, error) {
	target, err := calendarDate(date)
	if err != nil {
		return "", err
	}
	calendar, err := s.codes.calendar(ctx)
	if err != nil {
		return "", err
	}
	i := sort.SearchStrings(calendar, target) - 1
	if i < 0 {
		return "", argument("no trading day before " + target + " in calendar")
	}
	return calendar[i], nil
}

// MarketStatus uses weekday/session schedules only; it does not check holidays or half days.
func (s *CalendarService) MarketStatus(market symbols.Market, at ...time.Time) (MarketStatus, error) {
	if len(at) > 1 {
		return "", argument("at most one time is allowed")
	}
	if market == "" {
		market = symbols.CN
	}
	if market != symbols.CN && market != symbols.HK && market != symbols.US {
		return "", argument("market must be CN, HK or US")
	}
	now := time.Now()
	if len(at) == 1 {
		now = at[0]
	}
	loc, _ := time.LoadLocation(marketZone(market))
	local := now.In(loc)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return MarketClosed, nil
	}
	minutes := local.Hour()*60 + local.Minute()
	open, close, lunchStart, lunchEnd := 570, 900, 690, 780
	if market == symbols.HK {
		close, lunchStart = 960, 720
	} else if market == symbols.US {
		close, lunchStart, lunchEnd = 960, 0, 0
	}
	if minutes < open {
		return PreMarket, nil
	}
	if minutes >= close {
		return AfterHours, nil
	}
	if lunchStart > 0 && minutes >= lunchStart && minutes < lunchEnd {
		return LunchBreak, nil
	}
	return MarketOpen, nil
}

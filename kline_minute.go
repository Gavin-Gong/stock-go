package stockgo

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/internal/markettime"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"github.com/Gavin-Gong/stock-go/symbols"
)

func minuteBoundary(raw string, end bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) == 8 || len(raw) == 10 {
		date, err := dateISO(raw)
		if err != nil {
			return "", err
		}
		if end {
			return date + " 23:59", nil
		}
		return date + " 00:00", nil
	}
	for _, layout := range []string{"200601021504", "20060102 15:04", "2006-01-02 15:04", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02T15:04:05"} {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return t.Format("2006-01-02 15:04"), nil
		}
	}
	return "", argument("invalid minute boundary: " + raw)
}
func minuteOptions(options []MinuteOptions) (MinuteOptions, error) {
	if len(options) > 1 {
		return MinuteOptions{}, argument("at most one MinuteOptions is allowed")
	}
	var o MinuteOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Period == "" {
		o.Period = "1"
	}
	switch o.Period {
	case "1", "5", "15", "30", "60":
	default:
		return o, argument("minute period must be 1, 5, 15, 30 or 60")
	}
	a, _, err := adjustCode(o.Adjust)
	if err != nil {
		return o, err
	}
	o.Adjust = a
	if o.Period == "1" {
		o.Adjust = AdjustNone
	}
	if o.NDays < 0 || o.NDays > 5 {
		return o, argument("ndays must be 1..5 (or zero for default)")
	}
	if o.NDays == 0 {
		o.NDays = 1
	}
	o.StartDate, err = minuteBoundary(o.StartDate, false)
	if err != nil {
		return o, err
	}
	o.EndDate, err = minuteBoundary(o.EndDate, true)
	if err != nil {
		return o, err
	}
	if o.StartDate != "" && o.EndDate != "" && o.StartDate > o.EndDate {
		return o, argument("startDate exceeds endDate")
	}
	return o, nil
}
func (s *KlineService) CNMinute(ctx context.Context, code string, options ...MinuteOptions) ([]KlineBar, error) {
	return s.minute(ctx, code, symbols.CN, options)
}
func (s *KlineService) HKMinute(ctx context.Context, code string, options ...MinuteOptions) ([]KlineBar, error) {
	return s.minute(ctx, code, symbols.HK, options)
}
func (s *KlineService) USMinute(ctx context.Context, code string, options ...MinuteOptions) ([]KlineBar, error) {
	return s.minute(ctx, code, symbols.US, options)
}
func (s *KlineService) minute(ctx context.Context, code string, market symbols.Market, options []MinuteOptions) ([]KlineBar, error) {
	o, err := minuteOptions(options)
	if err != nil {
		return nil, err
	}
	secid, plain, err := s.target(ctx, code, market)
	if err != nil {
		return nil, err
	}
	_, fqt, _ := adjustCode(o.Adjust)
	start, end := "0", "20500000"
	if o.StartDate != "" {
		start = compactDate(o.StartDate[:10])
	}
	if o.EndDate != "" {
		end = o.EndDate[:10]
		if market == symbols.US {
			end = dateAdd(end, 1)
		}
		end = compactDate(end)
	}
	params := emParams(secid, o.Period, fqt, start, end)
	params.Del("lmt")
	if o.Period == "1" {
		ndays := o.NDays
		if market == symbols.CN {
			ndays = 5
		}
		params = url.Values{"fields1": {"f1,f2,f3,f4,f5,f6,f7,f8,f9,f10,f11,f12,f13"}, "fields2": {"f51,f52,f53,f54,f55,f56,f57,f58"}, "ut": {emToken}, "secid": {secid}, "ndays": {numberString(ndays)}, "iscr": {"0"}}
	}
	client := s.client
	if market == symbols.CN && s.fallback && o.Period != "1" {
		client = s.probe
	}
	data, err := fetchEM(ctx, client, s.marketEndpoint(market, o.Period == "1"), params)
	if err == nil && data == nil && market == symbols.CN && o.Period != "1" {
		err = klineError(sdkerrors.UpstreamEmpty, "eastmoney", s.endpoints.CN, "minute K-line response has no data payload", nil)
	}
	if err != nil {
		if market != symbols.CN || !s.fallback || o.Period == "1" || !canFallback(err) {
			return nil, err
		}
		primary := err
		if bars, err := s.tencentMinute(ctx, code, o); err == nil {
			return bars, nil
		}
		if ctx.Err() != nil {
			return nil, wrapContext(ctx.Err())
		}
		if bars, err := s.sinaBars(ctx, code, KlineOptions{StartDate: o.StartDate, EndDate: o.EndDate}, o.Period); err == nil {
			return bars, nil
		}
		if ctx.Err() != nil {
			return nil, wrapContext(ctx.Err())
		}
		return nil, primary
	}
	out := []KlineBar{}
	if data == nil {
		return out, nil
	}
	raws := data.Klines
	if o.Period == "1" {
		raws = data.Trends
	}
	for _, raw := range raws {
		line, ok := rawString(raw)
		if !ok {
			continue
		}
		bar := csvBar(line)
		rawTime := bar.Date
		bar.Date = ""
		bar.Time = rawTime
		if o.Period == "1" {
			f := strings.Split(line, ",")
			bar.AvgPrice = nullable(at(f, 7))
			bar.Amplitude = nil
			bar.ChangePercent = nil
			bar.Change = nil
			bar.TurnoverRate = nil
		}
		bar.TZ = marketZone(market)
		bar.Timestamp = markettime.Parse(rawTime, markettime.CN)
		if market != symbols.CN {
			bar.Code = plain
			bar.Currency = "HKD"
			if market == symbols.US {
				bar.Currency = "USD"
			}
			if bar.Timestamp != nil {
				loc, _ := time.LoadLocation(bar.TZ)
				bar.Time = time.UnixMilli(*bar.Timestamp).In(loc).Format("2006-01-02 15:04")
			}
		}
		bar.Source = "eastmoney"
		bar.Adjust = o.Adjust
		if inWindow(bar.Time, o.StartDate, o.EndDate) {
			out = append(out, bar)
		}
	}
	return out, nil
}
func inWindow(s, start, end string) bool {
	return (start == "" || s >= start) && (end == "" || s <= end)
}

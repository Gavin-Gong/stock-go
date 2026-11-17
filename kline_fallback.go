package stockgo

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/internal/markettime"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"github.com/Gavin-Gong/stock-go/symbols"
)

func numberString(n int) string { return strconv.Itoa(n) }
func roundTo(n float64, digits int) *float64 {
	factor := math.Pow10(digits)
	v := math.Floor(n*factor+0.5) / factor
	return &v
}
func derive(b *KlineBar, previous *float64) {
	if b.Close != nil && previous != nil {
		b.Change = roundTo(*b.Close-*previous, 3)
		if *previous != 0 {
			b.ChangePercent = roundTo(*b.Change / *previous * 100, 2)
		}
	}
	if b.High != nil && b.Low != nil && previous != nil && *previous != 0 {
		b.Amplitude = roundTo((*b.High-*b.Low) / *previous * 100, 2)
	}
}
func rawNumber(raw json.RawMessage) *float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return nullable(s)
	}
	return nullable(string(raw))
}
func waitPage(ctx context.Context) error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return wrapContext(ctx.Err())
	case <-timer.C:
		return nil
	}
}
func (s *KlineService) tencentPage(ctx context.Context, key, period, adjust, start, end string, minute bool) ([]KlineBar, error) {
	endpoint := s.endpoints.Tencent
	param := strings.Join([]string{key, period, start, end, "640", adjust}, ",")
	if minute {
		endpoint = s.endpoints.TencentMinute
		param = strings.Join([]string{key, period, end, "640"}, ",")
	}
	endpoint = endpointQuery(endpoint, url.Values{"param": {param}})
	var response struct {
		Code *int                                  `json:"code"`
		Msg  string                                `json:"msg"`
		Data map[string]map[string]json.RawMessage `json:"data"`
	}
	if err := getJSON(ctx, s.client, "tencent", endpoint, &response); err != nil {
		return nil, err
	}
	if response.Code != nil && *response.Code != 0 {
		return nil, klineError(sdkerrors.UpstreamError, "tencent", endpoint, response.Msg, nil)
	}
	node, ok := response.Data[key]
	if !ok || node == nil {
		return nil, klineError(sdkerrors.UpstreamEmpty, "tencent", endpoint, "missing symbol payload", nil)
	}
	raw := node[adjust+period]
	if len(raw) == 0 {
		raw = node[period]
	}
	out := []KlineBar{}
	if len(raw) == 0 || string(raw) == "null" {
		return out, nil
	}
	var rows [][]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, klineError(sdkerrors.Parse, "tencent", endpoint, "invalid OHLCV rows", err)
	}
	for _, r := range rows {
		if len(r) == 0 {
			continue
		}
		date, ok := rawString(r[0])
		if !ok || date == "" {
			continue
		}
		if minute {
			if len(date) != 12 || !digitsOnly(date) {
				continue
			}
			date = date[:4] + "-" + date[4:6] + "-" + date[6:8] + " " + date[8:10] + ":" + date[10:]
		}
		b := KlineBar{Date: date, Source: "tencent", TZ: markettime.CN}
		nums := []**float64{&b.Open, &b.Close, &b.High, &b.Low, &b.Volume}
		for i, p := range nums {
			if i+1 < len(r) {
				*p = rawNumber(r[i+1])
			}
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}
func (s *KlineService) tencentHistory(ctx context.Context, code string, o KlineOptions) ([]KlineBar, error) {
	ns, err := symbols.Normalize(code, symbols.Hint{Market: symbols.CN})
	if err != nil {
		return nil, err
	}
	key, err := symbols.ToTencent(ns)
	if err != nil {
		return nil, err
	}
	period := map[string]string{"daily": "day", "weekly": "week", "monthly": "month"}[o.Period]
	lookback := map[string]int{"daily": 10, "weekly": 14, "monthly": 45}[o.Period]
	start := dateAdd(o.StartDate, -lookback)
	cursor := o.EndDate
	adjust := string(o.Adjust)
	if o.Adjust == AdjustQFQ {
		adjust = "hfq"
	}
	if o.Adjust == AdjustNone {
		adjust = ""
	}
	byDate := map[string]KlineBar{}
	for page := 0; page < 100; page++ {
		rows, err := s.tencentPage(ctx, key, period, adjust, start, cursor, false)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			byDate[row.Date] = row
		}
		if len(rows) == 0 || len(rows) < 640 || rows[0].Date <= start {
			break
		}
		next := dateAdd(rows[0].Date, -1)
		if next >= cursor || page == 99 {
			return nil, klineError(sdkerrors.UpstreamEmpty, "tencent", s.endpoints.Tencent, "history pagination did not cover the requested window", nil)
		}
		cursor = next
		if err := waitPage(ctx); err != nil {
			return nil, err
		}
	}
	rows := sortedBars(byDate)
	if o.Adjust == AdjustQFQ && len(rows) > 0 {
		anchor := -1
		for i := len(rows) - 1; i >= 0; i-- {
			if rows[i].Close != nil && *rows[i].Close != 0 {
				anchor = i
				break
			}
		}
		if anchor < 0 {
			return nil, klineError(sdkerrors.UpstreamEmpty, "tencent", "", "no usable hfq anchor", nil)
		}
		raw, err := s.tencentPage(ctx, key, period, "", dateAdd(rows[anchor].Date, -lookback), rows[anchor].Date, false)
		if err != nil {
			return nil, err
		}
		var rawClose *float64
		for _, bar := range raw {
			if bar.Date == rows[anchor].Date {
				rawClose = bar.Close
				break
			}
		}
		if rawClose == nil {
			return nil, klineError(sdkerrors.UpstreamEmpty, "tencent", "", "missing unadjusted qfq anchor", nil)
		}
		scale := *rawClose / *rows[anchor].Close
		for i := range rows {
			for _, p := range []**float64{&rows[i].Open, &rows[i].Close, &rows[i].High, &rows[i].Low} {
				if *p != nil {
					*p = roundTo(**p*scale, 3)
				}
			}
		}
	}
	return finishFallback(rows, ns.Code, o.StartDate, o.EndDate, o.Adjust, false), nil
}
func sortedBars(byDate map[string]KlineBar) []KlineBar {
	rows := make([]KlineBar, 0, len(byDate))
	for _, row := range byDate {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
	return rows
}
func finishFallback(rows []KlineBar, code, start, end string, adjust AdjustType, minute bool) []KlineBar {
	out := []KlineBar{}
	var prev *float64
	for _, b := range rows {
		derive(&b, prev)
		prev = b.Close
		b.Timestamp = markettime.Parse(b.Date, markettime.CN)
		b.TZ = markettime.CN
		b.Adjust = adjust
		b.Code = code
		date := b.Date
		if minute {
			b.Time = b.Date
			b.Date = ""
		}
		if inWindow(date, start, end) {
			out = append(out, b)
		}
	}
	return out
}
func (s *KlineService) tencentMinute(ctx context.Context, code string, o MinuteOptions) ([]KlineBar, error) {
	ns, err := symbols.Normalize(code, symbols.Hint{Market: symbols.CN})
	if err != nil {
		return nil, err
	}
	key, err := symbols.ToTencent(ns)
	if err != nil {
		return nil, err
	}
	byDate := map[string]KlineBar{}
	cursor := strings.NewReplacer("-", "", " ", "", ":", "").Replace(o.EndDate)
	for page := 0; page < 100; page++ {
		rows, err := s.tencentPage(ctx, key, "m"+o.Period, "", "", cursor, true)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			byDate[r.Date] = r
		}
		if len(rows) == 0 || o.StartDate == "" || len(rows) < 640 || rows[0].Date <= o.StartDate {
			break
		}
		next := strings.NewReplacer("-", "", " ", "", ":", "").Replace(rows[0].Date)
		if next == cursor || page == 99 {
			return nil, klineError(sdkerrors.UpstreamEmpty, "tencent", s.endpoints.TencentMinute, "minute pagination did not advance", nil)
		}
		cursor = next
		if err := waitPage(ctx); err != nil {
			return nil, err
		}
	}
	// Tencent minute OHLCV is unadjusted; unlike the source SDK, report that explicitly.
	return finishFallback(sortedBars(byDate), ns.Code, o.StartDate, o.EndDate, AdjustNone, true), nil
}
func (s *KlineService) sinaBars(ctx context.Context, code string, o KlineOptions, minute string) ([]KlineBar, error) {
	ns, err := symbols.Normalize(code, symbols.Hint{Market: symbols.CN})
	if err != nil {
		return nil, err
	}
	key, err := symbols.ToTencent(ns)
	if err != nil {
		return nil, err
	}
	scale, depth := "240", "10000"
	if minute != "" {
		scale, depth = minute, "5000"
	}
	endpoint := endpointQuery(s.endpoints.Sina, url.Values{"symbol": {key}, "scale": {scale}, "ma": {"no"}, "datalen": {depth}})
	var raw json.RawMessage
	if err := getJSON(ctx, s.client, "sina", endpoint, &raw); err != nil {
		return nil, err
	}
	var data []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, klineError(sdkerrors.UpstreamError, "sina", endpoint, "invalid Sina OHLCV payload", err)
	}
	if string(raw) == "null" {
		return nil, klineError(sdkerrors.UpstreamEmpty, "sina", endpoint, "empty Sina payload", nil)
	}
	rows := []KlineBar{}
	for _, r := range data {
		date, ok := rawString(r["day"])
		if !ok || date == "" {
			continue
		}
		if len(date) > 16 {
			date = date[:16]
		}
		b := KlineBar{Date: date, Open: rawNumber(r["open"]), Close: rawNumber(r["close"]), High: rawNumber(r["high"]), Low: rawNumber(r["low"]), Volume: rawNumber(r["volume"]), Source: "sina"}
		if b.Volume != nil {
			v := *b.Volume / 100
			b.Volume = &v
		}
		if minute != "" {
			b.Amount = rawNumber(r["amount"])
		}
		rows = append(rows, b)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
	depthN, _ := strconv.Atoi(depth)
	if len(rows) >= depthN && o.StartDate != "" && rows[0].Date > o.StartDate {
		return nil, klineError(sdkerrors.UpstreamEmpty, "sina", endpoint, "fallback history depth insufficient", nil)
	}
	if minute == "" && o.Period != "daily" {
		rows = aggregateSina(rows, o.Period)
	}
	return finishFallback(rows, ns.Code, o.StartDate, o.EndDate, AdjustNone, minute != ""), nil
}
func aggregateSina(rows []KlineBar, period string) []KlineBar {
	out := []KlineBar{}
	lastKey := ""
	for _, row := range rows {
		date := row.Date
		if len(date) > 10 {
			date = date[:10]
		}
		key := date
		if period == "weekly" {
			t, err := time.Parse("2006-01-02", date)
			if err != nil {
				continue
			}
			offset := (int(t.Weekday()) + 6) % 7
			key = t.AddDate(0, 0, -offset).Format("2006-01-02")
		} else {
			if len(date) < 7 {
				continue
			}
			key = date[:7]
		}
		if key != lastKey {
			row.Amount = nil
			out = append(out, row)
			lastKey = key
			continue
		}
		b := &out[len(out)-1]
		b.Date = row.Date
		b.Close = row.Close
		if row.High != nil && (b.High == nil || *row.High > *b.High) {
			b.High = row.High
		}
		if row.Low != nil && (b.Low == nil || *row.Low < *b.Low) {
			b.Low = row.Low
		}
		if row.Volume != nil {
			v := *row.Volume
			if b.Volume != nil {
				v += *b.Volume
			}
			b.Volume = roundTo(v, 2)
		}
	}
	return out
}

package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/internal/markettime"
	"github.com/Gavin-Gong/stock-go/internal/memo"
	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"github.com/Gavin-Gong/stock-go/symbols"
)

type AdjustType string

const (
	AdjustQFQ  AdjustType = "qfq"
	AdjustHFQ  AdjustType = "hfq"
	AdjustNone AdjustType = "none"
)

type KlineOptions struct {
	Period             string
	Adjust             AdjustType
	StartDate, EndDate string
}
type MinuteOptions struct {
	Period             string
	Adjust             AdjustType
	StartDate, EndDate string
	NDays              int
}

// KlineBar represents history (Date) or minute data (Time). Numbers absent upstream remain nil.
// Source and Adjust describe the actual data returned, including fallback limitations.
type KlineBar struct {
	Date          string     `json:"date,omitempty"`
	Time          string     `json:"time,omitempty"`
	Timestamp     *int64     `json:"timestamp"`
	TZ            string     `json:"tz"`
	Code          string     `json:"code,omitempty"`
	Name          string     `json:"name,omitempty"`
	Currency      string     `json:"currency,omitempty"`
	Source        string     `json:"source"`
	Adjust        AdjustType `json:"adjust"`
	Open          *float64   `json:"open"`
	Close         *float64   `json:"close"`
	High          *float64   `json:"high"`
	Low           *float64   `json:"low"`
	Volume        *float64   `json:"volume"`
	Amount        *float64   `json:"amount"`
	Amplitude     *float64   `json:"amplitude"`
	ChangePercent *float64   `json:"changePercent"`
	Change        *float64   `json:"change"`
	TurnoverRate  *float64   `json:"turnoverRate"`
	AvgPrice      *float64   `json:"avgPrice,omitempty"`
}
type KlineEndpoints struct{ CN, HK, US, Trends, HKTrends, USTrends, Tencent, TencentMinute, Sina string }
type KlineService struct {
	client, probe *transport.Client
	codes         *CodeService
	endpoints     KlineEndpoints
	fallback      bool
	usCache       *memo.Store[string]
}

const emToken = "7eea3edcaed734bea9cbfc24409ed989"

func klineError(code sdkerrors.Code, provider, endpoint, message string, cause error) error {
	return &sdkerrors.Error{Code: code, Provider: provider, URL: endpoint, Message: message, Cause: cause}
}
func getJSON(ctx context.Context, c *transport.Client, provider, endpoint string, target any) error {
	return c.GetJSON(ctx, provider, endpoint, target)
}
func adjustCode(a AdjustType) (AdjustType, string, error) {
	switch a {
	case "", AdjustQFQ:
		return AdjustQFQ, "1", nil
	case AdjustHFQ:
		return a, "2", nil
	case AdjustNone:
		return a, "0", nil
	}
	return a, "", argument("adjust must be qfq, hfq or none")
}
func historyOptions(options []KlineOptions) (KlineOptions, error) {
	if len(options) > 1 {
		return KlineOptions{}, argument("at most one KlineOptions is allowed")
	}
	var o KlineOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Period == "" {
		o.Period = "daily"
	}
	if o.Period != "daily" && o.Period != "weekly" && o.Period != "monthly" {
		return o, argument("period must be daily, weekly or monthly")
	}
	a, _, err := adjustCode(o.Adjust)
	if err != nil {
		return o, err
	}
	o.Adjust = a
	if o.StartDate == "" {
		o.StartDate = "19700101"
	}
	if o.EndDate == "" {
		o.EndDate = "20500101"
	}
	start, err := dateISO(o.StartDate)
	if err != nil {
		return o, err
	}
	end, err := dateISO(o.EndDate)
	if err != nil {
		return o, err
	}
	if start > end {
		return o, argument("startDate exceeds endDate")
	}
	o.StartDate = start
	o.EndDate = end
	return o, nil
}
func dateISO(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 8 && digitsOnly(s) {
		s = s[:4] + "-" + s[4:6] + "-" + s[6:]
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return "", argument("invalid calendar date: " + s)
	}
	return t.Format("2006-01-02"), nil
}
func dateAdd(s string, n int) string {
	t, _ := time.Parse("2006-01-02", s)
	return t.AddDate(0, 0, n).Format("2006-01-02")
}
func compactDate(s string) string { return strings.ReplaceAll(s, "-", "") }
func emParams(secid, period, fqt, start, end string) url.Values {
	return url.Values{"fields1": {"f1,f2,f3,f4,f5,f6"}, "fields2": {"f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61"}, "ut": {emToken}, "secid": {secid}, "klt": {period}, "fqt": {fqt}, "beg": {start}, "end": {end}, "lmt": {"1000000"}}
}

type emPayload struct {
	Code, Name     string
	Klines, Trends []json.RawMessage
}

func fetchEM(ctx context.Context, c *transport.Client, endpoint string, params url.Values) (*emPayload, error) {
	endpoint = endpointQuery(endpoint, params)
	var response struct {
		RC   int        `json:"rc"`
		Data *emPayload `json:"data"`
	}
	if err := getJSON(ctx, c, "eastmoney", endpoint, &response); err != nil {
		return nil, err
	}
	if response.RC != 0 {
		return nil, klineError(sdkerrors.UpstreamError, "eastmoney", endpoint, fmt.Sprintf("rc=%d", response.RC), nil)
	}
	return response.Data, nil
}
func csvBar(line string) KlineBar {
	f := strings.Split(line, ",")
	return KlineBar{Date: at(f, 0), Open: nullable(at(f, 1)), Close: nullable(at(f, 2)), High: nullable(at(f, 3)), Low: nullable(at(f, 4)), Volume: nullable(at(f, 5)), Amount: nullable(at(f, 6)), Amplitude: nullable(at(f, 7)), ChangePercent: nullable(at(f, 8)), Change: nullable(at(f, 9)), TurnoverRate: nullable(at(f, 10))}
}
func (s *KlineService) CN(ctx context.Context, code string, options ...KlineOptions) ([]KlineBar, error) {
	return s.history(ctx, code, symbols.CN, options)
}
func (s *KlineService) HK(ctx context.Context, code string, options ...KlineOptions) ([]KlineBar, error) {
	return s.history(ctx, code, symbols.HK, options)
}
func (s *KlineService) US(ctx context.Context, code string, options ...KlineOptions) ([]KlineBar, error) {
	return s.history(ctx, code, symbols.US, options)
}
func (s *KlineService) target(ctx context.Context, code string, market symbols.Market) (string, string, error) {
	if market == symbols.US && regexp.MustCompile(`^\d{2,3}\.[A-Za-z0-9.\-]+$`).MatchString(code) {
		_, plain, _ := strings.Cut(code, ".")
		return code, canonicalUS(code, plain), nil
	}
	ns, err := symbols.Normalize(code, symbols.Hint{Market: market})
	if err != nil {
		return "", "", err
	}
	secid, err := symbols.ToEastmoney(ns)
	if err != nil {
		return "", "", err
	}
	if market != symbols.US || ns.AssetType == symbols.Index || ns.Exchange != "US" {
		return secid, ns.Code, nil
	}
	resolved, err := s.usCache.Load(ctx, ns.Code, 7*24*time.Hour, func(ctx context.Context) (string, error) {
		list, err := s.codes.US(ctx)
		if err == nil {
			for _, id := range list {
				_, plain, _ := strings.Cut(id, ".")
				if plain == ns.Code {
					return id, nil
				}
			}
		}
		if ctx.Err() != nil {
			return "", wrapContext(ctx.Err())
		}
		for _, prefix := range []string{"105.", "106.", "107."} {
			id := prefix + ns.Code
			params := emParams(id, "101", "0", "19700101", "20500101")
			params.Set("lmt", "1")
			payload, err := fetchEM(ctx, s.client, s.endpoints.US, params)
			if err != nil {
				return "", err
			}
			if payload != nil && len(payload.Klines) > 0 {
				return id, nil
			}
		}
		return "", klineError(sdkerrors.NotFound, "eastmoney", "", "US ticker not found: "+ns.Code, nil)
	})
	return resolved, ns.Code, wrapContext(err)
}
func canonicalUS(secid, plain string) string {
	switch secid {
	case "100.DJIA":
		return "DJI"
	case "100.SPX":
		return "INX"
	case "100.NDX":
		return "IXIC"
	}
	return plain
}
func marketZone(m symbols.Market) string {
	switch m {
	case symbols.HK:
		return markettime.HK
	case symbols.US:
		return markettime.US
	}
	return markettime.CN
}
func (s *KlineService) marketEndpoint(m symbols.Market, trends bool) string {
	if trends {
		switch m {
		case symbols.HK:
			return s.endpoints.HKTrends
		case symbols.US:
			return s.endpoints.USTrends
		}
		return s.endpoints.Trends
	}
	switch m {
	case symbols.HK:
		return s.endpoints.HK
	case symbols.US:
		return s.endpoints.US
	}
	return s.endpoints.CN
}
func (s *KlineService) history(ctx context.Context, code string, market symbols.Market, options []KlineOptions) ([]KlineBar, error) {
	o, err := historyOptions(options)
	if err != nil {
		return nil, err
	}
	secid, plain, err := s.target(ctx, code, market)
	if err != nil {
		return nil, err
	}
	_, fqt, _ := adjustCode(o.Adjust)
	period := map[string]string{"daily": "101", "weekly": "102", "monthly": "103"}[o.Period]
	params := emParams(secid, period, fqt, compactDate(o.StartDate), compactDate(o.EndDate))
	if market == symbols.CN {
		params.Set("fields2", params.Get("fields2")+",f116")
	}
	client := s.client
	if s.fallback && market == symbols.CN {
		client = s.probe
	}
	data, err := fetchEM(ctx, client, s.marketEndpoint(market, false), params)
	if err == nil && data == nil && market == symbols.CN {
		err = klineError(sdkerrors.UpstreamEmpty, "eastmoney", s.endpoints.CN, "K-line response has no data payload", nil)
	}
	if err != nil {
		if market != symbols.CN || !s.fallback || !canFallback(err) {
			return nil, err
		}
		primary := err
		if bars, err := s.tencentHistory(ctx, code, o); err == nil {
			return bars, nil
		}
		if ctx.Err() != nil {
			return nil, wrapContext(ctx.Err())
		}
		if bars, err := s.sinaBars(ctx, code, o, ""); err == nil {
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
	for _, raw := range data.Klines {
		line, ok := rawString(raw)
		if !ok {
			continue
		}
		bar := csvBar(line)
		bar.Code = plain
		bar.TZ = marketZone(market)
		bar.Timestamp = markettime.Parse(bar.Date, bar.TZ)
		bar.Source = "eastmoney"
		bar.Adjust = o.Adjust
		if market != symbols.CN {
			bar.Name = data.Name
			if data.Code != "" {
				bar.Code = data.Code
			}
			if market == symbols.US {
				bar.Code = canonicalUS(secid, bar.Code)
				bar.Currency = "USD"
			} else {
				bar.Currency = "HKD"
			}
		}
		out = append(out, bar)
	}
	return out, nil
}
func canFallback(err error) bool {
	switch sdkerrors.CodeOf(err) {
	case sdkerrors.Network, sdkerrors.Timeout, sdkerrors.HTTP, sdkerrors.RateLimited, sdkerrors.CircuitOpen, sdkerrors.UpstreamEmpty, sdkerrors.UpstreamError, sdkerrors.Parse:
		return true
	}
	return false
}

func rawString(raw json.RawMessage) (string, bool) {
	var value *string
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return "", false
	}
	return *value, true
}

package stockgo

import (
	"context"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/internal/markettime"
	"github.com/Gavin-Gong/stock-go/internal/memo"
	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type BoardEndpoints struct{ List, Spot, Constituents, Kline, Trends string }
type BoardService struct{ Industry, Concept *BoardProvider }
type BoardProvider struct {
	client    *transport.Client
	endpoints BoardEndpoints
	kind      string
	names     *memo.Store[map[string]string]
}

const boardToken = "bd1d9ddb04089700cf9c27f6f7426281"

var boardCodePattern = regexp.MustCompile(`^BK\d+$`)

const boardListFields = "f1,f2,f3,f4,f5,f6,f7,f8,f9,f10,f12,f13,f14,f15,f16,f17,f18,f20,f21,f23,f24,f25,f26,f22,f33,f11,f62,f128,f136,f115,f152,f124,f107,f104,f105,f140,f141,f207,f208,f209,f222"
const conceptListFields = "f2,f3,f4,f8,f12,f14,f15,f16,f17,f18,f20,f21,f24,f25,f22,f33,f11,f62,f128,f124,f107,f104,f105,f136"
const boardConstituentFields = "f1,f2,f3,f4,f5,f6,f7,f8,f9,f10,f12,f13,f14,f15,f16,f17,f18,f20,f21,f23,f24,f25,f22,f11,f62,f128,f136,f115,f152,f45"

func (s *BoardProvider) List(ctx context.Context, options ...PageOptions) ([]BoardItem, error) {
	o, err := pageOptions(options)
	if err != nil {
		return nil, err
	}
	filter, fid, fields := "m:90 t:2 f:!50", "f3", boardListFields
	if s.kind == "concept" {
		filter, fid, fields = "m:90 t:3 f:!50", "f12", conceptListFields
	}
	rows, err := clistRows(ctx, s.client, s.endpoints.List, url.Values{"po": {"1"}, "np": {"1"}, "ut": {boardToken}, "fltt": {"2"}, "invt": {"2"}, "fid": {fid}, "fs": {filter}, "fields": {fields}}, o, false)
	if err != nil {
		return nil, err
	}
	out := make([]BoardItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, BoardItem{Name: rowText(r, "f14"), Code: rowText(r, "f12"), Price: rowNumber(r, "f2"), Change: rowNumber(r, "f4"), ChangePercent: rowNumber(r, "f3"), TotalMarketCap: rowNumber(r, "f20"), TurnoverRate: rowNumber(r, "f8"), RiseCount: rowNumber(r, "f104"), FallCount: rowNumber(r, "f105"), LeadingStock: optionalText(r, "f128"), LeadingStockChangePercent: rowNumber(r, "f136")})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := 0.0, 0.0
		if out[i].ChangePercent != nil {
			a = *out[i].ChangePercent
		}
		if out[j].ChangePercent != nil {
			b = *out[j].ChangePercent
		}
		return a > b
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out, nil
}
func (s *BoardProvider) code(ctx context.Context, symbol string) (string, error) {
	if strings.HasPrefix(symbol, "90.") {
		symbol = strings.TrimPrefix(symbol, "90.")
	}
	if boardCodePattern.MatchString(symbol) {
		return symbol, nil
	}
	if strings.TrimSpace(symbol) == "" {
		return "", argument("board symbol must not be empty")
	}
	names, err := s.names.Load(ctx, "names", time.Hour, func(ctx context.Context) (map[string]string, error) {
		boards, err := s.List(ctx)
		if err != nil {
			return nil, err
		}
		if len(boards) == 0 {
			return nil, klineError(sdkerrors.UpstreamEmpty, "eastmoney", s.endpoints.List, "empty board list", nil)
		}
		names := map[string]string{}
		for _, b := range boards {
			names[b.Name] = b.Code
		}
		return names, nil
	})
	if err != nil {
		return "", wrapContext(err)
	}
	code, ok := names[symbol]
	if !ok {
		return "", klineError(sdkerrors.NotFound, "eastmoney", "", "board not found: "+symbol, nil)
	}
	return code, nil
}
func (s *BoardProvider) Spot(ctx context.Context, symbol string) ([]BoardSpot, error) {
	code, err := s.code(ctx, symbol)
	if err != nil {
		return nil, err
	}
	endpoint := endpointQuery(s.endpoints.Spot, url.Values{"fields": {"f43,f44,f45,f46,f47,f48,f170,f171,f168,f169"}, "mpi": {"1000"}, "invt": {"2"}, "fltt": {"1"}, "secid": {"90." + code}})
	var response struct {
		Data emRow `json:"data"`
	}
	if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
		return nil, err
	}
	out := []BoardSpot{}
	if response.Data == nil {
		return out, nil
	}
	for _, field := range []struct {
		key, name string
		divide    bool
	}{{"f43", "最新", true}, {"f44", "最高", true}, {"f45", "最低", true}, {"f46", "开盘", true}, {"f47", "成交量", false}, {"f48", "成交额", false}, {"f170", "涨跌幅", true}, {"f171", "振幅", true}, {"f168", "换手率", true}, {"f169", "涨跌额", true}} {
		var value *float64
		if _, isString := rawString(response.Data[field.key]); !isString {
			value = rowNumber(response.Data, field.key)
		}
		if value != nil && field.divide {
			v := *value / 100
			value = &v
		}
		out = append(out, BoardSpot{field.name, value})
	}
	return out, nil
}
func (s *BoardProvider) Constituents(ctx context.Context, symbol string, options ...PageOptions) ([]BoardConstituent, error) {
	o, err := pageOptions(options)
	if err != nil {
		return nil, err
	}
	code, err := s.code(ctx, symbol)
	if err != nil {
		return nil, err
	}
	params := url.Values{"po": {"1"}, "np": {"1"}, "ut": {boardToken}, "fltt": {"2"}, "invt": {"2"}, "fid": {"f3"}, "fs": {"b:" + code + " f:!50"}, "fields": {boardConstituentFields}}
	rows, err := clistRows(ctx, s.client, s.endpoints.Constituents, params, o, false)
	if err != nil {
		return nil, err
	}
	out := make([]BoardConstituent, 0, len(rows))
	for i, r := range rows {
		out = append(out, BoardConstituent{Rank: i + 1, Code: rowText(r, "f12"), Name: rowText(r, "f14"), Price: rowNumber(r, "f2"), ChangePercent: rowNumber(r, "f3"), Change: rowNumber(r, "f4"), Volume: rowNumber(r, "f5"), Amount: rowNumber(r, "f6"), Amplitude: rowNumber(r, "f7"), High: rowNumber(r, "f15"), Low: rowNumber(r, "f16"), Open: rowNumber(r, "f17"), PrevClose: rowNumber(r, "f18"), TurnoverRate: rowNumber(r, "f8"), PE: rowNumber(r, "f9"), PB: rowNumber(r, "f23")})
	}
	return out, nil
}
func (s *BoardProvider) Kline(ctx context.Context, symbol string, options ...KlineOptions) ([]KlineBar, error) {
	if len(options) > 1 {
		return nil, argument("at most one KlineOptions is allowed")
	}
	var o KlineOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Adjust == "" {
		o.Adjust = AdjustNone
	}
	o, err := historyOptions([]KlineOptions{o})
	if err != nil {
		return nil, err
	}
	code, err := s.code(ctx, symbol)
	if err != nil {
		return nil, err
	}
	_, fqt, _ := adjustCode(o.Adjust)
	period := map[string]string{"daily": "101", "weekly": "102", "monthly": "103"}[o.Period]
	params := emParams("90."+code, period, fqt, compactDate(o.StartDate), compactDate(o.EndDate))
	params.Del("ut")
	params.Set("smplmt", "10000")
	data, err := fetchEM(ctx, s.client, s.endpoints.Kline, params)
	if err != nil {
		return nil, err
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
		b := csvBar(line)
		b.Timestamp = markettime.Parse(b.Date, markettime.CN)
		b.TZ = markettime.CN
		b.Source = "eastmoney"
		b.Adjust = o.Adjust
		b.Code = code
		out = append(out, b)
	}
	return out, nil
}

// BoardMinuteBar retains the board trend's distinct Price field (CSV column 8).
type BoardMinuteBar struct {
	KlineBar
	Price *float64 `json:"price"`
}

func (s *BoardProvider) MinuteKline(ctx context.Context, symbol string, periods ...string) ([]BoardMinuteBar, error) {
	if len(periods) > 1 {
		return nil, argument("at most one minute period is allowed")
	}
	period := "5"
	if len(periods) == 1 {
		period = periods[0]
	}
	switch period {
	case "1", "5", "15", "30", "60":
	default:
		return nil, argument("invalid minute period")
	}
	code, err := s.code(ctx, symbol)
	if err != nil {
		return nil, err
	}
	params := emParams("90."+code, period, "1", "0", "20500101")
	params.Del("ut")
	params.Set("smplmt", "10000")
	endpoint := s.endpoints.Kline
	if period == "1" {
		endpoint = s.endpoints.Trends
		params = url.Values{"fields1": {"f1,f2,f3,f4,f5,f6,f7,f8,f9,f10,f11,f12,f13"}, "fields2": {"f51,f52,f53,f54,f55,f56,f57,f58"}, "ndays": {"1"}, "iscr": {"0"}, "secid": {"90." + code}}
	}
	data, err := fetchEM(ctx, s.client, endpoint, params)
	if err != nil {
		return nil, err
	}
	out := []BoardMinuteBar{}
	if data == nil {
		return out, nil
	}
	rows := data.Klines
	if period == "1" {
		rows = data.Trends
	}
	for _, raw := range rows {
		line, ok := rawString(raw)
		if !ok {
			continue
		}
		b := csvBar(line)
		b.Time = b.Date
		b.Date = ""
		b.TZ = markettime.CN
		b.Timestamp = markettime.Parse(b.Time, markettime.CN)
		b.Source = "eastmoney"
		b.Adjust = AdjustQFQ
		b.Code = code
		var price *float64
		if period == "1" {
			price = b.Amplitude
			b.Amplitude = nil
			b.Change = nil
			b.ChangePercent = nil
			b.TurnoverRate = nil
			b.Adjust = AdjustNone
		}
		out = append(out, BoardMinuteBar{b, price})
	}
	return out, nil
}

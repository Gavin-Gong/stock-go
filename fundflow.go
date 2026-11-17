package stockgo

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/internal/markettime"
	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/symbols"
)

const emDataToken = "b2884a393a59ad64002292a3e90d46a5"
const flowFields = "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61,f62,f63,f64,f65"
const realtimeFlowFields = "f48,f57,f58,f86,f135,f136,f137,f138,f139,f140,f141,f142,f143,f144,f145,f146,f147,f148,f149"
const flowStockFilter = "m:0+t:6+f:!2,m:0+t:13+f:!2,m:0+t:80+f:!2,m:1+t:2+f:!2,m:1+t:23+f:!2,m:0+t:7+f:!2,m:1+t:3+f:!2"

type FundFlowEndpoints struct{ History, Rank, Realtime string }
type FundFlowService struct {
	client    *transport.Client
	endpoints FundFlowEndpoints
}
type FundFlowOptions struct{ Period string }
type FundFlowRankOptions struct {
	Indicator, SectorType string
	PageOptions
}
type RealtimeFundFlow struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Date           string  `json:"date"`
	Timestamp      *int64  `json:"timestamp"`
	TZ             string  `json:"tz"`
	MainInflow     float64 `json:"mainInflow"`
	MainOutflow    float64 `json:"mainOutflow"`
	MainNet        float64 `json:"mainNet"`
	MainNetRatio   float64 `json:"mainNetRatio"`
	RetailInflow   float64 `json:"retailInflow"`
	RetailOutflow  float64 `json:"retailOutflow"`
	RetailNet      float64 `json:"retailNet"`
	RetailNetRatio float64 `json:"retailNetRatio"`
	TotalFlow      float64 `json:"totalFlow"`
}

func (s *QuoteService) FundFlow(ctx context.Context, codes []string) ([]RealtimeFundFlow, error) {
	return s.fundFlow.Realtime(ctx, codes)
}
func (s *FundFlowService) Realtime(ctx context.Context, codes []string) ([]RealtimeFundFlow, error) {
	keys := symbols.TencentBatch(codes, symbols.CN).Keys
	return batchFetch(ctx, keys, BatchOptions{BatchSize: 1, Concurrency: 5}, func(ctx context.Context, keys []string) ([]RealtimeFundFlow, error) {
		ns, err := symbols.Normalize(keys[0], symbols.Hint{Market: symbols.CN})
		if err != nil {
			return nil, err
		}
		secid, err := symbols.ToEastmoney(ns)
		if err != nil {
			return nil, err
		}
		endpoint := endpointQuery(s.endpoints.Realtime, url.Values{"secid": {secid}, "fields": {realtimeFlowFields}, "ut": {emToken}, "invt": {"2"}, "fltt": {"2"}})
		var response struct {
			Data emRow `json:"data"`
		}
		if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
			return nil, err
		}
		if response.Data == nil {
			return []RealtimeFundFlow{}, nil
		}
		return []RealtimeFundFlow{parseRealtimeFlow(response.Data)}, nil
	})
}
func parseRealtimeFlow(r emRow) RealtimeFundFlow {
	number := func(key string) float64 {
		v := rowNumber(r, key)
		if v == nil {
			return 0
		}
		return *v
	}
	turnover := number("f48")
	ratio := func(net float64) float64 {
		if turnover > 0 {
			return net / turnover * 100
		}
		return 0
	}
	out := RealtimeFundFlow{Code: rowText(r, "f57"), Name: rowText(r, "f58"), TZ: markettime.CN, MainInflow: number("f135") / 10000, MainOutflow: number("f136") / 10000, MainNet: number("f137") / 10000, MainNetRatio: ratio(number("f137")), RetailInflow: number("f147") / 10000, RetailOutflow: number("f148") / 10000, RetailNet: number("f149") / 10000, RetailNetRatio: ratio(number("f149")), TotalFlow: turnover / 10000}
	if stamp := rowNumber(r, "f86"); stamp != nil && *stamp > 0 && *stamp < float64(1<<63)/1000 {
		ms := int64(*stamp * 1000)
		out.Timestamp = &ms
		loc, _ := time.LoadLocation(markettime.CN)
		out.Date = time.UnixMilli(ms).In(loc).Format("20060102")
	}
	return out
}
func flowPeriod(options []FundFlowOptions) (string, error) {
	if len(options) > 1 {
		return "", argument("at most one FundFlowOptions is allowed")
	}
	period := "daily"
	if len(options) == 1 && options[0].Period != "" {
		period = options[0].Period
	}
	klt := map[string]string{"daily": "101", "weekly": "102", "monthly": "103"}[period]
	if klt == "" {
		return "", argument("period must be daily, weekly or monthly")
	}
	return klt, nil
}
func (s *FundFlowService) history(ctx context.Context, params url.Values) ([]string, error) {
	endpoint := endpointQuery(s.endpoints.History, params)
	var response struct {
		Data *struct {
			Klines []json.RawMessage `json:"klines"`
		} `json:"data"`
	}
	if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
		return nil, err
	}
	out := []string{}
	if response.Data == nil {
		return out, nil
	}
	for _, raw := range response.Data.Klines {
		if line, ok := rawString(raw); ok {
			out = append(out, line)
		}
	}
	return out, nil
}
func flowParams(secid, klt string) url.Values {
	return url.Values{"lmt": {"0"}, "klt": {klt}, "secid": {secid}, "fields1": {"f1,f2,f3,f7"}, "fields2": {flowFields}, "ut": {emDataToken}}
}
func (s *FundFlowService) Individual(ctx context.Context, code string, options ...FundFlowOptions) ([]StockFundFlowDaily, error) {
	klt, err := flowPeriod(options)
	if err != nil {
		return nil, err
	}
	ns, err := symbols.Normalize(code, symbols.Hint{Market: symbols.CN})
	if err != nil {
		for _, market := range []symbols.Market{symbols.CN, symbols.HK, symbols.US, symbols.Global} {
			if index, e := symbols.Normalize(code, symbols.Hint{Market: market}); e == nil && index.AssetType == symbols.Index {
				return nil, argument("individual fund flow is not available for special index symbols")
			}
		}
		return nil, err
	}
	if ns.AssetType == symbols.Index {
		return nil, argument("individual fund flow is not available for special index symbols")
	}
	secid, err := symbols.ToEastmoney(ns)
	if err != nil {
		return nil, err
	}
	lines, err := s.history(ctx, flowParams(secid, klt))
	if err != nil {
		return nil, err
	}
	out := make([]StockFundFlowDaily, 0, len(lines))
	for _, line := range lines {
		out = append(out, parseStockFlow(line))
	}
	return out, nil
}
func (s *FundFlowService) Market(ctx context.Context) ([]MarketFundFlow, error) {
	params := flowParams("1.000001", "101")
	params.Set("secid2", "0.399001")
	lines, err := s.history(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]MarketFundFlow, 0, len(lines))
	for _, line := range lines {
		out = append(out, parseMarketFlow(line))
	}
	return out, nil
}
func (s *FundFlowService) SectorHistory(ctx context.Context, code string, options ...FundFlowOptions) ([]StockFundFlowDaily, error) {
	klt, err := flowPeriod(options)
	if err != nil {
		return nil, err
	}
	code = strings.TrimPrefix(code, "90.")
	if !boardCodePattern.MatchString(code) {
		return nil, argument("sector history requires a BK board code")
	}
	lines, err := s.history(ctx, flowParams("90."+code, klt))
	if err != nil {
		return nil, err
	}
	out := make([]StockFundFlowDaily, 0, len(lines))
	for _, line := range lines {
		out = append(out, parseStockFlow(line))
	}
	return out, nil
}

// rankConfig preserves each horizon's distinct price-change and inflow fields.
type flowRankConfig struct{ fid, fields, change, main, mainPct, super, superPct, large, largePct, medium, mediumPct, small, smallPct string }

func parseStockFlow(line string) StockFundFlowDaily {
	f := strings.Split(line, ",")
	return StockFundFlowDaily{Date: at(f, 0),
		MainNetInflow:              nullable(at(f, 1)),
		SmallNetInflow:             nullable(at(f, 2)),
		MediumNetInflow:            nullable(at(f, 3)),
		LargeNetInflow:             nullable(at(f, 4)),
		SuperLargeNetInflow:        nullable(at(f, 5)),
		MainNetInflowPercent:       nullable(at(f, 6)),
		SmallNetInflowPercent:      nullable(at(f, 7)),
		MediumNetInflowPercent:     nullable(at(f, 8)),
		LargeNetInflowPercent:      nullable(at(f, 9)),
		SuperLargeNetInflowPercent: nullable(at(f, 10)),
		Close:                      nullable(at(f, 11)),
		ChangePercent:              nullable(at(f, 12)),
	}
}

func parseMarketFlow(line string) MarketFundFlow {
	f := strings.Split(line, ",")
	return MarketFundFlow{Date: at(f, 0),
		MainNetInflow:              nullable(at(f, 1)),
		SmallNetInflow:             nullable(at(f, 2)),
		MediumNetInflow:            nullable(at(f, 3)),
		LargeNetInflow:             nullable(at(f, 4)),
		SuperLargeNetInflow:        nullable(at(f, 5)),
		MainNetInflowPercent:       nullable(at(f, 6)),
		SmallNetInflowPercent:      nullable(at(f, 7)),
		MediumNetInflowPercent:     nullable(at(f, 8)),
		LargeNetInflowPercent:      nullable(at(f, 9)),
		SuperLargeNetInflowPercent: nullable(at(f, 10)),
		ShClose:                    nullable(at(f, 11)),
		ShChangePercent:            nullable(at(f, 12)),
		SzClose:                    nullable(at(f, 13)),
		SzChangePercent:            nullable(at(f, 14)),
	}
}

var flowRankConfigs = map[string]flowRankConfig{
	"today": {"f62", "f12,f14,f2,f3,f62,f184,f66,f69,f72,f75,f78,f81,f84,f87,f124", "f3", "f62", "f184", "f66", "f69", "f72", "f75", "f78", "f81", "f84", "f87"},
	"3day":  {"f267", "f12,f14,f2,f127,f267,f268,f269,f270,f271,f272,f273,f274,f275,f276,f124", "f127", "f267", "f268", "f269", "f270", "f271", "f272", "f273", "f274", "f275", "f276"},
	"5day":  {"f164", "f12,f14,f2,f109,f164,f165,f166,f167,f168,f169,f170,f171,f172,f173,f124", "f109", "f164", "f165", "f166", "f167", "f168", "f169", "f170", "f171", "f172", "f173"},
	"10day": {"f174", "f12,f14,f2,f160,f174,f175,f176,f177,f178,f179,f180,f181,f182,f183,f124", "f160", "f174", "f175", "f176", "f177", "f178", "f179", "f180", "f181", "f182", "f183"},
}

func rankOptions(options []FundFlowRankOptions) (FundFlowRankOptions, flowRankConfig, error) {
	if len(options) > 1 {
		return FundFlowRankOptions{}, flowRankConfig{}, argument("at most one FundFlowRankOptions is allowed")
	}
	var o FundFlowRankOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Indicator == "" {
		o.Indicator = "today"
	}
	c, ok := flowRankConfigs[o.Indicator]
	if !ok {
		return o, c, argument("indicator must be today, 3day, 5day or 10day")
	}
	p, err := pageOptions([]PageOptions{o.PageOptions})
	o.PageOptions = p
	return o, c, err
}
func (s *FundFlowService) Rank(ctx context.Context, options ...FundFlowRankOptions) ([]FundFlowRankItem, error) {
	o, c, err := rankOptions(options)
	if err != nil {
		return nil, err
	}
	rows, err := s.rankRows(ctx, o, c, false)
	if err != nil {
		return nil, err
	}
	out := make([]FundFlowRankItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, parseFlowRank(r, c))
	}
	return out, nil
}
func parseFlowRank(r emRow, c flowRankConfig) FundFlowRankItem {
	return FundFlowRankItem{Code: rowText(r, "f12"), Name: rowText(r, "f14"), Price: rowNumber(r, "f2"), ChangePercent: rowNumber(r, c.change), MainNetInflow: rowNumber(r, c.main), MainNetInflowPercent: rowNumber(r, c.mainPct), SuperLargeNetInflow: rowNumber(r, c.super), SuperLargeNetInflowPercent: rowNumber(r, c.superPct), LargeNetInflow: rowNumber(r, c.large), LargeNetInflowPercent: rowNumber(r, c.largePct), MediumNetInflow: rowNumber(r, c.medium), MediumNetInflowPercent: rowNumber(r, c.mediumPct), SmallNetInflow: rowNumber(r, c.small), SmallNetInflowPercent: rowNumber(r, c.smallPct)}
}
func (s *FundFlowService) SectorRank(ctx context.Context, options ...FundFlowRankOptions) ([]SectorFundFlowItem, error) {
	o, c, err := rankOptions(options)
	if err != nil {
		return nil, err
	}
	rows, err := s.rankRows(ctx, o, c, true)
	if err != nil {
		return nil, err
	}
	out := make([]SectorFundFlowItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, SectorFundFlowItem{Code: rowText(r, "f12"), Name: rowText(r, "f14"), ChangePercent: rowNumber(r, c.change), MainNetInflow: rowNumber(r, c.main), MainNetInflowPercent: rowNumber(r, c.mainPct), SuperLargeNetInflow: rowNumber(r, c.super), LargeNetInflow: rowNumber(r, c.large), MediumNetInflow: rowNumber(r, c.medium), SmallNetInflow: rowNumber(r, c.small), TopStockCode: optionalRowText(r, "f204"), TopStockName: optionalRowText(r, "f205")})
	}
	return out, nil
}
func (s *FundFlowService) rankRows(ctx context.Context, o FundFlowRankOptions, c flowRankConfig, sector bool) ([]emRow, error) {
	filter, fields := flowStockFilter, c.fields
	if sector {
		if o.SectorType == "" {
			o.SectorType = "industry"
		}
		filter = map[string]string{"industry": "m:90+t:2", "concept": "m:90+t:3", "region": "m:90+t:1"}[o.SectorType]
		if filter == "" {
			return nil, argument("sectorType must be industry, concept or region")
		}
		fields += ",f204,f205"
	}
	return clistRows(ctx, s.client, s.endpoints.Rank, url.Values{"fid": {c.fid}, "po": {"1"}, "np": {"1"}, "fltt": {"2"}, "invt": {"2"}, "ut": {emDataToken}, "fs": {filter}, "fields": {fields}}, o.PageOptions, true)
}

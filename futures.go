package stockgo

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

const futuresSpotToken = "58b2fa8f54638b60b87d69b31969089c"

type FuturesEndpoints struct{ Kline, GlobalSpot string }
type FuturesKlineOptions struct{ Period, StartDate, EndDate string }
type GlobalFuturesKlineOptions struct {
	Period, StartDate, EndDate string
	MarketCode                 *int
}
type GlobalFuturesSpotOptions struct{ PageSize, MaxPages int }
type FuturesInventoryOptions struct {
	StartDate string
	PageSize  int
}
type ComexInventoryOptions struct{ PageSize int }
type FuturesService struct {
	client    *transport.Client
	dc        *DatacenterService
	endpoints FuturesEndpoints
}

var contractPattern = regexp.MustCompile(`^[A-Za-z]+[A-Za-z0-9]*$`)
var varietyPrefix = regexp.MustCompile(`^[A-Za-z]+`)
var globalVarietyPrefix = regexp.MustCompile(`^[A-Z]+`)

// FuturesMarketCode resolves a domestic variety, including uppercase main-contract M suffixes.
func FuturesMarketCode(variety string) (int, error) {
	lookup := func(v string) string {
		for _, key := range []string{v, strings.ToLower(v), strings.ToUpper(v)} {
			if ex := futuresVarietyExchange[key]; ex != "" {
				return ex
			}
		}
		return ""
	}
	ex := lookup(variety)
	if ex == "" && len(variety) > 1 && strings.HasSuffix(variety, "M") {
		ex = lookup(variety[:len(variety)-1])
	}
	if ex == "" {
		return 0, argument("unknown domestic futures variety: " + variety)
	}
	return futuresExchangeCodes[ex], nil
}
func futuresHistoryOptions(o FuturesKlineOptions) (FuturesKlineOptions, error) {
	parsed, err := historyOptions([]KlineOptions{{Period: o.Period, StartDate: o.StartDate, EndDate: o.EndDate, Adjust: AdjustNone}})
	if err != nil {
		return o, err
	}
	return FuturesKlineOptions{Period: parsed.Period, StartDate: compactDate(parsed.StartDate), EndDate: compactDate(parsed.EndDate)}, nil
}
func (s *FuturesService) Kline(ctx context.Context, symbol string, options ...FuturesKlineOptions) ([]FuturesKline, error) {
	if len(options) > 1 {
		return nil, argument("at most one FuturesKlineOptions is allowed")
	}
	var o FuturesKlineOptions
	if len(options) == 1 {
		o = options[0]
	}
	if !contractPattern.MatchString(symbol) {
		return nil, argument("invalid futures contract")
	}
	market, err := FuturesMarketCode(varietyPrefix.FindString(symbol))
	if err != nil {
		return nil, err
	}
	o, err = futuresHistoryOptions(o)
	if err != nil {
		return nil, err
	}
	return s.kline(ctx, symbol, market, o)
}
func (s *FuturesService) GlobalKline(ctx context.Context, symbol string, options ...GlobalFuturesKlineOptions) ([]FuturesKline, error) {
	if len(options) > 1 {
		return nil, argument("at most one GlobalFuturesKlineOptions is allowed")
	}
	var o GlobalFuturesKlineOptions
	if len(options) == 1 {
		o = options[0]
	}
	if !contractPattern.MatchString(symbol) {
		return nil, argument("invalid global futures contract")
	}
	market := 0
	if o.MarketCode != nil {
		market = *o.MarketCode
		if market < 0 {
			return nil, argument("marketCode must be nonnegative")
		}
	} else {
		var ok bool
		market, ok = globalFuturesMarkets[globalVarietyPrefix.FindString(symbol)]
		if !ok {
			return nil, argument("unknown global futures variety; specify marketCode explicitly")
		}
	}
	parsed, err := futuresHistoryOptions(FuturesKlineOptions{Period: o.Period, StartDate: o.StartDate, EndDate: o.EndDate})
	if err != nil {
		return nil, err
	}
	return s.kline(ctx, symbol, market, parsed)
}
func (s *FuturesService) kline(ctx context.Context, symbol string, market int, o FuturesKlineOptions) ([]FuturesKline, error) {
	params := url.Values{"fields1": {"f1,f2,f3,f4,f5,f6"}, "fields2": {"f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61,f62,f63,f64"}, "ut": {emToken}, "klt": {map[string]string{"daily": "101", "weekly": "102", "monthly": "103"}[o.Period]}, "fqt": {"0"}, "secid": {numberString(market) + "." + symbol}, "beg": {o.StartDate}, "end": {o.EndDate}}
	data, err := fetchEM(ctx, s.client, s.endpoints.Kline, params)
	if err != nil {
		return nil, err
	}
	out := []FuturesKline{}
	if data == nil {
		return out, nil
	}
	code := data.Code
	if code == "" {
		code = symbol
	}
	for _, raw := range data.Klines {
		line, ok := rawString(raw)
		if !ok {
			continue
		}
		b := csvBar(line)
		parts := strings.Split(line, ",")
		out = append(out, FuturesKline{Date: b.Date, Code: code, Name: data.Name, Open: b.Open, Close: b.Close, High: b.High, Low: b.Low, Volume: b.Volume, Amount: b.Amount, Amplitude: b.Amplitude, ChangePercent: b.ChangePercent, Change: b.Change, TurnoverRate: b.TurnoverRate, OpenInterest: nullable(at(parts, 12))})
	}
	return out, nil
}
func mapGlobalFutures(r emRow) GlobalFuturesQuote {
	return GlobalFuturesQuote{Code: rowText(r, "dm"), Name: rowText(r, "name"), Price: rowNumber(r, "p"), Change: rowNumber(r, "zde"), ChangePercent: rowNumber(r, "zdf"), Open: rowNumber(r, "o"), High: rowNumber(r, "h"), Low: rowNumber(r, "l"), PrevSettle: rowNumber(r, "zjsj"), Volume: rowNumber(r, "vol"), BuyVolume: rowNumber(r, "wp"), SellVolume: rowNumber(r, "np"), OpenInterest: rowNumber(r, "ccl")}
}
func (s *FuturesService) GlobalSpot(ctx context.Context, options ...GlobalFuturesSpotOptions) ([]GlobalFuturesQuote, error) {
	if len(options) > 1 {
		return nil, argument("at most one GlobalFuturesSpotOptions is allowed")
	}
	var o GlobalFuturesSpotOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.PageSize < 0 || o.PageSize > 20000 || o.MaxPages < 0 {
		return nil, argument("invalid global futures paging options")
	}
	if o.PageSize == 0 {
		o.PageSize = 20
	}
	if o.MaxPages == 0 {
		o.MaxPages = 1000
	}
	out := []GlobalFuturesQuote{}
	var total *float64
	for page := 0; page < o.MaxPages; page++ {
		endpoint := endpointQuery(s.endpoints.GlobalSpot, url.Values{"orderBy": {"dm"}, "sort": {"desc"}, "pageSize": {numberString(o.PageSize)}, "pageIndex": {numberString(page)}, "token": {futuresSpotToken}, "field": {"dm,sc,name,p,zsjd,zde,zdf,f152,o,h,l,zjsj,vol,wp,np,ccl"}, "blockName": {"callback"}})
		var response emRow
		if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
			return nil, err
		}
		if page == 0 {
			total = rowNumber(response, "total")
		}
		rows := arrayRows(response["list"])
		for _, r := range rows {
			out = append(out, mapGlobalFutures(r))
		}
		if len(rows) == 0 || total == nil || float64(len(out)) >= *total {
			return out, nil
		}
	}
	return nil, klineError(sdkerrors.UpstreamError, "eastmoney", s.endpoints.GlobalSpot, "global futures pagination exceeded maxPages", nil)
}
func (s *FuturesService) InventorySymbols(ctx context.Context) ([]FuturesInventorySymbol, error) {
	return mapReport(ctx, s.dc, DatacenterQuery{ReportName: "RPT_FUTU_POSITIONCODE", Columns: "TRADE_MARKET_CODE,TRADE_CODE,TRADE_TYPE", Filter: `(IS_MAINCODE="1")`, PageSize: 500, FetchAllPages: Bool(false)}, func(r emRow) FuturesInventorySymbol {
		return FuturesInventorySymbol{Code: rowText(r, "TRADE_CODE"), Name: rowText(r, "TRADE_TYPE"), MarketCode: rowText(r, "TRADE_MARKET_CODE")}
	})
}
func (s *FuturesService) Inventory(ctx context.Context, symbol string, options ...FuturesInventoryOptions) ([]FuturesInventory, error) {
	if len(options) > 1 {
		return nil, argument("at most one FuturesInventoryOptions is allowed")
	}
	var o FuturesInventoryOptions
	if len(options) == 1 {
		o = options[0]
	}
	if !contractPattern.MatchString(symbol) {
		return nil, argument("invalid inventory variety")
	}
	if o.StartDate == "" {
		o.StartDate = "2020-10-28"
	}
	date, err := dateISO(o.StartDate)
	if err != nil {
		return nil, err
	}
	return mapReport(ctx, s.dc, DatacenterQuery{ReportName: "RPT_FUTU_STOCKDATA", Columns: "SECURITY_CODE,TRADE_DATE,ON_WARRANT_NUM,ADDCHANGE", Filter: `(SECURITY_CODE="` + strings.ToUpper(symbol) + `")(TRADE_DATE>='` + date + `')`, PageSize: o.PageSize, SortColumns: "TRADE_DATE", SortTypes: "-1"}, func(r emRow) FuturesInventory {
		code := symbol
		if nullableText(r, "SECURITY_CODE") != nil {
			code = rowText(r, "SECURITY_CODE")
		}
		return FuturesInventory{Code: code, Date: dcDate(rowText(r, "TRADE_DATE")), Inventory: rowNumber(r, "ON_WARRANT_NUM"), Change: rowNumber(r, "ADDCHANGE")}
	})
}
func (s *FuturesService) ComexInventory(ctx context.Context, symbol string, options ...ComexInventoryOptions) ([]ComexInventory, error) {
	if len(options) > 1 {
		return nil, argument("at most one ComexInventoryOptions is allowed")
	}
	var o ComexInventoryOptions
	if len(options) == 1 {
		o = options[0]
	}
	id := map[string]string{"gold": "EMI00069026", "silver": "EMI00069027"}[symbol]
	if id == "" {
		return nil, argument("COMEX inventory symbol must be gold or silver")
	}
	return mapReport(ctx, s.dc, DatacenterQuery{ReportName: "RPT_FUTUOPT_GOLDSIL", SortColumns: "REPORT_DATE", SortTypes: "-1", PageSize: o.PageSize, Filter: `(INDICATOR_ID1="` + id + `")(@STORAGE_TON<>"NULL")`}, func(r emRow) ComexInventory {
		return ComexInventory{Date: dcDate(rowText(r, "REPORT_DATE")), Name: map[string]string{"gold": "黄金", "silver": "白银"}[symbol], StorageTon: rowNumber(r, "STORAGE_TON"), StorageOunce: rowNumber(r, "STORAGE_OUNCE")}
	})
}

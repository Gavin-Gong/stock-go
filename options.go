package stockgo

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/Gavin-Gong/stock-go/internal/jsonp"
	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type IndexOptionProduct string

const (
	OptionHO IndexOptionProduct = "ho"
	OptionIO IndexOptionProduct = "io"
	OptionMO IndexOptionProduct = "mo"
)

type ETFOptionCate string

const (
	ETF50          ETFOptionCate = "50ETF"
	ETF300         ETFOptionCate = "300ETF"
	ETF500         ETFOptionCate = "500ETF"
	ETFStar50      ETFOptionCate = "科创50"
	ETFStarBoard50 ETFOptionCate = "科创板50"
)

type OptionEndpoints struct{ Spot, Dayline, ETFMonths, ETFExpire, ETFMinute, ETFDaily, ETF5Day, CFFEX, LHB string }
type CFFEXOptionQuotesOptions struct{ PageSize int }
type OptionService struct {
	client    *transport.Client
	endpoints OptionEndpoints
}

const optionDataToken = "b2884a393a59ad64002292a3e90d46a5"

func (s *OptionService) sina(ctx context.Context, base string, params url.Values, path bool) (json.RawMessage, error) {
	const callback = "__stock_go_jsonp"
	if path {
		base = strings.ReplaceAll(base, "{callback}", callback)
	} else {
		params = cloneValues(params)
		params.Set("callback", callback)
	}
	endpoint := endpointQuery(base, params)
	var payload json.RawMessage
	_, err := s.client.GetParsed(ctx, "sina", endpoint, nil, func(body []byte) error { var e error; payload, e = jsonp.Extract(body); return e })
	if err != nil {
		return nil, err
	}

	var response struct {
		Result *struct {
			Status *struct {
				Code int
				Msg  string
			} `json:"status"`
		} `json:"result"`
	}
	if json.Unmarshal(payload, &response) == nil && response.Result != nil && response.Result.Status != nil && response.Result.Status.Code != 0 {
		return nil, klineError(sdkerrors.UpstreamError, "sina", endpoint, response.Result.Status.Msg, nil)
	}
	return payload, nil
}
func sinaResult(raw json.RawMessage) json.RawMessage {
	var root struct {
		Result struct {
			Data json.RawMessage `json:"data"`
		} `json:"result"`
	}
	_ = json.Unmarshal(raw, &root)
	return root.Result.Data
}
func parseOptionTQuote(rows json.RawMessage, put bool) []OptionTQuote {
	var list [][]json.RawMessage
	_ = json.Unmarshal(rows, &list)
	out := []OptionTQuote{}
	for _, row := range list {
		get := func(i int) json.RawMessage {
			if i < len(row) {
				return row[i]
			}
			return nil
		}
		strike := rawNumber(get(7))
		symbolIndex := 8
		if put {
			strike = nil
			symbolIndex = 7
		}
		out = append(out, OptionTQuote{Symbol: rowText(emRow{"v": get(symbolIndex)}, "v"), BuyVolume: rawNumber(get(0)), BuyPrice: rawNumber(get(1)), Price: rawNumber(get(2)), AskPrice: rawNumber(get(3)), AskVolume: rawNumber(get(4)), OpenInterest: rawNumber(get(5)), Change: rawNumber(get(6)), StrikePrice: strike})
	}
	return out
}
func (s *OptionService) tQuote(ctx context.Context, product, exchange, contract string) (*OptionTQuoteResult, error) {
	if !contractPattern.MatchString(contract) {
		return nil, argument("invalid option contract")
	}
	raw, err := s.sina(ctx, s.endpoints.Spot, url.Values{"type": {"futures"}, "product": {product}, "exchange": {exchange}, "pinzhong": {contract}}, false)
	if err != nil {
		return nil, err
	}
	var data emRow
	_ = json.Unmarshal(sinaResult(raw), &data)
	return &OptionTQuoteResult{Calls: parseOptionTQuote(data["up"], false), Puts: parseOptionTQuote(data["down"], true)}, nil
}
func (s *OptionService) IndexSpot(ctx context.Context, product IndexOptionProduct, contract string) (*OptionTQuoteResult, error) {
	if product != OptionHO && product != OptionIO && product != OptionMO {
		return nil, argument("index option product must be ho, io or mo")
	}
	return s.tQuote(ctx, string(product), "cffex", contract)
}
func (s *OptionService) CommoditySpot(ctx context.Context, variety, contract string) (*OptionTQuoteResult, error) {
	mapping, ok := commodityOptionMarkets[variety]
	if !ok {
		return nil, argument("unknown commodity option variety: " + variety)
	}
	return s.tQuote(ctx, mapping.Product, mapping.Exchange, contract)
}
func mapOptionKline(raw json.RawMessage) []OptionKline {
	out := []OptionKline{}
	for _, r := range arrayRows(raw) {
		out = append(out, OptionKline{Date: rowText(r, "d"), Open: rowNumber(r, "o"), High: rowNumber(r, "h"), Low: rowNumber(r, "l"), Close: rowNumber(r, "c"), Volume: rowNumber(r, "v")})
	}
	return out
}
func (s *OptionService) daily(ctx context.Context, base, symbol string) ([]OptionKline, error) {
	raw, err := s.sina(ctx, base, url.Values{"symbol": {symbol}}, true)
	if err != nil {
		return nil, err
	}
	return mapOptionKline(raw), nil
}
func (s *OptionService) IndexKline(ctx context.Context, symbol string) ([]OptionKline, error) {
	if !contractPattern.MatchString(symbol) {
		return nil, argument("invalid option symbol")
	}
	return s.daily(ctx, s.endpoints.Dayline, symbol)
}
func (s *OptionService) CommodityKline(ctx context.Context, symbol string) ([]OptionKline, error) {
	return s.IndexKline(ctx, symbol)
}
func validateETFCate(cate ETFOptionCate) error {
	switch cate {
	case ETF50, ETF300, ETF500, ETFStar50, ETFStarBoard50:
		return nil
	}
	return argument("invalid ETF option category")
}
func (s *OptionService) ETFMonths(ctx context.Context, cate ETFOptionCate) (*ETFOptionMonth, error) {
	if err := validateETFCate(cate); err != nil {
		return nil, err
	}
	raw, err := s.sina(ctx, s.endpoints.ETFMonths, url.Values{"exchange": {"null"}, "cate": {string(cate)}}, false)
	if err != nil {
		return nil, err
	}
	var data struct {
		ContractMonth []string
		StockID       string
		CateID        string
		CateList      []string
	}
	_ = json.Unmarshal(sinaResult(raw), &data)
	months := data.ContractMonth
	if len(months) > 1 {
		months = months[1:]
	}
	return &ETFOptionMonth{Months: append([]string{}, months...), StockId: data.StockID, CateId: data.CateID, CateList: append([]string{}, data.CateList...)}, nil
}
func (s *OptionService) ETFExpireDay(ctx context.Context, cate ETFOptionCate, month string) (*ETFOptionExpireDay, error) {
	if err := validateETFCate(cate); err != nil {
		return nil, err
	}
	if len(month) != 7 {
		return nil, argument("month must be YYYY-MM")
	}
	if _, err := dateISO(month + "-01"); err != nil {
		return nil, err
	}
	query := url.Values{"exchange": {"null"}, "cate": {string(cate)}, "date": {month}}
	raw, err := s.sina(ctx, s.endpoints.ETFExpire, query, false)
	if err != nil {
		return nil, err
	}
	var data emRow
	_ = json.Unmarshal(sinaResult(raw), &data)
	if days := numericValue(data["remainderDays"]); days != nil && *days < 0 {
		query.Set("cate", "XD"+string(cate))
		raw, err = s.sina(ctx, s.endpoints.ETFExpire, query, false)
		if err != nil {
			return nil, err
		}
		data = nil
		_ = json.Unmarshal(sinaResult(raw), &data)
	}
	days := 0.0
	if v := rowNumber(data, "remainderDays"); v != nil {
		days = *v
	}
	var other emRow
	_ = json.Unmarshal(data["other"], &other)
	return &ETFOptionExpireDay{ExpireDay: rowText(data, "expireDay"), RemainderDays: days, StockId: rowText(data, "stockId"), Name: rowText(other, "name")}, nil
}

// numericValue retains JavaScript's typeof-number distinction for the XD retry.
func numericValue(raw json.RawMessage) *float64 {
	if _, ok := rawString(raw); ok {
		return nil
	}
	return strictNumber(raw)
}
func validateETFCode(code string) error {
	if len(code) != 8 || !digitsOnly(code) {
		return argument("ETF option code must contain eight digits")
	}
	return nil
}
func parseOptionMinutes(raw json.RawMessage) []OptionMinute {
	out := []OptionMinute{}
	date := ""
	for _, r := range arrayRows(raw) {
		if d := rowText(r, "d"); d != "" {
			date = d
		}
		out = append(out, OptionMinute{Time: rowText(r, "i"), Date: date, Price: rowNumber(r, "p"), Volume: rowNumber(r, "v"), OpenInterest: rowNumber(r, "t"), AvgPrice: rowNumber(r, "a")})
	}
	return out
}
func (s *OptionService) ETFMinute(ctx context.Context, code string) ([]OptionMinute, error) {
	if err := validateETFCode(code); err != nil {
		return nil, err
	}
	raw, err := s.sina(ctx, s.endpoints.ETFMinute, url.Values{"symbol": {"CON_OP_" + code}}, false)
	if err != nil {
		return nil, err
	}
	return parseOptionMinutes(sinaResult(raw)), nil
}
func (s *OptionService) ETFDailyKline(ctx context.Context, code string) ([]OptionKline, error) {
	if err := validateETFCode(code); err != nil {
		return nil, err
	}
	return s.daily(ctx, s.endpoints.ETFDaily, "CON_OP_"+code)
}
func (s *OptionService) ETF5DayMinute(ctx context.Context, code string) ([]OptionMinute, error) {
	if err := validateETFCode(code); err != nil {
		return nil, err
	}
	raw, err := s.sina(ctx, s.endpoints.ETF5Day, url.Values{"symbol": {"CON_OP_" + code}}, false)
	if err != nil {
		return nil, err
	}
	var days []json.RawMessage
	_ = json.Unmarshal(sinaResult(raw), &days)
	out := []OptionMinute{}
	for _, day := range days {
		out = append(out, parseOptionMinutes(day)...)
	}
	return out, nil
}
func mapCFFEX(r emRow) CFFEXOptionQuote {
	return CFFEXOptionQuote{Code: rowText(r, "dm"), Name: rowText(r, "name"), Price: rowNumber(r, "p"), Change: rowNumber(r, "zde"), ChangePercent: rowNumber(r, "zdf"), Volume: rowNumber(r, "vol"), Amount: rowNumber(r, "cje"), OpenInterest: rowNumber(r, "ccl"), StrikePrice: rowNumber(r, "xqj"), RemainDays: rowNumber(r, "syr"), DailyChange: rowNumber(r, "rz"), PrevSettle: rowNumber(r, "zjsj"), Open: rowNumber(r, "o")}
}

// CFFEXQuotes returns one page, matching the source API (default 20000 contracts).
func (s *OptionService) CFFEXQuotes(ctx context.Context, options ...CFFEXOptionQuotesOptions) ([]CFFEXOptionQuote, error) {
	if len(options) > 1 {
		return nil, argument("at most one CFFEXOptionQuotesOptions is allowed")
	}
	size := 20000
	if len(options) == 1 && options[0].PageSize != 0 {
		size = options[0].PageSize
	}
	if size < 1 || size > 20000 {
		return nil, argument("CFFEX pageSize must be 1..20000")
	}
	endpoint := endpointQuery(s.endpoints.CFFEX, url.Values{"orderBy": {"zdf"}, "sort": {"desc"}, "pageSize": {numberString(size)}, "pageIndex": {"0"}, "token": {futuresSpotToken}, "field": {"dm,sc,name,p,zsjd,zde,zdf,f152,vol,cje,ccl,xqj,syr,rz,zjsj,o"}})
	var response emRow
	if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
		return nil, err
	}
	out := []CFFEXOptionQuote{}
	for _, r := range arrayRows(response["list"]) {
		out = append(out, mapCFFEX(r))
	}
	return out, nil
}
func mapOptionLHB(r emRow) OptionLHBItem {
	rank := 0.0
	if v := rowNumber(r, "MEMBER_RANK"); v != nil {
		rank = *v
	}
	return OptionLHBItem{TradeType: rowText(r, "TRADE_TYPE"), Date: dcDate(rowText(r, "TRADE_DATE")), Symbol: rowText(r, "SECURITY_CODE"), TargetName: rowText(r, "TARGET_NAME"), Rank: rank, MemberName: rowText(r, "MEMBER_NAME_ABBR"), SellVolume: rowNumber(r, "SELL_VOLUME"), SellVolumeChange: rowNumber(r, "SELL_VOLUME_CHANGE"), NetSellVolume: rowNumber(r, "NET_SELL_VOLUME"), SellVolumeRatio: rowNumber(r, "SELL_VOLUME_RATIO"), BuyVolume: rowNumber(r, "BUY_VOLUME"), BuyVolumeChange: rowNumber(r, "BUY_VOLUME_CHANGE"), NetBuyVolume: rowNumber(r, "NET_BUY_VOLUME"), BuyVolumeRatio: rowNumber(r, "BUY_VOLUME_RATIO"), SellPosition: rowNumber(r, "SELL_POSITION"), SellPositionChange: rowNumber(r, "SELL_POSITION_CHANGE"), NetSellPosition: rowNumber(r, "NET_SELL_POSITION"), SellPositionRatio: rowNumber(r, "SELL_POSITION_RATIO"), BuyPosition: rowNumber(r, "BUY_POSITION"), BuyPositionChange: rowNumber(r, "BUY_POSITION_CHANGE"), NetBuyPosition: rowNumber(r, "NET_BUY_POSITION"), BuyPositionRatio: rowNumber(r, "BUY_POSITION_RATIO")}
}
func (s *OptionService) LHB(ctx context.Context, symbol, date string) ([]OptionLHBItem, error) {
	code, err := pureCNCode(symbol)
	if err != nil {
		return nil, err
	}
	date, err = dateISO(date)
	if err != nil {
		return nil, err
	}
	endpoint := endpointQuery(s.endpoints.LHB, url.Values{"type": {"RPT_IF_BILLBOARD_TD"}, "sty": {"ALL"}, "p": {"1"}, "ps": {"200"}, "source": {"IFBILLBOARD"}, "client": {"WEB"}, "ut": {optionDataToken}, "filter": {`(SECURITY_CODE="` + code + `")(TRADE_DATE='` + date + `')`}})
	var response struct {
		Success *bool
		Message string
		Result  *struct{ Data []emRow }
	}
	if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
		return nil, err
	}
	if response.Success != nil && !*response.Success {
		return nil, klineError(sdkerrors.UpstreamError, "eastmoney", endpoint, response.Message, nil)
	}
	out := []OptionLHBItem{}
	if response.Result != nil {
		for _, r := range response.Result.Data {
			out = append(out, mapOptionLHB(r))
		}
	}
	return out, nil
}

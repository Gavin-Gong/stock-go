package stockgo

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/Gavin-Gong/stock-go/internal/markettime"
	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"github.com/Gavin-Gong/stock-go/symbols"
)

type ZTPoolType string

const (
	PoolLimitUp   ZTPoolType = "zt"
	PoolYesterday ZTPoolType = "yesterday"
	PoolStrong    ZTPoolType = "strong"
	PoolSubNew    ZTPoolType = "sub_new"
	PoolBroken    ZTPoolType = "broken"
	PoolLimitDown ZTPoolType = "dt"
)

type ZTPoolOptions struct {
	Type ZTPoolType
	Date string
}
type StockChangeType string

const (
	RocketLaunch   StockChangeType = "rocket_launch"
	QuickRebound   StockChangeType = "quick_rebound"
	LargeBuy       StockChangeType = "large_buy"
	LimitUpSeal    StockChangeType = "limit_up_seal"
	LimitDownOpen  StockChangeType = "limit_down_open"
	BigBuyOrder    StockChangeType = "big_buy_order"
	AuctionUp      StockChangeType = "auction_up"
	HighOpen5d     StockChangeType = "high_open_5d"
	GapUp          StockChangeType = "gap_up"
	High60d        StockChangeType = "high_60d"
	Surge60d       StockChangeType = "surge_60d"
	AccelerateDown StockChangeType = "accelerate_down"
	HighDive       StockChangeType = "high_dive"
	LargeSell      StockChangeType = "large_sell"
	LimitDownSeal  StockChangeType = "limit_down_seal"
	LimitUpOpen    StockChangeType = "limit_up_open"
	BigSellOrder   StockChangeType = "big_sell_order"
	AuctionDown    StockChangeType = "auction_down"
	LowOpen5d      StockChangeType = "low_open_5d"
	GapDown        StockChangeType = "gap_down"
	Low60d         StockChangeType = "low_60d"
	Drop60d        StockChangeType = "drop_60d"
	AllChanges     StockChangeType = "all"
	UnknownChange  StockChangeType = "unknown"
)

var changeDefinitions = []struct {
	Type        StockChangeType
	Code, Label string
}{
	{RocketLaunch, "8201", "火箭发射"}, {QuickRebound, "8202", "快速反弹"}, {LargeBuy, "8193", "大笔买入"}, {LimitUpSeal, "4", "封涨停板"}, {LimitDownOpen, "32", "打开跌停板"}, {BigBuyOrder, "64", "有大买盘"}, {AuctionUp, "8207", "竞价上涨"}, {HighOpen5d, "8209", "高开5日线"}, {GapUp, "8211", "向上缺口"}, {High60d, "8213", "60日新高"}, {Surge60d, "8215", "60日大幅上涨"}, {AccelerateDown, "8204", "加速下跌"}, {HighDive, "8203", "高台跳水"}, {LargeSell, "8194", "大笔卖出"}, {LimitDownSeal, "8", "封跌停板"}, {LimitUpOpen, "16", "打开涨停板"}, {BigSellOrder, "128", "有大卖盘"}, {AuctionDown, "8208", "竞价下跌"}, {LowOpen5d, "8210", "低开5日线"}, {GapDown, "8212", "向下缺口"}, {Low60d, "8214", "60日新低"}, {Drop60d, "8216", "60日大幅下跌"},
}

func changeIdentity(code string) (StockChangeType, string) {
	for _, d := range changeDefinitions {
		if d.Code == code {
			return d.Type, d.Label
		}
	}
	return UnknownChange, ""
}

// A nil Types selects large_buy; []StockChangeType{AllChanges} selects all 22 types.
// Page or PageSize selects one page; otherwise automatic paging uses up to MaxPages (default 10).
type StockChangesOptions struct {
	Types                    []StockChangeType
	Page, PageSize, MaxPages int
}
type UnusualFluctuationOptions struct {
	Date, StartDate, EndDate string
	Triggered                *bool
}
type ChangesCoverage struct {
	From          string  `json:"from"`
	To            string  `json:"to"`
	AvailableFrom *string `json:"availableFrom"`
}
type IndividualChangesHistory struct {
	Code          string                     `json:"code"`
	Name          string                     `json:"name"`
	RequestedDays int                        `json:"requestedDays"`
	Coverage      ChangesCoverage            `json:"coverage"`
	Days          []IndividualChangesDay     `json:"days"`
	Stats         map[string]ChangeTypeCount `json:"stats"`
}
type MarketEventService struct {
	client *transport.Client
	dc     *DatacenterService
	codes  *CodeService
	base   string
}

func formatTopicTime(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	text := rowText(emRow{"v": raw}, "v")
	if len(text) < 6 {
		text = strings.Repeat("0", 6-len(text)) + text
	}
	return text[:2] + ":" + text[2:4] + ":" + text[4:6]
}
func topicPrice(raw json.RawMessage) *float64 {
	v := rawNumber(raw)
	if v != nil {
		*v /= 1000
	}
	return v
}
func mapZTPool(r emRow) ZTPoolItem {
	var first, last *string
	if nullableText(r, "fbt") != nil {
		s := formatTopicTime(r["fbt"])
		first = &s
	}
	if nullableText(r, "lbt") != nil {
		s := formatTopicTime(r["lbt"])
		last = &s
	}
	stat := ""
	var obj emRow
	if json.Unmarshal(r["zttj"], &obj) == nil && obj != nil {
		stat = rowText(obj, "days") + "/" + rowText(obj, "ct")
	}
	return ZTPoolItem{Code: rowTextDefault(r, "c", "m"), Name: rowText(r, "n"), Price: topicPrice(r["p"]), ChangePercent: rowNumber(r, "zdp"), LimitPrice: topicPrice(r["tp"]), Amount: rawNumber(rawDefault(r, "amount", "zb")), FloatMarketValue: rowNumber(r, "ltsz"), TotalMarketValue: rowNumber(r, "tshare"), TurnoverRate: rowNumber(r, "hs"), ContinuousBoardCount: rowNumber(r, "lbc"), FirstBoardTime: first, LastBoardTime: last, BoardAmount: rowNumber(r, "fund"), SealAmount: rowNumber(r, "fund"), FailedCount: rowNumber(r, "zbc"), Industry: rowText(r, "hybk"), ZtStatistics: stat, Amplitude: rowNumber(r, "zf"), Speed: rowNumber(r, "zs")}
}

// topicData preserves data:null as absence and reports explicit upstream rc errors.
func (s *MarketEventService) topicData(ctx context.Context, path string, params url.Values, rawTypes string) (emRow, error) {
	endpoint := endpointQuery(appendURLPath(s.base, path), params)
	if rawTypes != "" {
		endpoint = strings.Replace(endpoint, "type="+url.QueryEscape(rawTypes), "type="+rawTypes, 1)
	}
	var response struct {
		RC   int   `json:"rc"`
		Data emRow `json:"data"`
	}
	if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
		return nil, err
	}
	if response.RC != 0 {
		return nil, klineError(sdkerrors.UpstreamError, "eastmoney", endpoint, "topic upstream error (rc="+numberString(response.RC)+")", nil)
	}
	return response.Data, nil
}
func (s *MarketEventService) ZTPool(ctx context.Context, options ...ZTPoolOptions) ([]ZTPoolItem, error) {
	if len(options) > 1 {
		return nil, argument("at most one ZTPoolOptions is allowed")
	}
	var o ZTPoolOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Type == "" {
		o.Type = PoolLimitUp
	}
	config := map[ZTPoolType][2]string{PoolLimitUp: {"getTopicZTPool", "fbt:asc"}, PoolYesterday: {"getYesterdayZTPool", "zs:desc"}, PoolStrong: {"getTopicQSPool", "zdp:desc"}, PoolSubNew: {"getTopicCXPool", "ods:asc"}, PoolBroken: {"getTopicZBPool", "fbt:asc"}, PoolLimitDown: {"getTopicDTPool", "fund:asc"}}[o.Type]
	if config[0] == "" {
		return nil, argument("invalid ZTPool type")
	}
	date, err := calendarDate([]string{o.Date})
	if err != nil {
		return nil, err
	}
	data, err := s.topicData(ctx, config[0], url.Values{"ut": {emToken}, "dpt": {"wz.ztzt"}, "Pageindex": {"0"}, "pagesize": {"10000"}, "sort": {config[1]}, "date": {compactDate(date)}}, "")
	if err != nil {
		return nil, err
	}
	out := []ZTPoolItem{}
	for _, r := range arrayRows(data["pool"]) {
		out = append(out, mapZTPool(r))
	}
	return out, nil
}
func (s *MarketEventService) StockChanges(ctx context.Context, options ...StockChangesOptions) ([]StockChangeItem, error) {
	if len(options) > 1 {
		return nil, argument("at most one StockChangesOptions is allowed")
	}
	var o StockChangesOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Page < 0 || o.PageSize < 0 || o.PageSize > 5000 || o.MaxPages < 0 {
		return nil, argument("invalid stock changes paging options")
	}
	types := o.Types
	if types == nil {
		types = []StockChangeType{LargeBuy}
	}
	if len(types) == 1 && types[0] == AllChanges {
		types = []StockChangeType{}
		for _, d := range changeDefinitions {
			types = append(types, d.Type)
		}
	}
	if len(types) == 0 {
		return nil, argument("stock change types must not be empty")
	}
	codes := []string{}
	for _, t := range types {
		code := ""
		for _, d := range changeDefinitions {
			if t == d.Type {
				code = d.Code
				break
			}
		}
		if code == "" {
			return nil, argument("invalid stock change type: " + string(t))
		}
		codes = append(codes, code)
	}
	joined := strings.Join(codes, ",")
	fallback := ""
	if len(codes) == 1 {
		fallback = codes[0]
	}
	single := o.Page != 0 || o.PageSize != 0
	page, size := 0, 5000
	if single {
		page = max(1, o.Page) - 1
		size = o.PageSize
		if size == 0 {
			size = 100
		}
	}
	if o.MaxPages == 0 {
		o.MaxPages = 10
	}
	out := []StockChangeItem{}
	var total *float64
	for n := 0; n < o.MaxPages; n++ {
		data, err := s.topicData(ctx, "getAllStockChanges", url.Values{"type": {joined}, "ut": {emToken}, "dpt": {"wzchanges"}, "pageindex": {numberString(page + n)}, "pagesize": {numberString(size)}}, joined)
		if err != nil {
			return nil, err
		}
		rows := arrayRows(data["allstock"])
		if tc := rowNumber(data, "tc"); tc != nil {
			total = tc
		}
		for _, r := range rows {
			code := rowText(r, "t")
			if nullableText(r, "t") == nil {
				code = fallback
			}
			kind, label := changeIdentity(code)
			out = append(out, StockChangeItem{Time: formatTopicTime(r["tm"]), Code: rowText(r, "c"), Name: rowText(r, "n"), TypeCode: code, ChangeType: kind, ChangeTypeLabel: label, Info: rowText(r, "i")})
		}
		if single || len(rows) < size || (total != nil && float64(len(out)) >= *total) {
			return out, nil
		}
	}
	return nil, klineError(sdkerrors.UpstreamError, "eastmoney", s.base, "stock changes pagination exceeded maxPages", nil)
}
func assertCNStock(symbol string) (symbols.Symbol, error) {
	ns, err := symbols.Normalize(symbol, symbols.Hint{Market: symbols.CN})
	if err != nil {
		return ns, err
	}
	if ns.AssetType != symbols.Stock {
		return ns, argument("individual changes require a CN stock")
	}
	return ns, nil
}

// IndividualDay exposes availability and the date echoed by the server, including non-trading-day fallback.
func (s *MarketEventService) IndividualDay(ctx context.Context, symbol string, dates ...string) (*IndividualChangesDay, error) {
	ns, err := assertCNStock(symbol)
	if err != nil {
		return nil, err
	}
	date, err := calendarDate(dates)
	if err != nil {
		return nil, err
	}
	secid, err := symbols.ToEastmoney(ns)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(secid, ".", 2)
	data, err := s.topicData(ctx, "getStockChanges", url.Values{"ut": {emToken}, "dpt": {"wzchanges"}, "code": {parts[1]}, "market": {parts[0]}, "date": {compactDate(date)}}, "")
	if err != nil {
		return nil, err
	}
	out := &IndividualChangesDay{Date: date, Available: data != nil, Code: ns.Code, Changes: []IndividualStockChangeItem{}}
	if data == nil {
		return out, nil
	}
	out.Name = rowText(data, "n")
	if nullableText(data, "c") != nil {
		out.Code = rowText(data, "c")
	}
	if echoed := rowText(data, "d"); len(echoed) == 8 {
		if iso, e := dateISO(echoed); e == nil {
			out.Date = iso
		}
	}
	for _, r := range arrayRows(data["data"]) {
		code := rowText(r, "t")
		kind, label := changeIdentity(code)
		out.Changes = append(out.Changes, IndividualStockChangeItem{Time: formatTopicTime(r["tm"]), TypeCode: code, ChangeType: kind, ChangeTypeLabel: label, Price: topicPrice(r["p"]), ChangePercent: rowNumber(r, "u"), Info: rowText(r, "i"), V: rowNumber(r, "v")})
	}
	return out, nil
}
func (s *MarketEventService) IndividualChanges(ctx context.Context, symbol string, dates ...string) ([]IndividualStockChangeItem, error) {
	day, err := s.IndividualDay(ctx, symbol, dates...)
	if err != nil {
		return nil, err
	}
	return day.Changes, nil
}
func (s *MarketEventService) IndividualChangesHistory(ctx context.Context, symbol string, spans ...int) (*IndividualChangesHistory, error) {
	if len(spans) > 1 {
		return nil, argument("at most one history span is allowed")
	}
	span := 7
	if len(spans) == 1 {
		span = spans[0]
	}
	if span < 1 || span > 60 {
		return nil, argument("history days must be 1..60")
	}
	ns, err := assertCNStock(symbol)
	if err != nil {
		return nil, err
	}
	to, _ := calendarDate(nil)
	from := dateAdd(to, -(span - 1))
	calendar, err := s.codes.calendar(ctx)
	if err != nil {
		return nil, err
	}
	dates := []string{}
	for _, d := range calendar {
		if d >= from && d <= to {
			dates = append(dates, d)
		}
	}
	days, err := batchFetch(ctx, dates, BatchOptions{BatchSize: 1, Concurrency: 4}, func(ctx context.Context, ids []string) ([]IndividualChangesDay, error) {
		day, e := s.IndividualDay(ctx, symbol, ids[0])
		if e != nil {
			return nil, e
		}
		if day.Date != ids[0] {
			day = &IndividualChangesDay{Date: ids[0], Code: day.Code, Changes: []IndividualStockChangeItem{}}
		}
		return []IndividualChangesDay{*day}, nil
	})
	if err != nil {
		return nil, err
	}
	out := &IndividualChangesHistory{Code: ns.Code, RequestedDays: span, Coverage: ChangesCoverage{From: from, To: to}, Days: append([]IndividualChangesDay{}, days...), Stats: map[string]ChangeTypeCount{}}
	if len(days) > 0 {
		out.Code = days[0].Code
	}
	for _, day := range days {
		if !day.Available {
			continue
		}
		if out.Coverage.AvailableFrom == nil {
			d := day.Date
			out.Coverage.AvailableFrom = &d
		}
		if out.Name == "" && day.Name != "" {
			out.Code, out.Name = day.Code, day.Name
		}
		for _, c := range day.Changes {
			key := c.TypeCode
			if key == "" {
				key = "unknown"
			}
			entry := out.Stats[key]
			if entry.Count == 0 {
				entry.Label = c.ChangeTypeLabel
			}
			entry.Count++
			out.Stats[key] = entry
		}
	}
	return out, nil
}
func distributionNumber(raw json.RawMessage) float64 {
	if string(raw) == "true" {
		return 1
	}
	if n := strictNumber(raw); n != nil {
		return *n
	}
	return 0
}
func mapBoardChange(r emRow) BoardChangeItem {
	var ms emRow
	_ = json.Unmarshal(r["ms"], &ms)
	direction := ""
	if string(ms["m"]) == "0" {
		direction = "大笔买入"
	} else if string(ms["m"]) == "1" {
		direction = "大笔卖出"
	}
	dist := map[string]float64{}
	var entries []emRow
	if json.Unmarshal(r["ydl"], &entries) == nil && entries != nil {
		for _, e := range entries {
			if nullableText(e, "t") != nil {
				dist[rowText(e, "t")] = distributionNumber(e["ct"])
			}
		}
	} else {
		var obj emRow
		_ = json.Unmarshal(rawDefault(r, "bkdf", "bkdfdis"), &obj)
		for key, raw := range obj {
			dist[key] = distributionNumber(raw)
		}
	}
	return BoardChangeItem{Name: rowTextDefault(r, "n", "bkn"), ChangePercent: rawNumber(rawDefault(r, "u", "bkz")), MainNetInflow: rawNumber(rawDefault(r, "zjl", "bkj")), TotalChangeCount: rawNumber(rawDefault(r, "ct", "bkc")), TopStockCode: rowText(ms, "c"), TopStockName: rowText(ms, "n"), TopStockDirection: direction, ChangeTypeDistribution: dist}
}
func (s *MarketEventService) BoardChanges(ctx context.Context) ([]BoardChangeItem, error) {
	data, err := s.topicData(ctx, "getAllBKChanges", url.Values{"ut": {emToken}, "dpt": {"wzchanges"}, "pageindex": {"0"}, "pagesize": {"5000"}}, "")
	if err != nil {
		return nil, err
	}
	out := []BoardChangeItem{}
	for _, r := range arrayRows(data["allbk"]) {
		out = append(out, mapBoardChange(r))
	}
	return out, nil
}
func mapUnusual(r emRow) UnusualFluctuationItem {
	date := dcDate(rowText(r, "TRADE_DATE"))
	direction := "down"
	if rowText(r, "IS_POSITIVE") == "1" {
		direction = "up"
	}
	return UnusualFluctuationItem{Code: rowText(r, "SECURITY_CODE"), Name: rowText(r, "SECURITY_NAME_ABBR"), Date: date, Timestamp: markettime.Parse(date, markettime.CN), Tz: markettime.CN, Rule: rowText(r, "UNUSUAL_TYPE"), Triggered: rowText(r, "IS_HAPPEN") == "1", DeviationValue: rowNumber(r, "DEVUATION_VALUE"), WindowDays: rowNumber(r, "MAX_DAYS"), ChangePercent: rowNumber(r, "CHANGE_RATE"), Direction: direction, TargetChangePercent: rowNumber(r, "CHANGE_RATE_TARGET")}
}
func (s *MarketEventService) UnusualFluctuation(ctx context.Context, options ...UnusualFluctuationOptions) ([]UnusualFluctuationItem, error) {
	if len(options) > 1 {
		return nil, argument("at most one UnusualFluctuationOptions is allowed")
	}
	var o UnusualFluctuationOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Date != "" && (o.StartDate != "" || o.EndDate != "") {
		return nil, argument("date and startDate/endDate are mutually exclusive")
	}
	start, end, err := normalizedRange(o.StartDate, o.EndDate, false)
	if err != nil {
		return nil, err
	}
	filter := rangeFilter(start, end)
	if o.Date != "" {
		date, e := dateISO(o.Date)
		if e != nil {
			return nil, e
		}
		filter = `(TRADE_DATE='` + date + `')`
	}
	if o.Triggered != nil {
		v := "0"
		if *o.Triggered {
			v = "1"
		}
		filter += `(IS_HAPPEN="` + v + `")`
	}
	return mapReport(ctx, s.dc, DatacenterQuery{ReportName: "RPT_WATCH_UNUSUAL_FLUCTUATE", SortColumns: "TRADE_DATE,SECURITY_CODE", SortTypes: "-1,1", PageSize: 500, Filter: filter}, mapUnusual)
}

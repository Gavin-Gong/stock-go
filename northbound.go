package stockgo

import (
	"context"
	"encoding/json"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"net/url"
	"strings"
	"sync/atomic"
)

type NorthboundDirection string

const (
	North NorthboundDirection = "north"
	South NorthboundDirection = "south"
)

type NorthboundHoldingRankOptions struct{ Market, Period, Date string }
type NorthboundHistoryOptions struct {
	Direction          NorthboundDirection
	StartDate, EndDate string
}
type NorthboundService struct {
	historySchema atomic.Int32
	dc            *DatacenterService
	minuteURL     string
}

func direction(d NorthboundDirection) (NorthboundDirection, error) {
	if d == "" {
		return North, nil
	}
	if d != North && d != South {
		return "", argument("direction must be north or south")
	}
	return d, nil
}
func (s *NorthboundService) Minute(ctx context.Context, directions ...NorthboundDirection) ([]NorthboundMinuteItem, error) {
	if len(directions) > 1 {
		return nil, argument("at most one direction is allowed")
	}
	d := North
	if len(directions) == 1 {
		d = directions[0]
	}
	d, err := direction(d)
	if err != nil {
		return nil, err
	}
	endpoint := endpointQuery(s.minuteURL, url.Values{"fields1": {"f1,f2,f3,f4"}, "fields2": {"f51,f54,f52,f58,f53,f62,f56,f57,f60,f61"}, "ut": {emDataToken}})
	var response struct {
		Data *struct {
			S2N, N2S         []json.RawMessage
			S2NDate, N2SDate string
		} `json:"data"`
	}
	if err := getJSON(ctx, s.dc.client, "eastmoney", endpoint, &response); err != nil {
		return nil, err
	}
	out := []NorthboundMinuteItem{}
	if response.Data == nil {
		return out, nil
	}
	rows, date := response.Data.S2N, response.Data.S2NDate
	if d == South {
		rows, date = response.Data.N2S, response.Data.N2SDate
	}
	if len(date) == 8 && digitsOnly(date) {
		date = date[:4] + "-" + date[4:6] + "-" + date[6:]
	}
	for _, raw := range rows {
		line, ok := rawString(raw)
		if !ok {
			continue
		}
		f := strings.Split(line, ",")
		out = append(out, NorthboundMinuteItem{Date: date, Time: at(f, 0), ShanghaiNetInflow: nullable(at(f, 1)), ShenzhenNetInflow: nullable(at(f, 3)), TotalNetInflow: nullable(at(f, 5))})
	}
	return out, nil
}
func (s *NorthboundService) Summary(ctx context.Context) ([]NorthboundFlowSummary, error) {
	rows, err := s.dc.rows(ctx, DatacenterQuery{ReportName: "RPT_MUTUAL_QUOTA", Columns: "TRADE_DATE,MUTUAL_TYPE,BOARD_TYPE,MUTUAL_TYPE_NAME,FUNDS_DIRECTION,INDEX_CODE,INDEX_NAME,BOARD_CODE", QuoteColumns: "status~07~BOARD_CODE,dayNetAmtIn~07~BOARD_CODE,dayAmtRemain~07~BOARD_CODE,dayAmtThreshold~07~BOARD_CODE,f104~07~BOARD_CODE,f105~07~BOARD_CODE,f106~07~BOARD_CODE,f3~03~INDEX_CODE~INDEX_f3,netBuyAmt~07~BOARD_CODE", QuoteType: "0", SortColumns: "MUTUAL_TYPE", SortTypes: "1", PageSize: 2000, FetchAllPages: Bool(false)})
	if err != nil {
		return nil, err
	}
	out := make([]NorthboundFlowSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapNorthSummary(r))
	}
	return out, nil
}
func (s *NorthboundService) HoldingRank(ctx context.Context, options ...NorthboundHoldingRankOptions) ([]NorthboundHoldingRankItem, error) {
	if len(options) > 1 {
		return nil, argument("at most one holding rank options is allowed")
	}
	o := NorthboundHoldingRankOptions{Market: "all", Period: "5day"}
	if len(options) == 1 {
		if options[0].Market != "" {
			o.Market = options[0].Market
		}
		if options[0].Period != "" {
			o.Period = options[0].Period
		}
		o.Date = options[0].Date
	}
	interval := map[string]string{"today": "1", "3day": "3", "5day": "5", "10day": "10", "month": "M", "quarter": "Q", "year": "Y"}[o.Period]
	if interval == "" {
		return nil, argument("invalid northbound holding period")
	}
	filter := `(INTERVAL_TYPE="` + interval + `")`
	if o.Date != "" {
		date, err := dateISO(o.Date)
		if err != nil {
			return nil, err
		}
		filter += `(TRADE_DATE='` + date + `')`
	}
	if o.Market != "all" {
		code := map[string]string{"shanghai": "001", "shenzhen": "003"}[o.Market]
		if code == "" {
			return nil, argument("market must be all, shanghai or shenzhen")
		}
		filter += `(MUTUAL_TYPE="` + code + `")`
	}
	rows, err := s.dc.rows(ctx, DatacenterQuery{ReportName: "RPT_MUTUAL_STOCK_NORTHSTA", SortColumns: "ADD_MARKET_CAP", SortTypes: "-1", PageSize: 500, Filter: filter})
	if err != nil {
		return nil, err
	}
	out := make([]NorthboundHoldingRankItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapNorthHolding(r))
	}
	return out, nil
}
func northHistoryOptions(options []NorthboundHistoryOptions) (NorthboundHistoryOptions, error) {
	if len(options) > 1 {
		return NorthboundHistoryOptions{}, argument("at most one northbound history options is allowed")
	}
	var o NorthboundHistoryOptions
	if len(options) == 1 {
		o = options[0]
	}
	var err error
	o.Direction, err = direction(o.Direction)
	if err != nil {
		return o, err
	}
	if o.StartDate != "" {
		o.StartDate, err = dateISO(o.StartDate)
		if err != nil {
			return o, err
		}
	}
	if o.EndDate != "" {
		o.EndDate, err = dateISO(o.EndDate)
		if err != nil {
			return o, err
		}
	}
	if o.StartDate != "" && o.EndDate != "" && o.StartDate > o.EndDate {
		return o, argument("startDate exceeds endDate")
	}
	return o, nil
}
func northDateFilter(o NorthboundHistoryOptions) string {
	filter := ""
	if o.StartDate != "" {
		filter += `(TRADE_DATE>='` + o.StartDate + `')`
	}
	if o.EndDate != "" {
		filter += `(TRADE_DATE<='` + o.EndDate + `')`
	}
	return filter
}
func (s *NorthboundService) History(ctx context.Context, options ...NorthboundHistoryOptions) ([]NorthboundHistoryItem, error) {
	o, err := northHistoryOptions(options)
	if err != nil {
		return nil, err
	}
	board := "1"
	if o.Direction == South {
		board = "0"
	}
	filter := `(BOARD_TYPE="` + board + `")` + northDateFilter(o)
	modernFilter := func() string {
		channel := "005"
		if o.Direction == South {
			channel = "006"
		}
		return `(MUTUAL_TYPE="` + channel + `")` + northDateFilter(o)
	}
	usedModern := s.historySchema.Load() == 2
	if usedModern {
		filter = modernFilter()
	}
	query := DatacenterQuery{ReportName: "RPT_MUTUAL_DEAL_HISTORY", SortColumns: "TRADE_DATE", SortTypes: "-1", PageSize: 500, Filter: filter}
	rows, err := s.dc.rows(ctx, query)
	if err != nil && !usedModern && sdkerrors.CodeOf(err) == sdkerrors.UpstreamError && strings.Contains(err.Error(), "BOARD_TYPE") {
		s.historySchema.Store(2)
		query.Filter = modernFilter()
		rows, err = s.dc.rows(ctx, query)
	}
	if err != nil {
		return nil, err
	}
	out := make([]NorthboundHistoryItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapNorthHistory(r))
	}
	return out, nil
}
func (s *NorthboundService) Individual(ctx context.Context, symbol string, options ...NorthboundHistoryOptions) ([]NorthboundIndividualItem, error) {
	o, err := northHistoryOptions(options)
	if err != nil {
		return nil, err
	}
	code, err := pureCNCode(symbol)
	if err != nil {
		return nil, err
	}
	filter := `(SECURITY_CODE="` + code + `")` + northDateFilter(o)
	rows, err := s.dc.rows(ctx, DatacenterQuery{ReportName: "RPT_MUTUAL_HOLDSTOCKNORTH_STA", SortColumns: "TRADE_DATE", SortTypes: "-1", PageSize: 500, Filter: filter})
	if err != nil {
		return nil, err
	}
	out := make([]NorthboundIndividualItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapNorthIndividual(r))
	}
	return out, nil
}

func mapNorthSummary(r emRow) NorthboundFlowSummary {
	return NorthboundFlowSummary{
		Date:               dcDate(rowText(r, "TRADE_DATE")),
		Type:               rowText(r, "MUTUAL_TYPE"),
		BoardName:          rowText(r, "MUTUAL_TYPE_NAME"),
		Direction:          rowText(r, "FUNDS_DIRECTION"),
		Status:             rowText(r, "status"),
		NetBuyAmount:       rowNumber(r, "netBuyAmt"),
		NetInflow:          rowNumber(r, "dayNetAmtIn"),
		RemainAmount:       rowNumber(r, "dayAmtRemain"),
		UpCount:            rowNumber(r, "f104"),
		FlatCount:          rowNumber(r, "f106"),
		DownCount:          rowNumber(r, "f105"),
		IndexCode:          rowText(r, "INDEX_CODE"),
		IndexName:          rowText(r, "INDEX_NAME"),
		IndexChangePercent: rowNumber(r, "INDEX_f3"),
	}
}

func mapNorthHolding(r emRow) NorthboundHoldingRankItem {
	return NorthboundHoldingRankItem{
		Date:                  dcDate(rowText(r, "TRADE_DATE")),
		Code:                  rowText(r, "SECURITY_CODE"),
		Name:                  rowTextDefault(r, "SECURITY_NAME", "SECURITY_NAME_ABBR"),
		Close:                 rowNumber(r, "CLOSE_PRICE"),
		ChangePercent:         rowNumber(r, "CHANGE_RATE"),
		HoldShares:            rowNumber(r, "HOLD_SHARES"),
		HoldMarketValue:       rowNumber(r, "HOLD_MARKET_CAP"),
		HoldRatioFloat:        rowNumberDefault(r, "HOLD_RATIO", "FREE_SHARES_RATIO"),
		HoldRatioTotal:        rowNumberDefault(r, "A_SHARES_RATIO", "TOTAL_SHARES_RATIO"),
		AddShares:             rowNumber(r, "ADD_SHARES"),
		AddMarketValue:        rowNumber(r, "ADD_MARKET_CAP"),
		AddMarketValuePercent: rowNumber(r, "ADD_MARKET_CAP_PROPORTION"),
		Sector:                rowText(r, "BOARD_NAME"),
	}
}

func mapNorthHistory(r emRow) NorthboundHistoryItem {
	return NorthboundHistoryItem{
		Date:                  dcDate(rowText(r, "TRADE_DATE")),
		NetBuyAmount:          rowNumber(r, "NET_DEAL_AMT"),
		BuyAmount:             rowNumber(r, "BUY_AMT"),
		SellAmount:            rowNumber(r, "SELL_AMT"),
		AccNetBuyAmount:       rowNumber(r, "ACCUM_DEAL_AMT"),
		NetInflow:             rowNumber(r, "FUND_INFLOW"),
		RemainAmount:          rowNumber(r, "QUOTA_BALANCE"),
		TopStockCode:          optionalText(r, "LEAD_STOCKS_CODE"),
		TopStockName:          optionalText(r, "LEAD_STOCKS_NAME"),
		TopStockChangePercent: rowNumber(r, "LS_CHANGE_RATE"),
	}
}

func mapNorthIndividual(r emRow) NorthboundIndividualItem {
	return NorthboundIndividualItem{
		Date:            dcDate(rowText(r, "TRADE_DATE")),
		HoldShares:      rowNumber(r, "HOLD_SHARES"),
		HoldMarketValue: rowNumber(r, "HOLD_MARKET_CAP"),
		HoldRatioFloat:  rowNumberDefault(r, "HOLD_RATIO", "FREE_SHARES_RATIO"),
		HoldRatioTotal:  rowNumberDefault(r, "A_SHARES_RATIO", "TOTAL_SHARES_RATIO"),
		Close:           rowNumber(r, "CLOSE_PRICE"),
		ChangePercent:   rowNumber(r, "CHANGE_RATE"),
	}
}

func rowNumberDefault(r emRow, primary, fallback string) *float64 {
	if n := rowNumber(r, primary); n != nil {
		return n
	}
	return rowNumber(r, fallback)
}

package stockgo

import (
	"context"
	"strings"
)

type DragonTigerDateOptions struct{ StartDate, EndDate string }
type BlockTradeDateOptions struct{ StartDate, EndDate string }
type DragonTigerPeriod string

const (
	DragonMonth    DragonTigerPeriod = "1month"
	DragonQuarter  DragonTigerPeriod = "3month"
	DragonHalfYear DragonTigerPeriod = "6month"
	DragonYear     DragonTigerPeriod = "1year"
)

type DragonTigerService struct{ dc *DatacenterService }
type BlockTradeService struct{ dc *DatacenterService }
type MarginService struct{ dc *DatacenterService }

func normalizedRange(start, end string, required bool) (string, string, error) {
	var err error
	if required && (start == "" || end == "") {
		return "", "", argument("startDate and endDate are required")
	}
	if start != "" {
		start, err = dateISO(start)
		if err != nil {
			return "", "", err
		}
	}
	if end != "" {
		end, err = dateISO(end)
		if err != nil {
			return "", "", err
		}
	}
	if start != "" && end != "" && start > end {
		return "", "", argument("startDate exceeds endDate")
	}
	return start, end, nil
}
func rangeFilter(start, end string) string {
	filter := ""
	if start != "" {
		filter += `(TRADE_DATE>='` + start + `')`
	}
	if end != "" {
		filter += `(TRADE_DATE<='` + end + `')`
	}
	return filter
}
func mapReport[T any](ctx context.Context, dc *DatacenterService, q DatacenterQuery, mapper func(emRow) T) ([]T, error) {
	rows, err := dc.rows(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapper(r))
	}
	return out, nil
}
func dragonRange(o DragonTigerDateOptions) (string, error) {
	start, end, err := normalizedRange(o.StartDate, o.EndDate, true)
	if err != nil {
		return "", err
	}
	return `(TRADE_DATE<='` + end + `')(TRADE_DATE>='` + start + `')`, nil
}
func dragonPeriod(periods []DragonTigerPeriod) (string, error) {
	if len(periods) > 1 {
		return "", argument("at most one dragon-tiger period is allowed")
	}
	period := DragonMonth
	if len(periods) == 1 && periods[0] != "" {
		period = periods[0]
	}
	cycle := map[DragonTigerPeriod]string{DragonMonth: "01", DragonQuarter: "02", DragonHalfYear: "03", DragonYear: "04"}[period]
	if cycle == "" {
		return "", argument("period must be 1month, 3month, 6month or 1year")
	}
	return `(STATISTICS_CYCLE="` + cycle + `")`, nil
}
func (s *DragonTigerService) Detail(ctx context.Context, o DragonTigerDateOptions) ([]DragonTigerDetailItem, error) {
	filter, err := dragonRange(o)
	if err != nil {
		return nil, err
	}
	return mapReport(ctx, s.dc, DatacenterQuery{ReportName: "RPT_DAILYBILLBOARD_DETAILSNEW", Columns: "SECURITY_CODE,SECUCODE,SECURITY_NAME_ABBR,TRADE_DATE,EXPLAIN,CLOSE_PRICE,CHANGE_RATE,BILLBOARD_NET_AMT,BILLBOARD_BUY_AMT,BILLBOARD_SELL_AMT,BILLBOARD_DEAL_AMT,ACCUM_AMOUNT,DEAL_NET_RATIO,DEAL_AMOUNT_RATIO,TURNOVERRATE,FREE_MARKET_CAP,EXPLANATION,D1_CLOSE_ADJCHRATE,D2_CLOSE_ADJCHRATE,D5_CLOSE_ADJCHRATE,D10_CLOSE_ADJCHRATE", SortColumns: "SECURITY_CODE,TRADE_DATE", SortTypes: "1,-1", PageSize: 5000, Filter: filter}, mapDragonDetail)
}
func (s *DragonTigerService) StockStats(ctx context.Context, periods ...DragonTigerPeriod) ([]DragonTigerStockStatItem, error) {
	filter, err := dragonPeriod(periods)
	if err != nil {
		return nil, err
	}
	return mapReport(ctx, s.dc, DatacenterQuery{ReportName: "RPT_BILLBOARD_TRADEALL", SortColumns: "BILLBOARD_TIMES,LATEST_TDATE,SECURITY_CODE", SortTypes: "-1,-1,1", PageSize: 5000, Filter: filter}, mapDragonStockStat)
}
func (s *DragonTigerService) Institution(ctx context.Context, o DragonTigerDateOptions) ([]DragonTigerInstitutionItem, error) {
	filter, err := dragonRange(o)
	if err != nil {
		return nil, err
	}
	return mapReport(ctx, s.dc, DatacenterQuery{ReportName: "RPT_ORGANIZATION_TRADE_DETAILS", SortColumns: "TRADE_DATE,SECURITY_CODE", SortTypes: "-1,1", PageSize: 5000, Filter: filter}, mapDragonInstitution)
}
func (s *DragonTigerService) BranchRank(ctx context.Context, periods ...DragonTigerPeriod) ([]DragonTigerBranchItem, error) {
	filter, err := dragonPeriod(periods)
	if err != nil {
		return nil, err
	}
	legacy := DatacenterQuery{ReportName: "RPT_BILLBOARD_TRADEDETAILS", SortColumns: "TOTAL_BUYER_SALESTIMES", SortTypes: "-1", PageSize: 5000, Filter: filter}
	modern := legacy
	modern.ReportName = "RPT_OPERATEDEPT_LIST_STATISTICS"
	modern.Filter = strings.Replace(filter, "STATISTICS_CYCLE", "STATISTICSCYCLE", 1)
	return compatibleReport(ctx, s.dc, legacy, modern, mapDragonBranch, mapModernBranch)
}
func (s *DragonTigerService) SeatDetail(ctx context.Context, symbol, date string) ([]DragonTigerSeatItem, error) {
	code, err := pureCNCode(symbol)
	if err != nil {
		return nil, err
	}
	date, err = dateISO(date)
	if err != nil {
		return nil, err
	}
	filter := `(SECURITY_CODE="` + code + `")(TRADE_DATE='` + date + `')`
	return batchFetch(ctx, []string{"buy", "sell"}, BatchOptions{BatchSize: 1, Concurrency: 2}, func(ctx context.Context, sides []string) ([]DragonTigerSeatItem, error) {
		side := sides[0]
		report, sort := "RPT_BILLBOARD_DAILYDETAILSBUY", "BUY_AMT_REAL"
		if side == "sell" {
			report, sort = "RPT_BILLBOARD_DAILYDETAILSSELL", "SELL_AMT_REAL"
		}
		query := DatacenterQuery{ReportName: report, SortColumns: sort, SortTypes: "-1", PageSize: 100, Filter: filter}
		rows, err := s.dc.rows(ctx, query)
		modern := reportSchemaChanged(err)
		if modern {
			query.SortColumns = "BUY"
			if side == "sell" {
				query.SortColumns = "SELL"
			}
			rows, err = s.dc.rows(ctx, query)
		}
		if err != nil {
			return nil, err
		}
		out := make([]DragonTigerSeatItem, 0, len(rows))
		for i, r := range rows {
			rank := rowNumber(r, "RANK")
			if rank == nil {
				value := float64(i + 1)
				rank = &value
			}
			item := DragonTigerSeatItem{Rank: rank, BranchName: rowText(r, "OPERATEDEPT_NAME"), BuyAmount: rawNumber(rawDefault(r, "BUY_AMT_REAL", "BUY_AMT")), BuyAmountRatio: rawNumber(rawDefault(r, "BUY_RATIO_TOTAL", "BUY_AMT_RATIO")), SellAmount: rawNumber(rawDefault(r, "SELL_AMT_REAL", "SELL_AMT")), SellAmountRatio: rawNumber(rawDefault(r, "SELL_RATIO_TOTAL", "SELL_AMT_RATIO")), NetAmount: rowNumber(r, "NET_AMT"), Side: side}
			if modern {
				item.BuyAmount = rowNumber(r, "BUY")
				item.SellAmount = rowNumber(r, "SELL")
				item.NetAmount = rowNumber(r, "NET")
				item.BuyAmountRatio = scaleNumber(rowNumber(r, "TOTAL_BUYRIO"), 100)
				item.SellAmountRatio = scaleNumber(rowNumber(r, "TOTAL_SELLRIO"), 100)
			}
			out = append(out, item)
		}
		return out, nil
	})
}
func blockRange(options []BlockTradeDateOptions) (string, error) {
	if len(options) > 1 {
		return "", argument("at most one block-trade date options is allowed")
	}
	var o BlockTradeDateOptions
	if len(options) == 1 {
		o = options[0]
	}
	start, end, err := normalizedRange(o.StartDate, o.EndDate, false)
	if err != nil {
		return "", err
	}
	return rangeFilter(start, end), nil
}
func (s *BlockTradeService) MarketStat(ctx context.Context) ([]BlockTradeMarketStatItem, error) {
	return mapReport(ctx, s.dc, DatacenterQuery{ReportName: "PRT_BLOCKTRADE_MARKET_STA", SortColumns: "TRADE_DATE", SortTypes: "-1", PageSize: 500}, mapBlockMarket)
}
func (s *BlockTradeService) Detail(ctx context.Context, options ...BlockTradeDateOptions) ([]BlockTradeDetailItem, error) {
	filter, err := blockRange(options)
	if err != nil {
		return nil, err
	}
	legacy := DatacenterQuery{ReportName: "RPT_BLOCK_TRADE_DETAIL", SortColumns: "TRADE_DATE,SECURITY_CODE", SortTypes: "-1,1", PageSize: 5000, Filter: filter}
	modern := legacy
	modern.ReportName = "RPT_DATA_BLOCKTRADE"
	return compatibleReport(ctx, s.dc, legacy, modern, mapBlockDetail, mapModernBlockDetail)
}
func (s *BlockTradeService) DailyStat(ctx context.Context, options ...BlockTradeDateOptions) ([]BlockTradeDailyStatItem, error) {
	filter, err := blockRange(options)
	if err != nil {
		return nil, err
	}
	legacy := DatacenterQuery{ReportName: "RPT_BLOCK_TRADE_STA", SortColumns: "TRADE_DATE,DEAL_AMT", SortTypes: "-1,-1", PageSize: 5000, Filter: filter}
	modern := legacy
	modern.ReportName = "RPT_BLOCKTRADE_STAINCLUDE"
	return compatibleReport(ctx, s.dc, legacy, modern, mapBlockDaily, mapModernBlockDaily)
}
func (s *MarginService) AccountInfo(ctx context.Context) ([]MarginAccountItem, error) {
	return mapReport(ctx, s.dc, DatacenterQuery{ReportName: "RPTA_WEB_MARGIN_DAILYTRADE", SortColumns: "STATISTICS_DATE", SortTypes: "-1", PageSize: 500}, mapMarginAccount)
}
func (s *MarginService) TargetList(ctx context.Context, dates ...string) ([]MarginTargetItem, error) {
	if len(dates) > 1 {
		return nil, argument("at most one margin date is allowed")
	}
	filter := ""
	if len(dates) == 1 && dates[0] != "" {
		date, err := dateISO(dates[0])
		if err != nil {
			return nil, err
		}
		filter = `(TRADE_DATE='` + date + `')`
	}
	rows, err := mapReport(ctx, s.dc, DatacenterQuery{ReportName: "RPT_MARGIN_TRADE_DETAIL", SortColumns: "FIN_BALANCE", SortTypes: "-1", PageSize: 5000, Filter: filter}, mapMarginTarget)
	if !reportSchemaChanged(err) {
		return rows, err
	}
	modern := DatacenterQuery{ReportName: "RPTA_WEB_RZRQ_GGMX", SortColumns: "RZYE", SortTypes: "-1", PageSize: 5000, Filter: strings.Replace(filter, "TRADE_DATE", "DATE", 1)}
	if modern.Filter == "" {
		latest, e := s.dc.rows(ctx, DatacenterQuery{ReportName: modern.ReportName, SortColumns: "DATE", SortTypes: "-1", PageSize: 1, FetchAllPages: Bool(false)})
		if e != nil {
			return nil, e
		}
		if len(latest) == 0 {
			return []MarginTargetItem{}, nil
		}
		date, e := dateISO(dcDate(rowText(latest[0], "DATE")))
		if e != nil {
			return nil, e
		}
		modern.Filter = `(DATE='` + date + `')`
	}
	return mapReport(ctx, s.dc, modern, mapModernMarginTarget)
}

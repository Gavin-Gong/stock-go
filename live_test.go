package stockgo_test

import (
	"context"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	stockgo "github.com/Gavin-Gong/stock-go"
	"github.com/Gavin-Gong/stock-go/indicators"
	"github.com/Gavin-Gong/stock-go/screener"
)

// Explicit opt-in: STOCK_GO_LIVE=1 go test -run TestLive -v .
// These tests require external services and fail if a real query returns no data.
func TestLive(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers")
	}
	sdk, err := stockgo.New(stockgo.Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	t.Run("CN", func(t *testing.T) {
		q, err := sdk.Quotes.CN(ctx, []string{"600519"})
		if err != nil {
			t.Fatal(err)
		}
		if len(q) != 1 || q[0].Name == "" || q[0].Timestamp == nil {
			t.Fatalf("missing live quote: %+v", q)
		}
		t.Log(q[0].Code, q[0].Name, q[0].Time)
	})
	t.Run("CNSimple", func(t *testing.T) {
		q, err := sdk.Quotes.CNSimple(ctx, []string{"600519"})
		if err != nil {
			t.Fatal(err)
		}
		if len(q) != 1 || q[0].Price <= 0 {
			t.Fatalf("missing live quote: %+v", q)
		}
		t.Log(q[0].Code, q[0].Name)
	})
	t.Run("HK", func(t *testing.T) {
		q, err := sdk.Quotes.HK(ctx, []string{"00700"})
		if err != nil {
			t.Fatal(err)
		}
		if len(q) != 1 || q[0].Timestamp == nil {
			t.Fatalf("missing live quote/time: %+v", q)
		}
		t.Log(q[0].Code, q[0].Name, q[0].Time)
	})
	t.Run("US", func(t *testing.T) {
		q, err := sdk.Quotes.US(ctx, []string{"AAPL"})
		if err != nil {
			t.Fatal(err)
		}
		if len(q) != 1 || q[0].Timestamp == nil {
			t.Fatalf("missing live quote/time: %+v", q)
		}
		t.Log(q[0].Code, q[0].Name, q[0].Time)
	})
	t.Run("Fund", func(t *testing.T) {
		q, err := sdk.Quotes.Fund(ctx, []string{"110011"})
		if err != nil {
			t.Fatal(err)
		}
		if len(q) != 1 || q[0].NAV <= 0 {
			t.Fatalf("missing live fund: %+v", q)
		}
		t.Log(q[0].Code, q[0].Name, q[0].NavDate)
	})
	t.Run("Search", func(t *testing.T) {
		q, err := sdk.Search(ctx, "茅台")
		if err != nil {
			t.Fatal(err)
		}
		if len(q) == 0 {
			t.Fatal("empty live search")
		}
		t.Log(q[0].Code, q[0].Name)
	})
	t.Run("Timeline", func(t *testing.T) {
		q, err := sdk.Quotes.Timeline(ctx, "600519")
		if err != nil {
			t.Fatal(err)
		}
		if len(q.Data) == 0 || q.Data[0].Timestamp == nil {
			t.Fatalf("missing timeline/time: %+v", q)
		}
		t.Log(q.Code, q.Date, "points:", len(q.Data))
	})
}

func TestLiveKlineAndCodes(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers")
	}
	sdk, err := stockgo.New(stockgo.Options{Timeout: 8 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -30).Format("20060102")
	end := now.Format("20060102")
	t.Run("CodesCNAndBatch", func(t *testing.T) {
		codes, err := sdk.Codes.CN(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(codes) < 3 {
			t.Fatal("empty code list")
		}
		q, err := sdk.Batch.ByCodes(ctx, codes[:3], stockgo.BatchOptions{BatchSize: 2, Concurrency: 2})
		if err != nil || len(q) != 3 {
			t.Fatalf("batch count=%d error=%v", len(q), err)
		}
		t.Log("CN codes:", len(codes), "batch quotes:", len(q))
	})
	t.Run("CodesHK", func(t *testing.T) {
		codes, err := sdk.Codes.HK(ctx)
		if err != nil || len(codes) == 0 {
			t.Fatalf("count=%d error=%v", len(codes), err)
		}
		t.Log("HK codes:", len(codes))
	})
	t.Run("CodesUS", func(t *testing.T) {
		codes, err := sdk.Codes.US(ctx)
		if err != nil || len(codes) == 0 {
			t.Fatalf("count=%d error=%v", len(codes), err)
		}
		t.Log("US codes:", len(codes))
	})
	t.Run("CodesFund", func(t *testing.T) {
		codes, err := sdk.Codes.Fund(ctx)
		if err != nil || len(codes) == 0 {
			t.Fatalf("count=%d error=%v", len(codes), err)
		}
		t.Log("fund codes:", len(codes))
	})
	for _, market := range []string{"CN", "HK", "US"} {
		t.Run("History"+market, func(t *testing.T) {
			o := stockgo.KlineOptions{StartDate: start, EndDate: end}
			var bars []stockgo.KlineBar
			var err error
			switch market {
			case "CN":
				bars, err = sdk.Kline.CN(ctx, "600519", o)
			case "HK":
				bars, err = sdk.Kline.HK(ctx, "00700", o)
			case "US":
				bars, err = sdk.Kline.US(ctx, "AAPL", o)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(bars) == 0 || bars[0].Close == nil || bars[0].Timestamp == nil {
				t.Fatalf("invalid live history: %+v", bars)
			}
			t.Log(market, "bars:", len(bars), "source:", bars[0].Source, "adjust:", bars[0].Adjust, "range:", bars[0].Date, bars[len(bars)-1].Date)
		})
	}
	t.Run("CNMinute", func(t *testing.T) {
		bars, err := sdk.Kline.CNMinute(ctx, "600519", stockgo.MinuteOptions{Period: "5"})
		if err != nil {
			t.Fatal(err)
		}
		if len(bars) == 0 || bars[0].Close == nil || bars[0].Timestamp == nil {
			t.Fatalf("invalid live minute: %+v", bars)
		}
		t.Log("minute bars:", len(bars), "source:", bars[0].Source, "adjust:", bars[0].Adjust)
	})
}

func TestLiveIndicators(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers")
	}
	sdk, err := stockgo.New(stockgo.Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	now := time.Now().UTC()
	o := stockgo.KlineOptions{StartDate: now.AddDate(0, 0, -30).Format("20060102"), EndDate: now.Format("20060102")}
	bars, err := sdk.Kline.WithIndicators(ctx, "600519", stockgo.KlineIndicatorOptions{KlineOptions: o, Indicators: indicators.Options{MA: &indicators.MAOptions{Periods: []int{5, 20}}, MACD: &indicators.MACDOptions{}, BOLL: &indicators.BOLLOptions{}, KDJ: &indicators.KDJOptions{}, RSI: &indicators.RSIOptions{}, SAR: &indicators.SAROptions{}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) == 0 {
		t.Fatal("empty indicator history")
	}
	last := bars[len(bars)-1]
	if last.MA["ma20"] == nil || last.MACD == nil || last.MACD.MACD == nil || last.BOLL == nil || last.BOLL.Upper == nil {
		t.Fatalf("missing warmed indicators: %+v", last)
	}
	events, err := sdk.Kline.Signals(ctx, "600519", stockgo.KlineSignalOptions{KlineOptions: o})
	if err != nil {
		t.Fatal(err)
	}
	report, err := screener.Backtest(screener.BacktestOptions[stockgo.KlineWithIndicators]{Klines: bars, Fee: screener.FeeRate(0.001), GetClose: func(b stockgo.KlineWithIndicators) *float64 { return b.Close }, GetDate: func(b stockgo.KlineWithIndicators) any { return b.Date }, Strategy: func(b stockgo.KlineWithIndicators, i int, all []stockgo.KlineWithIndicators) screener.StrategySignal {
		a, c := b.MA["ma5"], b.MA["ma20"]
		if a == nil || c == nil {
			return screener.Hold
		}
		if *a > *c {
			return screener.Buy
		}
		return screener.Sell
	}})
	if err != nil {
		t.Fatal(err)
	}
	if report.ValidBars != len(bars) || report.FinalEquity <= 0 {
		t.Fatalf("invalid report: %+v", report)
	}
	t.Log("indicator bars:", len(bars), "last date:", last.Date, "signal events:", len(events), "backtest valid bars:", report.ValidBars, "trades:", report.TradeCount)
}

func TestLiveExtensions(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers")
	}
	sdk, err := stockgo.New(stockgo.Options{Timeout: 8 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, market := range []string{"CN", "HK", "US"} {
		t.Run("Chips"+market, func(t *testing.T) {
			o := stockgo.ChipRequestOptions{Days: 5, Range: indicators.Int(30), IncludeHistogram: indicators.HistogramLast}
			var rows []indicators.ChipDistributionItem
			var err error
			switch market {
			case "CN":
				rows, err = sdk.Chips.CN(ctx, "600519", o)
			case "HK":
				rows, err = sdk.Chips.HK(ctx, "00700", o)
			case "US":
				rows, err = sdk.Chips.US(ctx, "105.AAPL", o)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 5 || rows[4].Histogram == nil || rows[4].AvgCost == nil {
				t.Fatalf("invalid chip distribution: %+v", rows)
			}
			t.Log("rows:", len(rows), "date:", rows[4].Date, "buckets:", len(rows[4].Histogram.Prices))
		})
	}
	for _, kind := range []string{"Industry", "Concept"} {
		t.Run("Board"+kind, func(t *testing.T) {
			provider := sdk.Board.Industry
			if kind == "Concept" {
				provider = sdk.Board.Concept
			}
			rows, err := provider.List(ctx, stockgo.PageOptions{Page: 1, PageSize: 5})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 {
				t.Fatal("empty board list")
			}
			code := rows[0].Code
			spot, err := provider.Spot(ctx, code)
			if err != nil || len(spot) == 0 {
				t.Fatalf("spot: count=%d error=%v", len(spot), err)
			}
			members, err := provider.Constituents(ctx, code, stockgo.PageOptions{Page: 1, PageSize: 5})
			if err != nil || len(members) == 0 {
				t.Fatalf("members: count=%d error=%v", len(members), err)
			}
			now := time.Now().UTC()
			bars, err := provider.Kline(ctx, code, stockgo.KlineOptions{StartDate: now.AddDate(0, 0, -15).Format("20060102"), EndDate: now.Format("20060102")})
			if err != nil || len(bars) == 0 {
				t.Fatalf("kline: count=%d error=%v", len(bars), err)
			}
			t.Log("boards:", len(rows), "sample:", code, "spot fields:", len(spot), "members:", len(members), "bars:", len(bars))
		})
	}
	t.Run("RealtimeFundFlow", func(t *testing.T) {
		rows, err := sdk.Quotes.FundFlow(ctx, []string{"600519"})
		if err != nil || len(rows) != 1 {
			t.Fatalf("count=%d error=%v", len(rows), err)
		}
		t.Log(rows[0].Code, rows[0].Name, rows[0].Date)
	})
	t.Run("IndividualFundFlow", func(t *testing.T) {
		rows, err := sdk.FundFlow.Individual(ctx, "600519")
		if err != nil || len(rows) == 0 {
			t.Fatalf("count=%d error=%v", len(rows), err)
		}
		t.Log("rows:", len(rows))
	})
	t.Run("MarketFundFlow", func(t *testing.T) {
		rows, err := sdk.FundFlow.Market(ctx)
		if err != nil || len(rows) == 0 {
			t.Fatalf("count=%d error=%v", len(rows), err)
		}
		t.Log("rows:", len(rows))
	})
	t.Run("FundFlowRank", func(t *testing.T) {
		rows, err := sdk.FundFlow.Rank(ctx, stockgo.FundFlowRankOptions{PageOptions: stockgo.PageOptions{Page: 1, PageSize: 5}})
		if err != nil || len(rows) == 0 {
			t.Fatalf("count=%d error=%v", len(rows), err)
		}
		t.Log("rows:", len(rows))
	})
	t.Run("SectorFundFlowRank", func(t *testing.T) {
		rows, err := sdk.FundFlow.SectorRank(ctx, stockgo.FundFlowRankOptions{PageOptions: stockgo.PageOptions{Page: 1, PageSize: 5}})
		if err != nil || len(rows) == 0 {
			t.Fatalf("count=%d error=%v", len(rows), err)
		}
		t.Log("rows:", len(rows))
	})
}

func TestLiveReferenceFund(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers")
	}
	sdk, err := stockgo.New(stockgo.Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	t.Run("Calendar", func(t *testing.T) {
		dates, err := sdk.Reference.TradingCalendar(ctx)
		if err != nil || len(dates) == 0 {
			t.Fatalf("calendar=%d err=%v", len(dates), err)
		}
		valid, err := sdk.Calendar.IsTradingDay(ctx, "20240102")
		if err != nil || !valid {
			t.Fatal(valid, err)
		}
		next, err := sdk.Calendar.NextTradingDay(ctx, "20240102")
		if err != nil {
			t.Fatal(err)
		}
		t.Log("dates:", len(dates), "next after 2024-01-02:", next)
	})
	t.Run("Dividend", func(t *testing.T) {
		rows, err := sdk.Reference.DividendDetail(ctx, "600519")
		if err != nil || len(rows) == 0 {
			t.Fatalf("rows=%d err=%v", len(rows), err)
		}
		t.Log("dividends:", len(rows))
	})
	t.Run("NorthboundMinuteSouth", func(t *testing.T) {
		rows, err := sdk.Northbound.Minute(ctx, stockgo.South)
		if err != nil || len(rows) == 0 {
			t.Fatalf("rows=%d err=%v", len(rows), err)
		}
		t.Log("minute rows:", len(rows), "date:", rows[0].Date)
	})
	t.Run("NorthboundSummary", func(t *testing.T) {
		rows, err := sdk.Northbound.Summary(ctx)
		if err != nil || len(rows) == 0 {
			t.Fatalf("rows=%d err=%v", len(rows), err)
		}
		t.Log("summary rows:", len(rows))
	})
	t.Run("NorthboundHistory", func(t *testing.T) {
		rows, err := sdk.Northbound.History(ctx, stockgo.NorthboundHistoryOptions{StartDate: "20240102", EndDate: "20240105"})
		if err != nil || len(rows) == 0 {
			t.Fatalf("rows=%d err=%v", len(rows), err)
		}
		t.Log("historical rows:", len(rows))
	})
	t.Run("NorthboundHoldingRank", func(t *testing.T) {
		rows, err := sdk.Northbound.HoldingRank(ctx, stockgo.NorthboundHoldingRankOptions{Period: "5day", Date: "20240105"})
		if sdkerrors.CodeOf(err) == sdkerrors.UpstreamError && strings.Contains(err.Error(), "服务器繁忙") {
			t.Skip("upstream RPT_MUTUAL_STOCK_NORTHSTA currently unavailable; SDK returns its error")
		}
		if err != nil || len(rows) == 0 {
			t.Fatalf("rows=%d err=%v", len(rows), err)
		}
		t.Log("holding rows:", len(rows))
	})
	t.Run("NorthboundIndividual", func(t *testing.T) {
		rows, err := sdk.Northbound.Individual(ctx, "600519", stockgo.NorthboundHistoryOptions{StartDate: "20260101", EndDate: time.Now().UTC().Format("20060102")})
		if err != nil || len(rows) == 0 {
			t.Fatalf("rows=%d err=%v", len(rows), err)
		}
		t.Log("individual rows:", len(rows))
	})
	t.Run("FundNav", func(t *testing.T) {
		result, err := sdk.Fund.NavHistory(ctx, "110011")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) == 0 {
			t.Fatal("empty fund NAV")
		}
		t.Log(result.Code, "NAV points:", len(result.Items))
	})
	t.Run("FundRank", func(t *testing.T) {
		result, err := sdk.Fund.RankHistory(ctx, "110011")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) == 0 {
			t.Fatal("empty fund ranking")
		}
		t.Log(result.Code, "rank points:", len(result.Items))
	})
	t.Run("FundProfile", func(t *testing.T) {
		result, err := sdk.Fund.Profile(ctx, "110011")
		if err != nil {
			t.Fatal(err)
		}
		if result.Name == nil || len(result.Managers) == 0 {
			t.Fatalf("invalid fund profile: %+v", result)
		}
		t.Log(result.Code, "managers:", len(result.Managers), "holdings:", len(result.Holdings))
	})
	t.Run("FundDividend", func(t *testing.T) {
		result, err := sdk.Fund.DividendList(ctx, stockgo.FundDividendOptions{Year: 2024})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) == 0 {
			t.Fatal("empty dividend page")
		}
		t.Log("dividend page rows:", len(result.Items), "pages:", result.TotalPages)
	})
	themeCode := ""
	t.Run("ThemeList", func(t *testing.T) {
		result, err := sdk.Fund.Theme.GetThemeList(ctx, stockgo.ThemeListOptions{PageSize: 5})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) == 0 {
			t.Fatal("empty themes")
		}
		themeCode = result.Items[0].Code
		t.Log("themes:", len(result.Items), "sample:", themeCode)
	})
	t.Run("ThemeFunds", func(t *testing.T) {
		if themeCode == "" {
			t.Fatal("theme list did not provide a code")
		}
		result, err := sdk.Fund.Theme.GetThemeFunds(ctx, themeCode, stockgo.ThemeFundsOptions{PageSize: 5})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) == 0 {
			t.Fatal("empty theme funds")
		}
		t.Log("funds:", len(result.Items), "total:", result.Total)
	})
}

func TestLiveTopicReports(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers")
	}
	sdk, err := stockgo.New(stockgo.Options{Timeout: 15 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	date := stockgo.DragonTigerDateOptions{StartDate: "20240102", EndDate: "20240102"}
	t.Run("DragonDetail", func(t *testing.T) {
		rows, e := sdk.DragonTiger.Detail(ctx, date)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].Code, rows[0].Date, rows[0].Reason)
	})
	t.Run("DragonStats", func(t *testing.T) {
		rows, e := sdk.DragonTiger.StockStats(ctx)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].Code)
	})
	t.Run("DragonInstitution", func(t *testing.T) {
		rows, e := sdk.DragonTiger.Institution(ctx, date)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows))
	})
	t.Run("DragonBranch", func(t *testing.T) {
		rows, e := sdk.DragonTiger.BranchRank(ctx)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].Name)
	})
	t.Run("DragonSeats", func(t *testing.T) {
		rows, e := sdk.DragonTiger.SeatDetail(ctx, "000595", "20240102")
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].BranchName)
	})
	t.Run("BlockMarket", func(t *testing.T) {
		rows, e := sdk.BlockTrade.MarketStat(ctx)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "date", rows[0].Date)
	})
	blockDate := stockgo.BlockTradeDateOptions{StartDate: "20240102", EndDate: "20240102"}
	t.Run("BlockDetail", func(t *testing.T) {
		rows, e := sdk.BlockTrade.Detail(ctx, blockDate)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].Code)
	})
	t.Run("BlockDaily", func(t *testing.T) {
		rows, e := sdk.BlockTrade.DailyStat(ctx, blockDate)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows))
	})
	t.Run("MarginAccount", func(t *testing.T) {
		rows, e := sdk.Margin.AccountInfo(ctx)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "date", rows[0].Date)
	})
	t.Run("MarginTarget", func(t *testing.T) {
		rows, e := sdk.Margin.TargetList(ctx, "20240102")
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].Code)
	})
	t.Run("Pool", func(t *testing.T) {
		rows, e := sdk.MarketEvent.ZTPool(ctx)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].Code)
	})
	t.Run("AllChanges", func(t *testing.T) {
		rows, e := sdk.MarketEvent.StockChanges(ctx, stockgo.StockChangesOptions{Types: []stockgo.StockChangeType{stockgo.AllChanges}})
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		types := map[stockgo.StockChangeType]int{}
		for _, r := range rows {
			types[r.ChangeType]++
		}
		if len(types) < 2 {
			t.Fatal("multi-type query returned only one type", types)
		}
		t.Log("events", len(rows), "types", len(types))
	})
	t.Run("BoardChanges", func(t *testing.T) {
		rows, e := sdk.MarketEvent.BoardChanges(ctx)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].Name)
	})
	t.Run("Individual", func(t *testing.T) {
		day, e := sdk.MarketEvent.IndividualDay(ctx, "600519")
		if e != nil || !day.Available || day.Name == "" {
			t.Fatal(day, e)
		}
		t.Log(day.Code, day.Date, "events", len(day.Changes))
	})
	t.Run("IndividualHistory", func(t *testing.T) {
		history, e := sdk.MarketEvent.IndividualChangesHistory(ctx, "600519", 7)
		if e != nil || len(history.Days) == 0 || history.Name == "" {
			t.Fatal(history, e)
		}
		t.Log("days", len(history.Days), "availableFrom", history.Coverage.AvailableFrom)
	})
	t.Run("Unusual", func(t *testing.T) {
		rows, e := sdk.MarketEvent.UnusualFluctuation(ctx, stockgo.UnusualFluctuationOptions{StartDate: time.Now().AddDate(0, 0, -7).Format("20060102")})
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		if rows[0].Timestamp == nil {
			t.Fatal("missing timestamp")
		}
		t.Log("rows", len(rows), "first", rows[0].Code, rows[0].Date)
	})
}

func TestLiveDerivatives(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers")
	}
	sdk, err := stockgo.New(stockgo.Options{Timeout: 15 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	start := time.Now().AddDate(0, 0, -14).Format("20060102")
	t.Run("Domestic", func(t *testing.T) {
		rows, e := sdk.Futures.Kline(ctx, "RBM", stockgo.FuturesKlineOptions{StartDate: start})
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("bars", len(rows), "name", rows[0].Name, "openInterest", rows[0].OpenInterest)
	})
	t.Run("GlobalSpot", func(t *testing.T) {
		rows, e := sdk.Futures.GlobalSpot(ctx)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("contracts", len(rows), "first", rows[0].Code)
	})
	t.Run("GlobalKline", func(t *testing.T) {
		rows, e := sdk.Futures.GlobalKline(ctx, "GC00Y", stockgo.GlobalFuturesKlineOptions{StartDate: start})
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("bars", len(rows), "first", rows[0].Name)
	})
	t.Run("InventorySymbols", func(t *testing.T) {
		rows, e := sdk.Futures.InventorySymbols(ctx)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("varieties", len(rows))
	})
	t.Run("Inventory", func(t *testing.T) {
		rows, e := sdk.Futures.Inventory(ctx, "cu", stockgo.FuturesInventoryOptions{StartDate: start})
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].Date)
	})
	t.Run("ComexGold", func(t *testing.T) {
		rows, e := sdk.Futures.ComexInventory(ctx, "gold")
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows), "first", rows[0].Date)
	})
	t.Run("CFFEX", func(t *testing.T) {
		rows, e := sdk.Options.CFFEXQuotes(ctx)
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("contracts", len(rows), "first", rows[0].Code)
	})
	t.Run("IndexSpot", func(t *testing.T) {
		rows, e := sdk.Options.IndexSpot(ctx, stockgo.OptionIO, "io2612")
		if e != nil {
			t.Fatal(e)
		}
		if len(rows.Calls) == 0 || len(rows.Puts) == 0 {
			t.Fatal(rows)
		}
		t.Log("calls", len(rows.Calls), "puts", len(rows.Puts), "first", rows.Calls[0].Symbol)
	})
	t.Run("IndexKline", func(t *testing.T) {
		rows, e := sdk.Options.IndexKline(ctx, "io2612C4000")
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("bars", len(rows))
	})
	t.Run("CommoditySpot", func(t *testing.T) {
		rows, e := sdk.Options.CommoditySpot(ctx, "au", "au2612")
		if e != nil {
			t.Fatal(e)
		}
		if len(rows.Calls) == 0 || len(rows.Puts) == 0 {
			t.Fatal(rows)
		}
		t.Log("calls", len(rows.Calls), "first", rows.Calls[0].Symbol)
	})
	t.Run("CommodityKline", func(t *testing.T) {
		rows, e := sdk.Options.CommodityKline(ctx, "au2612C1000")
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("bars", len(rows))
	})
	t.Run("ETFMonths", func(t *testing.T) {
		rows, e := sdk.Options.ETFMonths(ctx, stockgo.ETF50)
		if e != nil || len(rows.Months) == 0 {
			t.Fatal(rows, e)
		}
		t.Log(rows.Months, rows.StockId)
	})
	t.Run("ETFExpire", func(t *testing.T) {
		rows, e := sdk.Options.ETFExpireDay(ctx, stockgo.ETF50, "2026-12")
		if e != nil || rows.ExpireDay == "" {
			t.Fatal(rows, e)
		}
		t.Log(rows.ExpireDay, rows.RemainderDays)
	})
	t.Run("ETFDaily", func(t *testing.T) {
		rows, e := sdk.Options.ETFDailyKline(ctx, "10009633")
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("bars", len(rows))
	})
	t.Run("ETFMinute", func(t *testing.T) {
		rows, e := sdk.Options.ETFMinute(ctx, "10011430")
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("minutes", len(rows), "first", rows[0].Date, rows[0].Time)
	})
	t.Run("ETF5Day", func(t *testing.T) {
		rows, e := sdk.Options.ETF5DayMinute(ctx, "10011430")
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		dates := map[string]int{}
		for _, r := range rows {
			if r.Date == "" {
				t.Fatal("missing inherited trading date")
			}
			dates[r.Date]++
		}
		if len(dates) < 2 {
			t.Fatal("missing multiple days", dates)
		}
		t.Log("minutes", len(rows), "days", len(dates))
	})
	t.Run("OptionLHB", func(t *testing.T) {
		rows, e := sdk.Options.LHB(ctx, "510050", "2024-01-02")
		if e != nil || len(rows) == 0 {
			t.Fatal(len(rows), e)
		}
		t.Log("rows", len(rows))
	})
}

func TestLiveRequestGovernance(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers")
	}
	var mu sync.Mutex
	requests := map[string]int{}
	responses := 0
	sdk, e := stockgo.New(stockgo.Options{Request: &stockgo.RequestOptions{Timeout: 10 * time.Second, RateLimit: &stockgo.RateLimitOptions{RequestsPerSecond: 20, MaxBurst: 2}, CircuitBreaker: &stockgo.CircuitBreakerOptions{FailureThreshold: 3}, Hooks: &stockgo.RequestHooks{OnRequest: func(rc stockgo.RequestContext) { mu.Lock(); requests[rc.Provider]++; mu.Unlock() }, OnResponse: func(rc stockgo.RequestContext, meta stockgo.ResponseMeta) { mu.Lock(); responses++; mu.Unlock() }}}})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	quotes, e := sdk.Quotes.CNSimple(ctx, []string{"600519"})
	if e != nil || len(quotes) == 0 {
		t.Fatal(len(quotes), e)
	}
	calendar, e := sdk.Reference.TradingCalendar(ctx)
	if e != nil || len(calendar) == 0 {
		t.Fatal(len(calendar), e)
	}
	bars, e := sdk.Futures.Kline(ctx, "RBM", stockgo.FuturesKlineOptions{StartDate: time.Now().AddDate(0, 0, -7).Format("20060102")})
	if e != nil || len(bars) == 0 {
		t.Fatal(len(bars), e)
	}
	options, e := sdk.Options.IndexKline(ctx, "io2612C4000")
	if e != nil || len(options) == 0 {
		t.Fatal(len(options), e)
	}
	for _, provider := range []string{"tencent", "linkdiary", "eastmoney", "sina"} {
		if requests[provider] == 0 {
			t.Fatal("missing request hook for", provider)
		}
		state, enabled := sdk.Requests.CircuitStats(provider)
		if !enabled || state.State != stockgo.CircuitClosed {
			t.Fatal(provider, state, enabled)
		}
	}
	if responses < 4 || len(sdk.Requests.HostHealth("")) < 4 {
		t.Fatal(responses, sdk.Requests.HostHealth(""))
	}
	t.Log("requests", requests, "responses", responses, "healthy hosts", len(sdk.Requests.HostHealth("")))
}

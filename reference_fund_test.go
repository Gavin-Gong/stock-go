package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"github.com/Gavin-Gong/stock-go/symbols"
)

func TestDatacenterWavesMetadataAndLimits(t *testing.T) {
	for _, mode := range []string{"all", "bad-page", "empty-page", "single", "max", "start"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			calls := []int{}
			sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("reportName") != "REPORT" || q.Get("columns") != "CODE" || q.Get("source") != "WEB" || q.Get("client") != "WEB" || q.Get("quoteType") != "0" || q.Get("filter") != "(A=1)" || q.Get("extra") != "yes" {
					t.Error(q)
				}
				p, _ := strconv.Atoi(q.Get("pageNumber"))
				mu.Lock()
				calls = append(calls, p)
				mu.Unlock()
				if mode == "bad-page" && p == 2 {
					fmt.Fprint(w, `{"result":null}`)
					return
				}
				rows := []map[string]int{{"p": p}}
				if mode == "empty-page" && p == 2 {
					rows = []map[string]int{}
				}
				json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"pages": 4, "count": 400, "data": rows}})
			})
			q := DatacenterQuery{ReportName: "REPORT", Columns: "CODE", Filter: "(A=1)", QuoteType: "0", QuoteColumns: "quote", ExtraParams: map[string]string{"extra": "yes"}}
			if mode == "single" {
				q.FetchAllPages = Bool(false)
			}
			if mode == "max" {
				q.MaxPages = 2
			}
			if mode == "start" {
				q.StartPage = 3
			}
			result, err := sdk.Datacenter.Query(context.Background(), q)
			sort.Ints(calls)
			switch mode {
			case "max":
				if sdkerrors.CodeOf(err) != sdkerrors.UpstreamError || !reflect.DeepEqual(calls, []int{1, 2}) {
					t.Fatalf("%v %v", calls, err)
				}
				return
			case "all":
				if err != nil || len(result.Data) != 4 || !reflect.DeepEqual(calls, []int{1, 2, 3, 4}) {
					t.Fatalf("%+v %v %v", result, calls, err)
				}
			case "bad-page":
				if err != nil || len(result.Data) != 1 || !reflect.DeepEqual(calls, []int{1, 2, 3, 4}) {
					t.Fatalf("%+v %v %v", result, calls, err)
				}
			case "empty-page":
				if err != nil || len(result.Data) != 3 {
					t.Fatalf("%+v %v", result, err)
				}
			case "single":
				if err != nil || len(result.Data) != 1 || !reflect.DeepEqual(calls, []int{1}) {
					t.Fatalf("%+v %v %v", result, calls, err)
				}
			case "start":
				if err != nil || len(result.Data) != 2 || !reflect.DeepEqual(calls, []int{3, 4}) {
					t.Fatalf("%+v %v %v", result, calls, err)
				}
			}
			if result.Total != 400 || result.Pages != 4 {
				t.Fatalf("metadata: %+v", result)
			}
		})
	}
}
func TestCalendarCacheDatesAndBoundaries(t *testing.T) {
	calls := 0
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, "2024-01-05, 2024-01-02,invalid,2024-01-03")
	})
	ctx := context.Background()
	if valid, err := sdk.Calendar.IsTradingDay(ctx, "2024-01-02T23:30:00Z"); err != nil || !valid {
		t.Fatal(valid, err)
	}
	list, _ := sdk.Reference.TradingCalendar(ctx)
	list[0] = "tampered"
	again, _ := sdk.Reference.TradingCalendar(ctx)
	if again[0] != "2024-01-02" || calls != 1 {
		t.Fatal(again, calls)
	}
	if next, err := sdk.Calendar.NextTradingDay(ctx, "20240102"); err != nil || next != "2024-01-03" {
		t.Fatal(next, err)
	}
	if prev, err := sdk.Calendar.PrevTradingDay(ctx, "20240104"); err != nil || prev != "2024-01-03" {
		t.Fatal(prev, err)
	}
	if _, err := sdk.Calendar.PrevTradingDay(ctx, "20240102"); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := sdk.Calendar.NextTradingDay(ctx, "20240105"); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
	sdk.ClearCaches()
	sdk.Reference.TradingCalendar(ctx)
	if calls != 2 {
		t.Fatal("calendar cache not cleared")
	}
	for _, at := range []struct {
		iso  string
		want MarketStatus
	}{{"2024-01-02T14:30:00Z", MarketOpen}, {"2024-01-02T13:30:00Z", PreMarket}} {
		now, _ := time.Parse(time.RFC3339, at.iso)
		got, err := sdk.Calendar.MarketStatus(symbols.US, now)
		if err != nil || got != at.want {
			t.Fatal(got, err)
		}
	}
}
func TestNorthboundAndDividendFilters(t *testing.T) {
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		report := q.Get("reportName")
		filter := q.Get("filter")
		switch report {
		case "RPT_MUTUAL_QUOTA":
			if q.Get("pageSize") != "2000" || q.Get("quoteType") != "0" || !strings.Contains(q.Get("quoteColumns"), "netBuyAmt") {
				t.Error(q)
			}
		case "RPT_MUTUAL_STOCK_NORTHSTA":
			if filter != `(INTERVAL_TYPE="M")(TRADE_DATE='2024-01-02')(MUTUAL_TYPE="003")` {
				t.Error(filter)
			}
		case "RPT_MUTUAL_DEAL_HISTORY":
			if filter != `(BOARD_TYPE="0")(TRADE_DATE>='2024-01-01')(TRADE_DATE<='2024-01-31')` {
				t.Error(filter)
			}
		case "RPT_MUTUAL_HOLDSTOCKNORTH_STA":
			if filter != `(SECURITY_CODE="600519")(TRADE_DATE>='2024-01-01')` {
				t.Error(filter)
			}
		case "RPT_SHAREBONUS_DET":
			if filter != `(SECURITY_CODE="600519")` || q.Get("sortColumns") != "REPORT_DATE" {
				t.Error(q)
			}
		default:
			t.Error(report)
		}
		fmt.Fprint(w, `{"result":{"pages":1,"count":0,"data":[]}}`)
	})
	ctx := context.Background()
	sdk.Northbound.Summary(ctx)
	sdk.Northbound.HoldingRank(ctx, NorthboundHoldingRankOptions{Market: "shenzhen", Period: "month", Date: "20240102"})
	sdk.Northbound.History(ctx, NorthboundHistoryOptions{Direction: South, StartDate: "20240101", EndDate: "20240131"})
	sdk.Northbound.Individual(ctx, "600519.SH", NorthboundHistoryOptions{StartDate: "20240101"})
	sdk.Reference.DividendDetail(ctx, "sh600519")
}
func TestFundLiteralSafetyStrictNumbersAndQueries(t *testing.T) {
	calls := 0
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/ping/110011.js" {
			t.Error(r.URL.Path)
		}
		fmt.Fprint(w, `/*var fS_code="evil";*/ var fS_name="基金";var Data_netWorthTrend=[{"x":1704124800000,"y":"1.2%","equityReturn":"--"},{"x":"1704211200000","y":2},{"y":99},null];var Data_ACWorthTrend=[[1704124800000,"2.5abc"]]; malicious();`)
	})
	q, err := sdk.Fund.NavHistory(context.Background(), "110011")
	if err != nil || len(q.Items) != 1 || q.Code != "110011" || q.Items[0].NAV != nil || q.Items[0].AccNAV != nil || q.Items[0].Date != "2024-01-02" {
		t.Fatalf("%+v %v", q, err)
	}
	if _, err := sdk.Fund.NavHistory(context.Background(), "1/../evil"); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument || calls != 1 {
		t.Fatal(err, calls)
	}
	for _, input := range []string{`"1.5%"`, `"1.5x"`, `"NaN"`, `"Infinity"`, `"--"`, `true`, `"-0x1p3"`} {
		if n := strictNumber(json.RawMessage(input)); n != nil {
			t.Fatal(input, *n)
		}
	}
	if n := strictNumber(json.RawMessage(`"0x10"`)); n == nil || *n != 16 {
		t.Fatal(n)
	}
}
func TestFundDividendPaginationFilterAndLimit(t *testing.T) {
	calls := 0
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("year") != "2024" || q.Get("ftype") != "混合型" || q.Get("rank") != "FSRQ" || q.Get("dt") != "8" {
			t.Error(q)
		}
		p, _ := strconv.Atoi(q.Get("page"))
		code := "000001"
		if p == 2 {
			code = "110011"
		}
		fmt.Fprintf(w, `var pageinfo=[3,1,%d];var jjfh_data=[["%s","基金","2024-01-02","2024-01-03","0.5","2024-01-04",""]];`, p, code)
	})
	q, err := sdk.Fund.DividendList(context.Background(), FundDividendOptions{Year: 2024, Code: "110011", FundType: "混合型", AllPages: true})
	if err != nil || len(q.Items) != 1 || calls != 3 || q.CurrentPage != -1 {
		t.Fatalf("%+v %v calls=%d", q, err, calls)
	}
	_, err = sdk.Fund.DividendList(context.Background(), FundDividendOptions{Year: 2024, FundType: "混合型", AllPages: true, MaxPages: 2})
	if sdkerrors.CodeOf(err) != sdkerrors.UpstreamError || calls != 4 {
		t.Fatal(err, calls)
	}
}
func TestReferenceFundErrorsAndValidation(t *testing.T) {
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input caused HTTP") })
	ctx := context.Background()
	for _, run := range []func() error{
		func() error { _, e := sdk.Calendar.IsTradingDay(ctx, "20240230"); return e }, func() error { _, e := sdk.Calendar.MarketStatus(symbols.Global); return e }, func() error { _, e := sdk.Calendar.IsTradingDay(ctx, "a", "b"); return e }, func() error { _, e := sdk.Datacenter.Query(ctx, DatacenterQuery{}); return e }, func() error {
			_, e := sdk.Datacenter.Query(ctx, DatacenterQuery{ReportName: "R", PageSize: -1})
			return e
		},
		func() error { _, e := sdk.Northbound.Minute(ctx, "bad"); return e }, func() error {
			_, e := sdk.Northbound.HoldingRank(ctx, NorthboundHoldingRankOptions{Market: "bad"})
			return e
		}, func() error {
			_, e := sdk.Northbound.History(ctx, NorthboundHistoryOptions{StartDate: "20240102", EndDate: "20240101"})
			return e
		},
		func() error { _, e := sdk.Fund.Profile(ctx, "bad"); return e }, func() error { _, e := sdk.Fund.DividendList(ctx, FundDividendOptions{Sort: "bad"}); return e }, func() error { _, e := sdk.Fund.Theme.GetThemeList(ctx, ThemeListOptions{PageSize: 501}); return e }, func() error { _, e := sdk.Fund.Theme.GetThemeFunds(ctx, ""); return e },
	} {
		if err := run(); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatal(err)
		}
	}
}
func TestMissingPayloadAndUpstreamErrors(t *testing.T) {
	for _, body := range []string{`{`, `{"success":false,"message":"bad report"}`, `{"result":null}`} {
		t.Run(body, func(t *testing.T) {
			sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
			out, err := sdk.Datacenter.Query(context.Background(), DatacenterQuery{ReportName: "R"})
			if body == `{"result":null}` {
				if err != nil || out.Data == nil || len(out.Data) != 0 {
					t.Fatal(out, err)
				}
			} else if err == nil {
				t.Fatal("expected error")
			}
		})
	}
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `not data`) })
	if _, err := sdk.Fund.Profile(context.Background(), "110011"); sdkerrors.CodeOf(err) != sdkerrors.UpstreamEmpty {
		t.Fatal(err)
	}
}

func TestNorthboundCurrentHistorySchema(t *testing.T) {
	legacy, modern := 0, 0
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		if strings.Contains(filter, "BOARD_TYPE") {
			legacy++
			fmt.Fprint(w, `{"success":false,"message":"BOARD_TYPE列不存在"}`)
			return
		}
		modern++
		if !strings.Contains(filter, `(MUTUAL_TYPE="005")`) && !strings.Contains(filter, `(MUTUAL_TYPE="006")`) {
			t.Error(filter)
		}
		fmt.Fprint(w, `{"result":{"pages":1,"data":[{"TRADE_DATE":"2024-01-02","NET_DEAL_AMT":null}]}}`)
	})
	if rows, err := sdk.Northbound.History(context.Background()); err != nil || len(rows) != 1 || rows[0].NetBuyAmount != nil {
		t.Fatalf("%v %v", rows, err)
	}
	if _, err := sdk.Northbound.History(context.Background(), NorthboundHistoryOptions{Direction: South}); err != nil {
		t.Fatal(err)
	}
	if legacy != 1 || modern != 2 {
		t.Fatal(legacy, modern)
	}
	var row emRow
	json.Unmarshal([]byte(`{"HOLD_RATIO":null,"FREE_SHARES_RATIO":1.25,"TOTAL_SHARES_RATIO":2.5}`), &row)
	result := mapNorthIndividual(row)
	assertNum(t, result.HoldRatioFloat, 1.25)
	assertNum(t, result.HoldRatioTotal, 2.5)
}
func TestCurrentThemeCatalogAndUppercaseFields(t *testing.T) {
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("plat") != "Android" || q.Get("appType") != "ttjj" || q.Get("deviceid") != "stock-go" {
			t.Error(q)
		}
		if strings.HasSuffix(r.URL.Path, "FundMNSubjectList") {
			fmt.Fprint(w, `{"Success":true,"Datas":[{"INDEXCODE":"BK0001","INDEXNAME":"行业","TYPE":"1"},{"INDEXCODE":"BK0002","INDEXNAME":"概念","TYPE":"2"}],"TotalCount":2}`)
		} else {
			if q.Get("TOPICAL") != "BK0002" || q.Get("SortColumn") != "SYL_1N" {
				t.Error(q)
			}
			fmt.Fprint(w, `{"Success":true,"Datas":[{"FCODE":"110011","SHORTNAME":"基金","FUNDTYPE":"001","RZDF":"1.5","DWJZ":"1.2"}],"TotalCount":55}`)
		}
	})
	ctx := context.Background()
	result, err := sdk.Fund.Theme.GetThemeList(ctx, ThemeListOptions{Page: 2, PageSize: 1})
	if err != nil || len(result.Items) != 1 || result.Items[0].Code != "BK0002" || result.Items[0].Type != "概念" || !result.CatalogOnly || result.TotalPages != 2 || result.Items[0].DailyChange != nil {
		t.Fatalf("%+v %v", result, err)
	}
	filtered, err := sdk.Fund.Theme.GetThemeList(ctx, ThemeListOptions{Category: "0", PageSize: 5})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Code != "BK0001" || filtered.TotalPages != 1 {
		t.Fatalf("%+v %v", filtered, err)
	}
	empty, err := sdk.Fund.Theme.GetThemeList(ctx, ThemeListOptions{Page: 3, PageSize: 1})
	if err != nil || len(empty.Items) != 0 || empty.TotalPages != 2 {
		t.Fatalf("%+v %v", empty, err)
	}
	funds, err := sdk.Fund.Theme.GetThemeFunds(ctx, "BK0002")
	if err != nil || len(funds.Items) != 1 || funds.Total != 55 || funds.Items[0].Code != "110011" || funds.Items[0].Name != "基金" {
		t.Fatalf("%+v %v", funds, err)
	}
	assertNum(t, funds.Items[0].NAV, 1.2)
	assertNum(t, funds.Items[0].DailyChange, 1.5)
}
func TestThemeReportedFailure(t *testing.T) {
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Success":false,"ErrCode":1,"ErrMsg":"upstream unavailable"}`)
	})
	if _, err := sdk.Fund.Theme.GetThemeList(context.Background()); sdkerrors.CodeOf(err) != sdkerrors.UpstreamError {
		t.Fatal(err)
	}
	if _, err := sdk.Fund.Theme.GetThemeFunds(context.Background(), "BK0001"); sdkerrors.CodeOf(err) != sdkerrors.UpstreamError {
		t.Fatal(err)
	}
}

func TestConcurrentNorthboundSchemaDiscovery(t *testing.T) {
	started, release := make(chan struct{}, 2), make(chan struct{})
	sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("filter"), "BOARD_TYPE") {
			started <- struct{}{}
			<-release
			fmt.Fprint(w, `{"success":false,"message":"BOARD_TYPE列不存在"}`)
			return
		}
		fmt.Fprint(w, `{"result":{"pages":1,"data":[{"TRADE_DATE":"2024-01-02"}]}}`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := sdk.Northbound.History(ctx); results <- err }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			close(release)
			t.Fatal("schema probes did not start")
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

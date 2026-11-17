package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestTopicPaging(t *testing.T) {
	for _, mode := range []string{"total", "short", "empty", "limit", "single", "defaultSingle"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				p, _ := strconv.Atoi(q.Get("pageindex"))
				size, _ := strconv.Atoi(q.Get("pagesize"))
				if mode == "single" || mode == "defaultSingle" {
					if p != 2 {
						t.Error(q)
					}
					want := 7
					if mode == "defaultSingle" {
						want = 100
					}
					if size != want {
						t.Error(q)
					}
				} else if p != calls || size != 5000 {
					t.Error(q, calls)
				}
				calls++
				n := 5000
				total := 20000
				if mode == "total" {
					total = 10000
				}
				if p == 1 && mode == "short" {
					n = 2
				}
				if p == 1 && mode == "empty" {
					n = 0
				}
				if mode == "single" || mode == "defaultSingle" {
					n = 1
				}
				rows := make([]map[string]any, n)
				for i := range rows {
					rows[i] = map[string]any{"c": strconv.Itoa(p*5000 + i), "tm": 93000}
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"tc": total, "allstock": rows}})
			})
			o := StockChangesOptions{MaxPages: 2}
			if mode == "single" {
				o.Page = 3
				o.PageSize = 7
			}
			if mode == "defaultSingle" {
				o.Page = 3
			}
			rows, err := sdk.MarketEvent.StockChanges(context.Background(), o)
			if mode == "limit" {
				if sdkerrors.CodeOf(err) != sdkerrors.UpstreamError || rows != nil || calls != 2 {
					t.Fatal(len(rows), calls, err)
				}
				return
			}
			want := 10000
			if mode == "short" {
				want = 5002
			}
			if mode == "empty" {
				want = 5000
			}
			if mode == "single" || mode == "defaultSingle" {
				want = 1
			}
			if err != nil || len(rows) != want {
				t.Fatal(len(rows), want, err)
			}
			if want > 1 && rows[5000-1].Code != "4999" {
				t.Fatal("page order")
			}
			if rows[0].ChangeType != LargeBuy {
				t.Fatal("missing t must use the single requested type")
			}
		})
	}
}
func TestSeatPagingRanksAndOrder(t *testing.T) {
	var active, peak atomic.Int32
	sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(3 * time.Millisecond)
		q := r.URL.Query()
		p, _ := strconv.Atoi(q.Get("pageNumber"))
		side := "buy"
		if strings.HasSuffix(q.Get("reportName"), "SELL") {
			side = "sell"
		}
		if q.Get("filter") != `(SECURITY_CODE="600519")(TRADE_DATE='2024-01-02')` {
			t.Error(q)
		}
		fmt.Fprintf(w, `{"result":{"pages":3,"count":3,"data":[{"OPERATEDEPT_NAME":"%s-%d","RANK":null}]}}`, side, p)
	})
	rows, err := sdk.DragonTiger.SeatDetail(context.Background(), "1.600519", "20240102")
	if err != nil || len(rows) != 6 {
		t.Fatal(rows, err)
	}
	for i, r := range rows {
		side := "buy"
		if i >= 3 {
			side = "sell"
		}
		if r.Side != side || r.Rank == nil || *r.Rank != float64(i%3+1) || r.BranchName != fmt.Sprintf("%s-%d", side, i%3+1) {
			t.Fatal(rows)
		}
	}
	if peak.Load() < 2 || peak.Load() > 6 {
		t.Fatal("unexpected concurrency", peak.Load())
	}
}
func TestIndividualHistoryAvailabilityEchoAndStats(t *testing.T) {
	today, _ := calendarDate(nil)
	dates := []string{}
	for i := 4; i >= 0; i-- {
		dates = append(dates, dateAdd(today, -i))
	}
	var mu sync.Mutex
	requests := []string{}
	var active, peak atomic.Int32
	sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/calendar" {
			fmt.Fprint(w, strings.Join(dates, ","))
			return
		}
		d := r.URL.Query().Get("date")
		mu.Lock()
		requests = append(requests, d)
		mu.Unlock()
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(4 * time.Millisecond)
		if d == compactDate(dates[1]) {
			fmt.Fprint(w, `{"data":null}`)
			return
		}
		echo := d
		if d == compactDate(dates[2]) {
			echo = compactDate(dates[0])
		}
		events := []map[string]any{}
		name := ""
		if d != compactDate(dates[0]) {
			name = "茅台"
			events = []map[string]any{{"t": 8193}, {"t": "constructor"}, {"t": ""}, {"t": 8193}}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"d": echo, "c": "600519", "n": name, "data": events}})
	})
	got, err := sdk.MarketEvent.IndividualChangesHistory(context.Background(), "sh600519", 5)
	if err != nil {
		t.Fatal(err)
	}
	if got.Coverage.From != dates[0] || got.Coverage.To != today || got.Coverage.AvailableFrom == nil || *got.Coverage.AvailableFrom != dates[0] || got.Code != "600519" || got.Name != "茅台" || got.RequestedDays != 5 {
		t.Fatalf("%+v", got)
	}
	if len(got.Days) != 5 || len(requests) != 5 || peak.Load() > 4 || peak.Load() < 2 {
		t.Fatal(got, requests, peak.Load())
	}
	for i, day := range got.Days {
		if day.Date != dates[i] {
			t.Fatal(day)
		}
		if day.Available != (i != 1 && i != 2) {
			t.Fatal(day)
		}
		if (i == 1 || i == 2) && (len(day.Changes) != 0 || day.Name != "") {
			t.Fatal(day)
		}
	}
	want := map[string]ChangeTypeCount{"8193": {Count: 4, Label: "大笔买入"}, "constructor": {Count: 2}, "unknown": {Count: 2}}
	if !reflect.DeepEqual(got.Stats, want) {
		t.Fatal(got.Stats)
	}
}
func TestIndividualHistoryEmptyCalendarAndFailure(t *testing.T) {
	for _, mode := range []string{"empty", "failure", "calendarFailure"} {
		t.Run(mode, func(t *testing.T) {
			today, _ := calendarDate(nil)
			calls := 0
			sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/calendar" {
					if mode == "empty" {
						fmt.Fprint(w, "2024-01-02")
						return
					}
					if mode == "calendarFailure" {
						fmt.Fprint(w, "invalid")
						return
					}
					fmt.Fprint(w, today)
					return
				}
				fmt.Fprint(w, `{"rc":1,"data":null}`)
			})
			result, err := sdk.MarketEvent.IndividualChangesHistory(context.Background(), "600519", 1)
			if mode != "empty" {
				if err == nil || result != nil {
					t.Fatal(result, err)
				}
				return
			}
			if err != nil || result.Code != "600519" || result.Days == nil || result.Stats == nil || len(result.Days) != 0 || result.Coverage.AvailableFrom != nil || calls != 1 {
				t.Fatal(result, err, calls)
			}
		})
	}
}
func TestTopicAndReportValidationBeforeRequests(t *testing.T) {
	calls := 0
	sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, `{}`) })
	ctx := context.Background()
	checks := []func() error{
		func() error { _, e := sdk.MarketEvent.ZTPool(ctx, ZTPoolOptions{Type: "bad"}); return e },
		func() error { _, e := sdk.MarketEvent.ZTPool(ctx, ZTPoolOptions{Date: "20240230"}); return e },
		func() error { _, e := sdk.MarketEvent.ZTPool(ctx, ZTPoolOptions{}, ZTPoolOptions{}); return e },
		func() error {
			_, e := sdk.MarketEvent.StockChanges(ctx, StockChangesOptions{Types: []StockChangeType{}})
			return e
		},
		func() error {
			_, e := sdk.MarketEvent.StockChanges(ctx, StockChangesOptions{Types: []StockChangeType{AllChanges, LargeBuy}})
			return e
		},
		func() error {
			_, e := sdk.MarketEvent.StockChanges(ctx, StockChangesOptions{}, StockChangesOptions{})
			return e
		},
		func() error { _, e := sdk.MarketEvent.StockChanges(ctx, StockChangesOptions{Page: -1}); return e },
		func() error { _, e := sdk.MarketEvent.StockChanges(ctx, StockChangesOptions{PageSize: 5001}); return e },
		func() error { _, e := sdk.MarketEvent.StockChanges(ctx, StockChangesOptions{MaxPages: -1}); return e },
		func() error {
			_, e := sdk.MarketEvent.IndividualChanges(ctx, "600519", "20240101", "20240102")
			return e
		},
		func() error { _, e := sdk.MarketEvent.IndividualChanges(ctx, "600519", "invalid"); return e },
		func() error { _, e := sdk.MarketEvent.IndividualChanges(ctx, "930903"); return e },
		func() error { _, e := sdk.MarketEvent.IndividualChangesHistory(ctx, "600519", 61); return e },
		func() error { _, e := sdk.MarketEvent.IndividualChangesHistory(ctx, "600519", 0); return e },
		func() error { _, e := sdk.MarketEvent.IndividualChangesHistory(ctx, "600519", 1, 2); return e },
		func() error { _, e := sdk.MarketEvent.IndividualChangesHistory(ctx, "930903"); return e },
		func() error {
			_, e := sdk.MarketEvent.UnusualFluctuation(ctx, UnusualFluctuationOptions{Date: "20240102", StartDate: "20240101"})
			return e
		},
		func() error {
			_, e := sdk.MarketEvent.UnusualFluctuation(ctx, UnusualFluctuationOptions{Date: "bad"})
			return e
		},
		func() error {
			_, e := sdk.MarketEvent.UnusualFluctuation(ctx, UnusualFluctuationOptions{}, UnusualFluctuationOptions{})
			return e
		},
		func() error { _, e := sdk.DragonTiger.Detail(ctx, DragonTigerDateOptions{}); return e },
		func() error {
			_, e := sdk.DragonTiger.Detail(ctx, DragonTigerDateOptions{StartDate: "20240103", EndDate: "20240102"})
			return e
		},
		func() error {
			_, e := sdk.DragonTiger.Institution(ctx, DragonTigerDateOptions{StartDate: "bad", EndDate: "20240102"})
			return e
		},
		func() error {
			_, e := sdk.DragonTiger.Detail(ctx, DragonTigerDateOptions{StartDate: "20240101", EndDate: "bad"})
			return e
		},
		func() error { _, e := sdk.DragonTiger.StockStats(ctx, "bad"); return e },
		func() error { _, e := sdk.DragonTiger.BranchRank(ctx, DragonMonth, DragonYear); return e },
		func() error { _, e := sdk.DragonTiger.SeatDetail(ctx, "600519", "bad"); return e },
		func() error {
			_, e := sdk.BlockTrade.Detail(ctx, BlockTradeDateOptions{}, BlockTradeDateOptions{})
			return e
		},
		func() error { _, e := sdk.BlockTrade.DailyStat(ctx, BlockTradeDateOptions{EndDate: "bad"}); return e },
		func() error { _, e := sdk.Margin.TargetList(ctx, "bad"); return e },
		func() error { _, e := sdk.Margin.TargetList(ctx, "20240101", "20240102"); return e },
	}
	for i, check := range checks {
		if err := check(); err == nil {
			t.Errorf("check %d accepted invalid options", i)
		}
	}
	if calls != 0 {
		t.Fatal("validation performed HTTP requests", calls)
	}
	for _, symbol := range []string{"bad!", "00700.HK", "BK0475"} {
		if _, e := sdk.MarketEvent.IndividualDay(ctx, symbol); e == nil {
			t.Fatal(symbol)
		}
		if _, e := sdk.DragonTiger.SeatDetail(ctx, symbol, "20240102"); e == nil {
			t.Fatal(symbol)
		}
	}
	if calls != 0 {
		t.Fatal(calls)
	}
}
func TestTopicFailureAndCancellation(t *testing.T) {
	for _, body := range []string{`invalid`, `{"rc":5,"data":null}`} {
		sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		ctx := context.Background()
		if _, e := sdk.MarketEvent.BoardChanges(ctx); e == nil {
			t.Fatal(body)
		}
		if _, e := sdk.MarketEvent.ZTPool(ctx); e == nil {
			t.Fatal(body)
		}
		if _, e := sdk.MarketEvent.IndividualChanges(ctx, "600519"); e == nil {
			t.Fatal(body)
		}
	}
	calls := 0
	sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, `{"data":null}`) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sdk.MarketEvent.StockChanges(ctx); err == nil || calls != 0 {
		t.Fatal(err, calls)
	}
}

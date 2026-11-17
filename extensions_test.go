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

	"github.com/Gavin-Gong/stock-go/indicators"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestClistPaginationWavesAndServerCap(t *testing.T) {
	for _, mode := range []string{"all", "empty-wave", "cap", "max", "one-page"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			pages := []int{}
			sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				p, _ := strconv.Atoi(r.URL.Query().Get("pn"))
				size, _ := strconv.Atoi(r.URL.Query().Get("pz"))
				mu.Lock()
				pages = append(pages, p)
				mu.Unlock()
				total, n := 250, 100
				if mode == "empty-wave" {
					total = 1000
					if p == 2 {
						n = 0
					}
				} else if mode == "cap" && p > 1 {
					n = 50
				} else if mode == "all" && p == 3 {
					n = 50
				} else if mode == "one-page" {
					n = size
				}
				diff := make([]map[string]any, n)
				for i := range diff {
					diff[i] = map[string]any{"f12": fmt.Sprintf("BK%04d", p*100+i), "f14": fmt.Sprintf("B%d", p*100+i), "f3": float64(5000 - p*100 - i)}
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"total": total, "diff": diff}})
			})
			o := PageOptions{}
			if mode == "max" {
				o.MaxPages = 2
			}
			if mode == "one-page" {
				o.Page = 3
				o.PageSize = 25
			}
			got, err := sdk.Board.Industry.List(context.Background(), o)
			sort.Ints(pages)
			switch mode {
			case "all":
				if err != nil || len(got) != 250 || !reflect.DeepEqual(pages, []int{1, 2, 3}) {
					t.Fatalf("count=%d pages=%v error=%v", len(got), pages, err)
				}
			case "cap":
				if err != nil || len(got) != 250 || !reflect.DeepEqual(pages, []int{1, 2, 3, 4}) {
					t.Fatalf("count=%d pages=%v error=%v", len(got), pages, err)
				}
			case "empty-wave":
				if err != nil || len(got) != 100 || !reflect.DeepEqual(pages, []int{1, 2, 3, 4}) {
					t.Fatalf("count=%d pages=%v error=%v", len(got), pages, err)
				}
			case "max":
				if sdkerrors.CodeOf(err) != sdkerrors.UpstreamError || !reflect.DeepEqual(pages, []int{1, 2}) {
					t.Fatalf("pages=%v error=%v", pages, err)
				}
			case "one-page":
				if err != nil || len(got) != 25 || !reflect.DeepEqual(pages, []int{3}) {
					t.Fatalf("count=%d pages=%v error=%v", len(got), pages, err)
				}
			}
		})
	}
}
func TestBoardNameCacheAndClear(t *testing.T) {
	listCalls, spotCalls := 0, 0
	sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/list":
			listCalls++
			fmt.Fprint(w, `{"data":{"total":1,"diff":[{"f12":"BK0475","f14":"银行","f3":1}]}}`)
		case "/spot":
			spotCalls++
			if r.URL.Query().Get("secid") != "90.BK0475" {
				t.Error(r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"data":{"f43":1234}}`)
		}
	})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if got, err := sdk.Board.Industry.Spot(ctx, "银行"); err != nil || len(got) != 10 {
			t.Fatalf("%v %v", got, err)
		}
	}
	if listCalls != 1 || spotCalls != 2 {
		t.Fatal(listCalls, spotCalls)
	}
	sdk.Board.Concept.Spot(ctx, "银行")
	if listCalls != 2 {
		t.Fatal("name cache crossed board kinds")
	}
	sdk.ClearCaches()
	sdk.Board.Industry.Spot(ctx, "银行")
	if listCalls != 3 {
		t.Fatal("name cache was not cleared")
	}
	sdk.Board.Industry.Spot(ctx, "90.BK0475")
	if listCalls != 3 {
		t.Fatal("explicit secid fetched name list")
	}
	if _, err := sdk.Board.Industry.Spot(ctx, "missing"); sdkerrors.CodeOf(err) != sdkerrors.NotFound {
		t.Fatal(err)
	}
}
func TestBoardEmptyNameMapNotCached(t *testing.T) {
	calls := 0
	sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/list" {
			calls++
			if calls == 1 {
				fmt.Fprint(w, `{"data":{"total":0,"diff":[]}}`)
			} else {
				fmt.Fprint(w, `{"data":{"total":1,"diff":[{"f12":"BK0475","f14":"银行"}]}}`)
			}
		} else {
			fmt.Fprint(w, `{"data":null}`)
		}
	})
	if _, err := sdk.Board.Industry.Spot(context.Background(), "银行"); sdkerrors.CodeOf(err) != sdkerrors.UpstreamEmpty {
		t.Fatal(err)
	}
	if got, err := sdk.Board.Industry.Spot(context.Background(), "银行"); err != nil || len(got) != 0 || calls != 2 {
		t.Fatalf("%v %v calls=%d", got, err, calls)
	}
}
func TestBoardKlineParameters(t *testing.T) {
	sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("secid") != "90.BK0475" || q.Get("ut") != "" {
			t.Error(q)
		}
		switch r.URL.Path {
		case "/kline":
			if q.Get("klt") == "103" {
				if q.Get("fqt") != "0" || q.Get("beg") != "20240101" || q.Get("end") != "20240201" {
					t.Error(q)
				}
			} else if q.Get("klt") != "5" || q.Get("fqt") != "1" || q.Get("smplmt") != "10000" {
				t.Error(q)
			}
			fmt.Fprint(w, `{"data":{"klines":["2024-01-02,1,2,3,0,10,20,--,--,--,--",null]}}`)
		case "/trends":
			if q.Get("ndays") != "1" {
				t.Error(q)
			}
			fmt.Fprint(w, `{"data":{"trends":["2024-01-02 09:30,1,2,3,0,10,20,2"]}}`)
		}
	})
	q, err := sdk.Board.Industry.Kline(context.Background(), "BK0475", KlineOptions{Period: "monthly", StartDate: "20240101", EndDate: "20240201"})
	if err != nil || len(q) != 1 || q[0].Adjust != AdjustNone {
		t.Fatalf("%v %v", q, err)
	}
	minute, err := sdk.Board.Concept.MinuteKline(context.Background(), "BK0475")
	if err != nil || len(minute) != 1 || minute[0].Adjust != AdjustQFQ {
		t.Fatalf("%v %v", minute, err)
	}
	trend, err := sdk.Board.Industry.MinuteKline(context.Background(), "BK0475", "1")
	if err != nil || len(trend) != 1 {
		t.Fatalf("%v %v", trend, err)
	}
	assertNum(t, trend[0].Price, 2)
	if trend[0].Amplitude != nil {
		t.Fatal("trend price misclassified as amplitude")
	}
}
func TestExtensionsValidationBeforeHTTP(t *testing.T) {
	sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
	ctx := context.Background()
	for _, run := range []func() error{
		func() error { _, e := sdk.Board.Industry.List(ctx, PageOptions{Page: -1}); return e }, func() error {
			_, e := sdk.Board.Industry.Constituents(ctx, "BK0475", PageOptions{PageSize: 101})
			return e
		}, func() error { _, e := sdk.Board.Industry.Kline(ctx, "BK0475", KlineOptions{Period: "bad"}); return e }, func() error { _, e := sdk.Board.Industry.MinuteKline(ctx, "BK0475", "2"); return e }, func() error { _, e := sdk.Board.Industry.Spot(ctx, ""); return e },
		func() error { _, e := sdk.FundFlow.Individual(ctx, "600519", FundFlowOptions{Period: "bad"}); return e }, func() error { _, e := sdk.FundFlow.Individual(ctx, "930955"); return e }, func() error { _, e := sdk.FundFlow.Individual(ctx, "HSHCI"); return e }, func() error { _, e := sdk.FundFlow.SectorHistory(ctx, "600519"); return e }, func() error { _, e := sdk.FundFlow.Rank(ctx, FundFlowRankOptions{Indicator: "bad"}); return e }, func() error {
			_, e := sdk.FundFlow.Rank(ctx, FundFlowRankOptions{PageOptions: PageOptions{PageSize: 101}})
			return e
		}, func() error { _, e := sdk.FundFlow.SectorRank(ctx, FundFlowRankOptions{SectorType: "bad"}); return e },
		func() error { _, e := sdk.Chips.CN(ctx, "600519", ChipRequestOptions{Days: -1}); return e }, func() error {
			_, e := sdk.Chips.CN(ctx, "600519", ChipRequestOptions{Range: indicators.Int(-1)})
			return e
		}, func() error { _, e := sdk.Chips.CN(ctx, "600519", ChipRequestOptions{Adjust: "bad"}); return e }, func() error {
			_, e := sdk.Chips.CN(ctx, "600519", ChipRequestOptions{IncludeHistogram: "bad"})
			return e
		},
	} {
		if e := run(); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
			t.Fatal(e)
		}
	}
}
func TestRealtimeFlowQueryAndCancellation(t *testing.T) {
	requests := []string{}
	var mu sync.Mutex
	sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		requests = append(requests, q.Get("secid"))
		mu.Unlock()
		if !strings.Contains(q.Get("fields"), "f135") || q.Get("ut") != emToken {
			t.Error(q)
		}
		if q.Get("secid") == "0.000001" {
			fmt.Fprint(w, `{"data":null}`)
			return
		}
		fmt.Fprint(w, `{"data":{"f57":"600519","f58":"贵州茅台","f48":1000000,"f135":400000,"f136":300000,"f137":100000,"f147":200000,"f148":250000,"f149":-50000,"f86":1704159000}}`)
	})
	q, err := sdk.Quotes.FundFlow(context.Background(), []string{"600519", "sh600519", "sz000001", "930955", "AAPL"})
	sort.Strings(requests)
	if err != nil || len(q) != 1 || !reflect.DeepEqual(requests, []string{"0.000001", "1.600519"}) {
		t.Fatalf("%v %v %v", q, err, requests)
	}
	if q[0].MainNet != 10 || q[0].MainNetRatio != 10 || q[0].RetailNet != -5 || q[0].Date != "20240102" || q[0].Timestamp == nil {
		t.Fatalf("%+v", q[0])
	}
	if q, err := sdk.FundFlow.Realtime(context.Background(), nil); err != nil || q == nil || len(q) != 0 {
		t.Fatalf("%v %v", q, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sdk.FundFlow.Rank(ctx); sdkerrors.CodeOf(err) != sdkerrors.Aborted {
		t.Fatal(err)
	}
}
func TestFlowHistoryParametersAndRankPaging(t *testing.T) {
	sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch r.URL.Path {
		case "/flow":
			if q.Get("ut") != emDataToken || q.Get("fields2") != flowFields {
				t.Error(q)
			}
			if q.Get("secid") == "1.000001" {
				if q.Get("secid2") != "0.399001" {
					t.Error(q)
				}
			} else if q.Get("secid") == "90.BK0475" {
				if q.Get("klt") != "103" {
					t.Error(q)
				}
			} else if q.Get("secid") != "1.600519" || q.Get("klt") != "102" {
				t.Error(q)
			}
			fmt.Fprint(w, `{"data":{"klines":["2024-01-02,1,2,3,4,5,6,7,8,9,10,11,12,13,14",null]}}`)
		case "/rank":
			if q.Get("pn") != "2" || q.Get("pz") != "25" || q.Get("fid") != "f164" || q.Get("fs") != "m:90+t:1" || !strings.Contains(q.Get("fields"), "f204") {
				t.Error(q)
			}
			fmt.Fprint(w, `{"data":{"total":200,"diff":[{"f12":"BK0475","f164":123,"f109":5}]}}`)
		}
	})
	if q, err := sdk.FundFlow.Individual(context.Background(), "600519", FundFlowOptions{Period: "weekly"}); err != nil || len(q) != 1 {
		t.Fatalf("%v %v", q, err)
	}
	if q, err := sdk.FundFlow.Market(context.Background()); err != nil || len(q) != 1 {
		t.Fatalf("%v %v", q, err)
	} else {
		assertNum(t, q[0].SzClose, 13)
	}
	if q, err := sdk.FundFlow.SectorHistory(context.Background(), "90.BK0475", FundFlowOptions{Period: "monthly"}); err != nil || len(q) != 1 {
		t.Fatalf("%v %v", q, err)
	}
	q, err := sdk.FundFlow.SectorRank(context.Background(), FundFlowRankOptions{Indicator: "5day", SectorType: "region", PageOptions: PageOptions{Page: 2, PageSize: 25}})
	if err != nil || len(q) != 1 {
		t.Fatalf("%v %v", q, err)
	}
	assertNum(t, q[0].MainNetInflow, 123)
}
func TestChipsUsesOnlyEastmoneyAndTail(t *testing.T) {
	for _, market := range []string{"CN", "HK", "US"} {
		t.Run(market, func(t *testing.T) {
			calls := 0
			sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/tx" {
					t.Error("chips must not use fallback")
				}
				if r.URL.Query().Get("klt") != "101" {
					t.Error(r.URL.RawQuery)
				}
				fmt.Fprint(w, `{"data":{"klines":["2024-01-01,10,11,12,9,100,200,3,4,5,2","2024-01-02,11,12,13,10,100,200,3,4,5,3","2024-01-03,12,13,14,11,100,200,3,4,5,4"]}}`)
			})
			o := ChipRequestOptions{Days: 2, Range: indicators.Int(0), IncludeHistogram: indicators.HistogramLast}
			var got []indicators.ChipDistributionItem
			var err error
			switch market {
			case "CN":
				got, err = sdk.Chips.CN(context.Background(), "600519", o)
			case "HK":
				got, err = sdk.Chips.HK(context.Background(), "700", o)
			case "US":
				got, err = sdk.Chips.US(context.Background(), "105.AAPL", o)
			}
			if err != nil || len(got) != 2 || got[0].Date != "2024-01-02" || got[0].Histogram != nil || got[1].Histogram == nil || calls != 1 {
				t.Fatalf("%v %v calls=%d", got, err, calls)
			}
		})
	}
	t.Run("failure", func(t *testing.T) {
		calls := 0
		sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) })
		_, err := sdk.Chips.CN(context.Background(), "600519", ChipRequestOptions{Range: indicators.Int(0)})
		if sdkerrors.CodeOf(err) != sdkerrors.HTTP || calls != 1 {
			t.Fatal(err, calls)
		}
	})
}
func TestChipsWarmupRefetch(t *testing.T) {
	calls := 0
	begins := []string{}
	today := time.Now().UTC().Format("2006-01-02")
	sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		begins = append(begins, r.URL.Query().Get("beg"))
		rows := []string{}
		if calls > 1 {
			rows = append(rows, today+",10,11,12,9,100,200,3,4,5,2")
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"klines": rows}})
	})
	q, err := sdk.Chips.CN(context.Background(), "600519", ChipRequestOptions{Days: 2, Range: indicators.Int(5)})
	if err != nil || len(q) != 1 || calls != 2 || begins[1] != "19700101" {
		t.Fatalf("%v %v calls=%d begins=%v", q, err, calls, begins)
	}
}

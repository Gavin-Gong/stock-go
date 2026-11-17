package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestDerivativesPagingAndBoundaries(t *testing.T) {
	for _, mode := range []string{"empty", "missing", "short", "limit", "bad"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("pageIndex") != numberString(calls) {
					t.Error(q)
				}
				calls++
				switch mode {
				case "bad":
					fmt.Fprint(w, `invalid`)
				case "missing":
					fmt.Fprint(w, `{}`)
				case "empty":
					fmt.Fprint(w, `{"total":99,"list":[]}`)
				case "short":
					if calls == 3 {
						fmt.Fprint(w, `{"total":99,"list":[]}`)
					} else {
						fmt.Fprint(w, `{"total":99,"list":[{"dm":"GC00Y"}]}`)
					}
				case "limit":
					fmt.Fprint(w, `{"total":99,"list":[{}]}`)
				}
			})
			rows, err := sdk.Futures.GlobalSpot(context.Background(), GlobalFuturesSpotOptions{PageSize: 10, MaxPages: 3})
			if mode == "bad" || mode == "limit" {
				if err == nil || rows != nil {
					t.Fatal(rows, err)
				}
				return
			}
			want := 0
			if mode == "short" {
				want = 2
			}
			if err != nil || rows == nil || len(rows) != want {
				t.Fatal(rows, err)
			}
		})
	}
	sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("reportName") == "RPT_FUTU_POSITIONCODE" {
			if r.URL.Query().Get("pageNumber") != "1" {
				t.Fatal("symbols unexpectedly paginated")
			}
			fmt.Fprint(w, `{"result":{"pages":5,"data":[{"TRADE_CODE":"CU"}]}}`)
			return
		}
		fmt.Fprint(w, `{"data":null}`)
	})
	symbols, e := sdk.Futures.InventorySymbols(context.Background())
	if e != nil || len(symbols) != 1 {
		t.Fatal(symbols, e)
	}
	for _, global := range []bool{false, true} {
		var rows []FuturesKline
		var err error
		if global {
			rows, err = sdk.Futures.GlobalKline(context.Background(), "GC00Y")
		} else {
			rows, err = sdk.Futures.Kline(context.Background(), "RBM")
		}
		if err != nil || rows == nil || len(rows) != 0 {
			t.Fatal(rows, err)
		}
	}
}
func TestDerivativeArgumentValidation(t *testing.T) {
	var calls atomic.Int32
	sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `{}`) })
	ctx := context.Background()
	negative := -1
	checks := []func() error{
		func() error {
			_, e := sdk.Futures.Kline(ctx, "rb2605", FuturesKlineOptions{}, FuturesKlineOptions{})
			return e
		},
		func() error {
			_, e := sdk.Futures.Kline(ctx, "rb2605", FuturesKlineOptions{Period: "minute"})
			return e
		},
		func() error {
			_, e := sdk.Futures.Kline(ctx, "rb2605", FuturesKlineOptions{StartDate: "20240230"})
			return e
		},
		func() error {
			_, e := sdk.Futures.Kline(ctx, "rb2605", FuturesKlineOptions{StartDate: "20240102", EndDate: "20240101"})
			return e
		},
		func() error { _, e := sdk.Futures.Kline(ctx, "rb2605&x=y"); return e },
		func() error { _, e := sdk.Futures.Kline(ctx, "unknown2605"); return e },
		func() error {
			_, e := sdk.Futures.GlobalKline(ctx, "GC00Y", GlobalFuturesKlineOptions{}, GlobalFuturesKlineOptions{})
			return e
		},
		func() error {
			_, e := sdk.Futures.GlobalKline(ctx, "GC00Y", GlobalFuturesKlineOptions{MarketCode: &negative})
			return e
		},
		func() error {
			_, e := sdk.Futures.GlobalKline(ctx, "GC00Y", GlobalFuturesKlineOptions{EndDate: "bad"})
			return e
		},
		func() error { _, e := sdk.Futures.GlobalKline(ctx, "GC00Y&x=1"); return e },
		func() error { _, e := sdk.Futures.GlobalKline(ctx, "ZZ00Y"); return e },
		func() error {
			_, e := sdk.Futures.GlobalSpot(ctx, GlobalFuturesSpotOptions{}, GlobalFuturesSpotOptions{})
			return e
		},
		func() error { _, e := sdk.Futures.GlobalSpot(ctx, GlobalFuturesSpotOptions{PageSize: -1}); return e },
		func() error { _, e := sdk.Futures.GlobalSpot(ctx, GlobalFuturesSpotOptions{MaxPages: -1}); return e },
		func() error {
			_, e := sdk.Futures.Inventory(ctx, "cu", FuturesInventoryOptions{}, FuturesInventoryOptions{})
			return e
		},
		func() error {
			_, e := sdk.Futures.Inventory(ctx, "cu", FuturesInventoryOptions{StartDate: "bad"})
			return e
		},
		func() error { _, e := sdk.Futures.Inventory(ctx, `cu")`); return e },
		func() error {
			_, e := sdk.Futures.Inventory(ctx, "cu", FuturesInventoryOptions{PageSize: 5001})
			return e
		},
		func() error { _, e := sdk.Futures.ComexInventory(ctx, "invalid"); return e },
		func() error {
			_, e := sdk.Futures.ComexInventory(ctx, "gold", ComexInventoryOptions{}, ComexInventoryOptions{})
			return e
		},
		func() error {
			_, e := sdk.Futures.ComexInventory(ctx, "gold", ComexInventoryOptions{PageSize: -1})
			return e
		},
		func() error {
			_, e := sdk.Options.CFFEXQuotes(ctx, CFFEXOptionQuotesOptions{}, CFFEXOptionQuotesOptions{})
			return e
		},
		func() error { _, e := sdk.Options.CFFEXQuotes(ctx, CFFEXOptionQuotesOptions{PageSize: -1}); return e },
		func() error { _, e := sdk.Options.IndexSpot(ctx, "bad", "io2612"); return e },
		func() error { _, e := sdk.Options.IndexSpot(ctx, OptionIO, "io2612&x=1"); return e },
		func() error { _, e := sdk.Options.CommoditySpot(ctx, "XX", "au2612"); return e },
		func() error { _, e := sdk.Options.CommodityKline(ctx, "bad&value"); return e },
		func() error { _, e := sdk.Options.ETFMonths(ctx, "bad"); return e },
		func() error { _, e := sdk.Options.ETFExpireDay(ctx, "bad", "2026-12"); return e },
		func() error { _, e := sdk.Options.ETFExpireDay(ctx, ETF50, "202612"); return e },
		func() error { _, e := sdk.Options.ETFExpireDay(ctx, ETF50, "2026-13"); return e },
		func() error { _, e := sdk.Options.ETFMinute(ctx, "notcode"); return e },
		func() error { _, e := sdk.Options.ETFDailyKline(ctx, "123"); return e },
		func() error { _, e := sdk.Options.ETF5DayMinute(ctx, "10009633&x=1"); return e },
		func() error { _, e := sdk.Options.LHB(ctx, "00700.HK", "20240102"); return e },
		func() error { _, e := sdk.Options.LHB(ctx, "510050", "bad"); return e },
	}
	for i, check := range checks {
		if err := check(); err == nil {
			t.Errorf("validation %d accepted invalid options", i)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid args sent requests", calls.Load())
	}
	for _, v := range []string{"", "ZZ", "123", "rbm"} {
		if _, e := FuturesMarketCode(v); e == nil {
			t.Fatal(v)
		}
	}
	for _, cfg := range []Options{{FuturesURLs: FuturesEndpoints{Kline: "relative"}}, {OptionURLs: OptionEndpoints{ETFMinute: "ftp://host/path"}}} {
		if _, e := New(cfg); e == nil {
			t.Fatal(cfg)
		}
	}
}
func TestSinaPathCallbacksAndTypedExpiry(t *testing.T) {
	for _, mode := range []string{"numeric", "string", "empty", "errorOnXD"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				if q.Get("callback") != "__stock_go_jsonp" {
					t.Error(q)
				}
				if mode == "empty" {
					fmt.Fprint(w, `__stock_go_jsonp({});`)
					return
				}
				if calls == 1 {
					v := `-2`
					if mode == "string" {
						v = `"-2"`
					}
					fmt.Fprintf(w, `__stock_go_jsonp({"result":{"data":{"remainderDays":%s}}});`, v)
					return
				}
				if q.Get("cate") != "XD50ETF" {
					t.Error(q)
				}
				if mode == "errorOnXD" {
					fmt.Fprint(w, `bad`)
					return
				}
				fmt.Fprint(w, `__stock_go_jsonp({"result":{"data":{"expireDay":"2026-12-23","remainderDays":3}}});`)
			})
			day, e := sdk.Options.ETFExpireDay(context.Background(), ETF50, "2026-12")
			if mode == "errorOnXD" {
				if e == nil || day != nil {
					t.Fatal(day, e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			wantCalls := 1
			days := 0.0
			if mode == "numeric" {
				wantCalls = 2
				days = 3
			} else if mode == "string" {
				days = -2
			}
			if calls != wantCalls || day.RemainderDays != days {
				t.Fatal(calls, day)
			}
		})
	}
	sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/__stock_go_jsonp/") || r.URL.Query().Has("callback") {
			t.Error(r.URL)
		}
		fmt.Fprint(w, `__stock_go_jsonp(null);`)
	})
	rows, e := sdk.Options.ETFDailyKline(context.Background(), "10009633")
	if e != nil || rows == nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
}
func TestDerivativeEmptyAndErrors(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[]`, `invalid`, `cb({"result":{"status":{"code":5,"msg":"failed"}}});`} {
		t.Run(body, func(t *testing.T) {
			sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
			ctx := context.Background()
			checks := []func() error{
				func() error { _, e := sdk.Options.IndexSpot(ctx, OptionIO, "io2612"); return e },
				func() error { _, e := sdk.Options.CommodityKline(ctx, "au2612C1000"); return e },
				func() error { _, e := sdk.Options.ETFMonths(ctx, ETF50); return e },
				func() error { _, e := sdk.Options.ETFExpireDay(ctx, ETF50, "2026-12"); return e },
				func() error { _, e := sdk.Options.ETFMinute(ctx, "10009633"); return e },
				func() error { _, e := sdk.Options.ETF5DayMinute(ctx, "10009633"); return e },
			}
			for _, check := range checks {
				e := check()
				if (body == "invalid" || strings.HasPrefix(body, "cb(")) != (e != nil) {
					t.Fatal(body, e)
				}
			}
		})
	}
	for _, body := range []string{`invalid`, `{"rc":1}`, `{"success":false,"message":"failed"}`} {
		sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		ctx := context.Background()
		if body != `{"success":false,"message":"failed"}` {
			if _, e := sdk.Futures.Kline(ctx, "RBM"); e == nil {
				t.Fatal(body)
			}
		}
		if body == "invalid" || strings.Contains(body, "success") {
			if _, e := sdk.Options.LHB(ctx, "510050", "20240102"); e == nil {
				t.Fatal(body)
			}
			if _, e := sdk.Options.CFFEXQuotes(ctx); body == "invalid" && e == nil {
				t.Fatal(body)
			}
		}
	}
	var calls atomic.Int32
	sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `{}`) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := sdk.Options.ETFMinute(ctx, "10009633"); sdkerrors.CodeOf(e) != sdkerrors.Aborted {
		t.Fatal(e)
	}
	if _, e := sdk.Futures.Inventory(ctx, "cu"); sdkerrors.CodeOf(e) != sdkerrors.Aborted {
		t.Fatal(e)
	}
	if calls.Load() != 0 {
		t.Fatal(calls.Load())
	}
}
func TestETFMinuteDateResetsPerDay(t *testing.T) {
	sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `cb({"result":{"data":[[{"i":"09:30","d":"2024-01-02"},{"i":"09:31"}],[{"i":"09:30"},{"i":"09:31","d":"2024-01-03"}],null]}});`)
	})
	rows, e := sdk.Options.ETF5DayMinute(context.Background(), "10009633")
	if e != nil {
		t.Fatal(e)
	}
	dates := []string{}
	for _, r := range rows {
		dates = append(dates, r.Date)
	}
	if !reflect.DeepEqual(dates, []string{"2024-01-02", "2024-01-02", "", "2024-01-03"}) {
		t.Fatal(dates)
	}
	if raw, _ := json.Marshal(rows); len(raw) == 0 {
		t.Fatal("invalid JSON output")
	}
}

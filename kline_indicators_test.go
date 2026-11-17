package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/indicators"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"github.com/Gavin-Gong/stock-go/symbols"
)

func TestWithIndicatorsWarmupCalendarAndWindow(t *testing.T) {
	var begins []string
	start, _ := time.Parse("2006-01-02", "2024-01-01")
	calendar := []string{}
	for i := 0; i < 30; i++ {
		calendar = append(calendar, start.AddDate(0, 0, i).Format("2006-01-02"))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/calendar" {
			fmt.Fprint(w, strings.Join(calendar, ","))
			return
		}
		beg, end := r.URL.Query().Get("beg"), r.URL.Query().Get("end")
		begins = append(begins, beg)
		rows := []string{}
		for i, date := range calendar {
			if compactDate(date) >= beg && compactDate(date) <= end {
				v := float64(i + 1)
				rows = append(rows, fmt.Sprintf("%s,%g,%g,%g,%g,100", date, v, v, v+1, v-1))
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"klines": rows}})
	}))
	defer srv.Close()
	sdk, _ := New(Options{TradingCalendarURL: srv.URL + "/calendar", KlineURLs: KlineEndpoints{CN: srv.URL + "/kline"}, DisableKlineFallback: true})
	o := KlineIndicatorOptions{KlineOptions: KlineOptions{StartDate: "20240110", EndDate: "20240111"}, Indicators: indicators.Options{MA: &indicators.MAOptions{Periods: []int{3}}}}
	bars, err := sdk.Kline.WithIndicators(context.Background(), "600519", o)
	if err != nil || len(bars) != 2 {
		t.Fatalf("%v %v", bars, err)
	}
	if begins[0] != "20240106" || bars[0].Date != "2024-01-10" {
		t.Fatal(begins, bars[0].Date)
	}
	assertNum(t, bars[0].MA["ma3"], 9)
	o.LeadingBars = 1
	bars, err = sdk.Kline.WithIndicators(context.Background(), "600519", o)
	if err != nil || len(bars) != 3 || bars[0].Date != "2024-01-09" {
		t.Fatalf("%v %v", bars, err)
	}
}
func TestWithIndicatorsFullRefetchAndCumulativeOrigin(t *testing.T) {
	for _, cumulative := range []bool{false, true} {
		t.Run(fmt.Sprint(cumulative), func(t *testing.T) {
			var begins []string
			origin, _ := time.Parse("2006-01-02", "2020-01-01")
			target := origin.AddDate(0, 0, 900).Format("2006-01-02")
			sdk, srv := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				beg := r.URL.Query().Get("beg")
				begins = append(begins, beg)
				rows := []string{}
				if len(begins) > 1 {
					for i := 0; i <= 900; i++ {
						date := origin.AddDate(0, 0, i).Format("2006-01-02")
						rows = append(rows, fmt.Sprintf("%s,%d,%d,%d,%d,100", date, i+10, i+10, i+11, i+9))
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"klines": rows}})
			}, true, 0)
			// Use the foreign route to avoid a calendar lookup; data is still deterministic.
			_ = srv
			o := KlineIndicatorOptions{KlineOptions: KlineOptions{StartDate: target, EndDate: target}, Market: symbols.HK, Indicators: indicators.Options{MA: &indicators.MAOptions{Periods: []int{3}}}}
			if cumulative {
				o.Indicators.OBV = &indicators.OBVOptions{}
			}
			rows, err := sdk.Kline.WithIndicators(context.Background(), "700", o)
			if err != nil || len(rows) != 1 || len(begins) != 2 || begins[1] != "19700101" {
				t.Fatalf("len=%d begins=%v err=%v", len(rows), begins, err)
			}
			assertNum(t, rows[0].MA["ma3"], 909)
			if cumulative {
				assertNum(t, rows[0].OBV.OBV, 90100)
			}
		})
	}
}
func TestWithIndicatorsSignalsLeadingBar(t *testing.T) {
	origin, _ := time.Parse("2006-01-02", "2024-01-01")
	prices := []float64{10, 10, 10, 10, 5, 5, 5, 20}
	calendar := []string{}
	for i := range prices {
		calendar = append(calendar, origin.AddDate(0, 0, i).Format("2006-01-02"))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/calendar" {
			fmt.Fprint(w, strings.Join(calendar, ","))
			return
		}
		rows := []string{}
		for i, p := range prices {
			rows = append(rows, fmt.Sprintf("%s,%g,%g,%g,%g,100", calendar[i], p, p, p+1, p-1))
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"klines": rows}})
	}))
	defer srv.Close()
	sdk, _ := New(Options{TradingCalendarURL: srv.URL + "/calendar", KlineURLs: KlineEndpoints{CN: srv.URL}, DisableKlineFallback: true})
	events, err := sdk.Kline.Signals(context.Background(), "600519", KlineSignalOptions{KlineOptions: KlineOptions{StartDate: "20240108", EndDate: "20240108"}, MAFast: 2, MASlow: 3})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.Date != "2024-01-08" {
			t.Fatal("leading bar event leaked", e)
		}
		found = found || e.Type == "ma_golden_cross"
	}
	if !found {
		t.Fatalf("missing boundary crossover: %v", events)
	}
}
func TestIndicatorServiceValidation(t *testing.T) {
	sdk, _ := New()
	ctx := context.Background()
	for _, o := range []KlineIndicatorOptions{{LeadingBars: -1}, {Market: symbols.Global}, {KlineOptions: KlineOptions{StartDate: "bad"}}, {Indicators: indicators.Options{ATR: &indicators.ATROptions{Period: -1}}}} {
		if _, err := sdk.Kline.WithIndicators(ctx, "600519", o); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatalf("%+v %v", o, err)
		}
	}
	if _, err := sdk.Kline.WithIndicators(ctx, "GDAXI"); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := sdk.Kline.WithIndicators(ctx, "600519", KlineIndicatorOptions{}, KlineIndicatorOptions{}); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := sdk.Kline.Signals(ctx, "600519", KlineSignalOptions{MAFast: 20, MASlow: 5}); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
}

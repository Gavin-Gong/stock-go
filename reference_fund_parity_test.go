package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/symbols"
)

func referenceFundTestSDK(t *testing.T, h http.HandlerFunc) (*SDK, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	sdk, err := New(Options{DatacenterURL: srv.URL + "/dc", NorthboundMinuteURL: srv.URL + "/minute", TradingCalendarURL: srv.URL + "/calendar", FundURLs: FundEndpoints{PingzhongBase: srv.URL + "/ping", Dividend: srv.URL + "/dividend", ThemeBase: srv.URL + "/theme"}})
	if err != nil {
		t.Fatal(err)
	}
	return sdk, srv
}
func TestReferenceFundUpstreamParity(t *testing.T) {
	body, err := os.ReadFile("testdata/reference-fund-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Kind, Operation, Date, Market, At, Text, Variant string
		Calendar, Texts                                  []string
		Response, Expected                               json.RawMessage
	}
	if err := json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	for i, f := range fixtures {
		t.Run(fmt.Sprintf("%s/%s/%s/%d", f.Kind, f.Operation, f.Variant, i), func(t *testing.T) {
			sdk, _ := referenceFundTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				switch f.Kind {
				case "calendar":
					fmt.Fprint(w, strings.Join(f.Calendar, ","))
				case "fund":
					fmt.Fprint(w, f.Text)
				case "fundDividend":
					page, _ := strconv.Atoi(r.URL.Query().Get("page"))
					if page < 1 || page > len(f.Texts) {
						t.Error("unexpected dividend page")
						return
					}
					fmt.Fprint(w, f.Texts[page-1])
				default:
					w.Write(f.Response)
				}
			})
			var actual any
			var err error
			ctx := context.Background()
			switch f.Kind {
			case "calendar":
				switch f.Operation {
				case "isTradingDay":
					actual, err = sdk.Calendar.IsTradingDay(ctx, f.Date)
				case "nextTradingDay":
					actual, err = sdk.Calendar.NextTradingDay(ctx, f.Date)
				case "prevTradingDay":
					actual, err = sdk.Calendar.PrevTradingDay(ctx, f.Date)
				case "marketStatus":
					at, _ := time.Parse(time.RFC3339, f.At)
					market := symbols.Market(f.Market)
					if market == "A" {
						market = symbols.CN
					}
					actual, err = sdk.Calendar.MarketStatus(market, at)
				}
			case "dc":
				switch f.Operation {
				case "summary":
					actual, err = sdk.Northbound.Summary(ctx)
				case "holdingRank":
					actual, err = sdk.Northbound.HoldingRank(ctx, NorthboundHoldingRankOptions{Market: "shanghai", Period: "month", Date: "2024-01-02"})
				case "historyNorth", "historySouth":
					d := North
					if f.Operation == "historySouth" {
						d = South
					}
					actual, err = sdk.Northbound.History(ctx, NorthboundHistoryOptions{Direction: d, StartDate: "20240101", EndDate: "20240131"})
				case "individual":
					actual, err = sdk.Northbound.Individual(ctx, "600519.SH", NorthboundHistoryOptions{StartDate: "20240101", EndDate: "20240131"})
				case "dividend":
					actual, err = sdk.Reference.DividendDetail(ctx, "600519.SH")
				}
			case "northMinute":
				actual, err = sdk.Northbound.Minute(ctx, NorthboundDirection(f.Operation))
			case "fund":
				switch f.Operation {
				case "nav":
					actual, err = sdk.Fund.NavHistory(ctx, "110011")
				case "rank":
					actual, err = sdk.Fund.RankHistory(ctx, "110011")
				case "profile":
					actual, err = sdk.Fund.Profile(ctx, "110011")
				}
			case "fundDividend":
				actual, err = sdk.Fund.DividendList(ctx, FundDividendOptions{Year: 2024, Code: "110011", AllPages: f.Operation == "all"})
			case "theme":
				if f.Operation == "list" {
					actual, err = sdk.Fund.Theme.GetThemeList(ctx)
				} else {
					actual, err = sdk.Fund.Theme.GetThemeFunds(ctx, "T1")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			json.Unmarshal(encoded, &got)
			json.Unmarshal(f.Expected, &want)
			compareSourceFields(t, got, want)
		})
	}
}

package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

func derivativeTestSDK(t *testing.T, handler http.HandlerFunc) *SDK {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	sdk, err := New(Options{DatacenterURL: srv.URL + "/dc", FuturesURLs: FuturesEndpoints{Kline: srv.URL + "/kline", GlobalSpot: srv.URL + "/global"}, OptionURLs: OptionEndpoints{Spot: srv.URL + "/spot", Dayline: srv.URL + "/{callback}/dayline", ETFMonths: srv.URL + "/months", ETFExpire: srv.URL + "/expire", ETFMinute: srv.URL + "/minute", ETFDaily: srv.URL + "/{callback}/daily", ETF5Day: srv.URL + "/five", CFFEX: srv.URL + "/cffex", LHB: srv.URL + "/lhb"}})
	if err != nil {
		t.Fatal(err)
	}
	return sdk
}
func TestDerivativeUpstreamParity(t *testing.T) {
	body, err := os.ReadFile("testdata/derivative-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Operation, Symbol, Variant string
		Responses                  []json.RawMessage
		Queries                    []map[string]string
		Expected                   json.RawMessage
	}
	if err = json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Operation+"/"+f.Symbol+"/"+f.Variant, func(t *testing.T) {
			queries := []map[string]string{}
			sdk := derivativeTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				q.Del("callback")
				query := map[string]string{}
				for k, v := range q {
					query[k] = v[0]
				}
				i := len(queries)
				queries = append(queries, query)
				raw := f.Responses[min(i, len(f.Responses)-1)]
				switch r.URL.Path {
				case "/dc", "/kline", "/global", "/cffex", "/lhb":
					w.Write(raw)
				default:
					fmt.Fprintf(w, "/* guarded */ __stock_go_jsonp(%s);", raw)
				}
			})
			ctx := context.Background()
			var actual any
			var err error
			switch f.Operation {
			case "domestic":
				actual, err = sdk.Futures.Kline(ctx, f.Symbol, FuturesKlineOptions{Period: f.Variant, StartDate: "20240101", EndDate: "20240131"})
			case "global":
				o := GlobalFuturesKlineOptions{StartDate: "20240101", EndDate: "20240131"}
				if f.Variant == "override" {
					code := 999
					o.MarketCode = &code
				}
				actual, err = sdk.Futures.GlobalKline(ctx, f.Symbol, o)
			case "spot":
				actual, err = sdk.Futures.GlobalSpot(ctx, GlobalFuturesSpotOptions{PageSize: 2})
			case "inventorySymbols":
				actual, err = sdk.Futures.InventorySymbols(ctx)
			case "inventory":
				actual, err = sdk.Futures.Inventory(ctx, f.Symbol, FuturesInventoryOptions{StartDate: "20240101", PageSize: 10})
			case "comex":
				actual, err = sdk.Futures.ComexInventory(ctx, f.Symbol, ComexInventoryOptions{PageSize: 10})
			case "cffex":
				actual, err = sdk.Options.CFFEXQuotes(ctx)
			case "lhb":
				actual, err = sdk.Options.LHB(ctx, f.Symbol, "20240102")
			case "indexSpot":
				actual, err = sdk.Options.IndexSpot(ctx, IndexOptionProduct(f.Symbol), f.Variant)
			case "commoditySpot":
				actual, err = sdk.Options.CommoditySpot(ctx, f.Symbol, f.Variant)
			case "indexKline":
				actual, err = sdk.Options.IndexKline(ctx, f.Symbol)
			case "commodityKline":
				actual, err = sdk.Options.CommodityKline(ctx, f.Symbol)
			case "months":
				actual, err = sdk.Options.ETFMonths(ctx, ETFOptionCate(f.Symbol))
			case "expire":
				actual, err = sdk.Options.ETFExpireDay(ctx, ETFOptionCate(f.Symbol), "2026-12")
			case "minute":
				actual, err = sdk.Options.ETFMinute(ctx, f.Symbol)
			case "etfDaily":
				actual, err = sdk.Options.ETFDailyKline(ctx, f.Symbol)
			case "fiveDay":
				actual, err = sdk.Options.ETF5DayMinute(ctx, f.Symbol)
			default:
				t.Fatal(f.Operation)
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
			if !reflect.DeepEqual(queries, f.Queries) {
				t.Fatalf("query parity\ngot %v\nwant %v", queries, f.Queries)
			}
		})
	}
}

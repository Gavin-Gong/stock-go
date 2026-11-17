package stockgo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

func topicReportTestSDK(t *testing.T, handler http.HandlerFunc) *SDK {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	sdk, err := New(Options{TopicBaseURL: srv.URL, DatacenterURL: srv.URL + "/dc", TradingCalendarURL: srv.URL + "/calendar"})
	if err != nil {
		t.Fatal(err)
	}
	return sdk
}
func TestTopicReportUpstreamParity(t *testing.T) {
	body, err := os.ReadFile("testdata/topic-report-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Operation, Variant string
		Response, Expected json.RawMessage
		Queries            []map[string]string
	}
	if err = json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Operation+"/"+f.Variant, func(t *testing.T) {
			var mu sync.Mutex
			queries := []map[string]string{}
			sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				q := map[string]string{}
				for key, values := range r.URL.Query() {
					q[key] = values[0]
				}
				mu.Lock()
				queries = append(queries, q)
				mu.Unlock()
				w.Write(f.Response)
			})
			ctx := context.Background()
			var actual any
			var err error
			date := DragonTigerDateOptions{StartDate: "20240101", EndDate: "20240131"}
			blockDate := BlockTradeDateOptions{StartDate: "20240101", EndDate: "20240131"}
			switch f.Operation {
			case "dragonDetail":
				actual, err = sdk.DragonTiger.Detail(ctx, date)
			case "dragonInstitution":
				actual, err = sdk.DragonTiger.Institution(ctx, date)
			case "dragonStats":
				actual, err = sdk.DragonTiger.StockStats(ctx, DragonTigerPeriod(f.Variant))
			case "dragonBranch":
				actual, err = sdk.DragonTiger.BranchRank(ctx, DragonTigerPeriod(f.Variant))
			case "seat":
				actual, err = sdk.DragonTiger.SeatDetail(ctx, "600519.SH", "20240102")
			case "blockMarket":
				actual, err = sdk.BlockTrade.MarketStat(ctx)
			case "blockDetail":
				actual, err = sdk.BlockTrade.Detail(ctx, blockDate)
			case "blockDaily":
				actual, err = sdk.BlockTrade.DailyStat(ctx, blockDate)
			case "marginAccount":
				actual, err = sdk.Margin.AccountInfo(ctx)
			case "marginTarget":
				actual, err = sdk.Margin.TargetList(ctx, "20240102")
			case "pool":
				actual, err = sdk.MarketEvent.ZTPool(ctx, ZTPoolOptions{Type: ZTPoolType(f.Variant), Date: "20240102"})
			case "changes":
				types := []StockChangeType{LargeBuy}
				if f.Variant == "multi" {
					types = append(types, LargeSell)
				} else if f.Variant == "all" {
					types = []StockChangeType{AllChanges}
				}
				actual, err = sdk.MarketEvent.StockChanges(ctx, StockChangesOptions{Types: types, Page: 2, PageSize: 10})
			case "individual":
				actual, err = sdk.MarketEvent.IndividualDay(ctx, "600519.SH", "20240102")
			case "board":
				actual, err = sdk.MarketEvent.BoardChanges(ctx)
			case "unusual":
				o := UnusualFluctuationOptions{}
				if f.Variant == "date" {
					o.Date = "20240102"
					o.Triggered = Bool(true)
				} else if f.Variant == "range" {
					o.StartDate = "20240101"
					o.EndDate = "20240131"
					o.Triggered = Bool(false)
				}
				actual, err = sdk.MarketEvent.UnusualFluctuation(ctx, o)
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
			canonical := func(qs []map[string]string) []string {
				out := []string{}
				for _, q := range qs {
					b, _ := json.Marshal(q)
					out = append(out, string(b))
				}
				sort.Strings(out)
				return out
			}
			if !reflect.DeepEqual(canonical(queries), canonical(f.Queries)) {
				t.Fatalf("query parity\ngot: %v\nwant:%v", queries, f.Queries)
			}
		})
	}
}
func TestTopicRawCommasAndDefaults(t *testing.T) {
	calls := 0
	sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/getAllStockChanges" {
			if strings.Contains(strings.ToUpper(r.URL.RawQuery), "%2C") || r.URL.Query().Get("type") != "8193,8194" {
				t.Error(r.URL)
			}
			if r.URL.Query().Get("pagesize") != "5000" {
				t.Error(r.URL)
			}
			w.Write([]byte(`{"data":{"tc":1,"allstock":[{"tm":93000,"t":8194}]}}`))
		} else {
			q := r.URL.Query()
			today, _ := calendarDate(nil)
			if q.Get("date") != compactDate(today) || q.Get("sort") != "fbt:asc" {
				t.Error(q)
			}
			w.Write([]byte(`{"data":null}`))
		}
	})
	rows, err := sdk.MarketEvent.StockChanges(context.Background(), StockChangesOptions{Types: []StockChangeType{LargeBuy, LargeSell}})
	if err != nil || len(rows) != 1 || rows[0].ChangeType != LargeSell {
		t.Fatal(rows, err)
	}
	pool, err := sdk.MarketEvent.ZTPool(context.Background())
	if err != nil || len(pool) != 0 || pool == nil || calls != 2 {
		t.Fatal(pool, calls, err)
	}
}

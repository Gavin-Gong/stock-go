package operations

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	stockgo "github.com/Gavin-Gong/stock-go"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestRegistryCoverageAndWhitelist(t *testing.T) {
	sdk, e := stockgo.New()
	if e != nil {
		t.Fatal(e)
	}
	r, e := New(sdk)
	if e != nil || len(r.handlers) != 94 {
		t.Fatal(r, e)
	}
	specs := Specs()
	specs[0].Path[0] = "mutated"
	if Specs()[0].Path[0] == "mutated" {
		t.Fatal("manifest mutation")
	}
	if _, e := New(nil); e == nil {
		t.Fatal("nil SDK")
	}
	for _, path := range []string{"client.get", "Requests.Get", "__proto__", "constructor", "unknown", "search.extra"} {
		if r.Has(path) {
			t.Fatal(path)
		}
		if _, e = r.Invoke(context.Background(), path, nil); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
			t.Fatal(path, e)
		}
	}
	// Every binding can be invoked with its source signature under a canceled context,
	// without reaching the network or panicking (marketStatus is a local calculation).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, spec := range Specs() {
		path := strings.Join(spec.Path, ".")
		t.Run(path, func(t *testing.T) {
			args := []any{}
			if spec.ArgShape == "codes[]" || spec.ArgShape == "codes+options" {
				code := "600519"
				if strings.Contains(path, ".hk") {
					code = "00700"
				}
				if strings.Contains(path, ".us") {
					code = "AAPL"
				}
				if path == "quotes.fund" {
					code = "110011"
				}
				args = append(args, []string{code})
			}
			for _, p := range spec.Positional {
				value := "600519"
				if len(p.Enum) > 0 {
					value = p.Enum[0]
				} else {
					switch p.Name {
					case "date":
						value = "20240102"
					case "month":
						value = "2026-12"
					case "keyword":
						value = "茅台"
					case "contract":
						value = "io2612"
					case "code":
						if strings.Contains(path, "options.etf") {
							value = "10011430"
						} else {
							value = "110011"
						}
					}
				}
				if p.Name == "symbol" {
					if strings.Contains(path, "board.") {
						value = "BK0475"
					} else if path == "futures.kline" {
						value = "RBM"
					} else if path == "futures.globalKline" {
						value = "GC00Y"
					} else if path == "futures.inventory" {
						value = "cu"
					} else if strings.HasPrefix(path, "options.") {
						value = "io2612C4000"
					} else if path == "chips.hk" || strings.Contains(path, "kline.hk") {
						value = "00700"
					} else if path == "chips.us" || strings.Contains(path, "kline.us") {
						value = "AAPL"
					} else if path == "fund.theme.getThemeFunds" {
						value = "BK0474"
					}
				}
				args = append(args, value)
			}
			opts := map[string]any{}
			for _, p := range spec.Params {
				if p.Required {
					field := p.Field
					if field == "" {
						field = p.Flag
					}
					value := "20240101"
					if p.Flag == "end" {
						value = "20240131"
					}
					opts[field] = value
				}
			}
			if spec.ArgShape == "options" || spec.ArgShape == "symbol+options" || spec.ArgShape == "codes+options" {
				args = append(args, opts)
			}
			_, e := r.Invoke(ctx, path, args)
			if e != nil && sdkerrors.CodeOf(e) == "" {
				t.Fatal("untyped error", e)
			}
		})
	}
}
func TestArgumentBridgeConversions(t *testing.T) {
	cases := []struct {
		path       string
		args, want []any
	}{
		{"kline.cn", []any{"600519", map[string]any{"adjust": ""}}, []any{"600519", map[string]any{"adjust": "none"}}},
		{"kline.withIndicators", []any{"600519", map[string]any{"market": "A", "indicators": map[string]any{"macd": true, "boll": false}}}, []any{"600519", map[string]any{"market": "CN", "indicators": map[string]any{"macd": map[string]any{}}}}},
		{"fund.dividendList", []any{map[string]any{"year": "2024", "page": "all"}}, []any{map[string]any{"year": 2024, "allPages": true}}},
		{"fund.dividendList", []any{map[string]any{"page": "2"}}, []any{map[string]any{"page": 2}}},
		{"chips.cn", []any{"600519", map[string]any{"includeHistogram": true}}, []any{"600519", map[string]any{"includeHistogram": "all"}}},
		{"chips.cn", []any{"600519", map[string]any{"includeHistogram": false}}, []any{"600519", map[string]any{}}},
		{"marketEvent.ztPool", nil, []any{map[string]any{"type": nil, "date": nil}}},
		{"marketEvent.stockChanges", []any{"all", map[string]any{"page": 1}}, []any{map[string]any{"types": []any{"all"}, "page": 1}}},
		{"marketEvent.stockChanges", []any{[]any{"large_buy", "large_sell"}}, []any{map[string]any{"types": []any{"large_buy", "large_sell"}}}},
		{"northbound.history", []any{"south", map[string]any{"startDate": "20240102"}}, []any{map[string]any{"direction": "south", "startDate": "20240102"}}},
		{"marketEvent.individualChanges", []any{"600519", map[string]any{"date": "20240102"}}, []any{"600519", "20240102"}},
		{"marketEvent.individualChangesHistory", []any{"600519", map[string]any{"days": 7}}, []any{"600519", 7}},
		{"board.industry.minuteKline", []any{"BK0475", map[string]any{"period": "5"}}, []any{"BK0475", "5"}},
		{"board.concept.minuteKline", []any{"BK0475", map[string]any{}}, []any{"BK0475"}},
	}
	for _, c := range cases {
		got, e := prepare(c.path, c.args)
		if e != nil || !reflect.DeepEqual(got, c.want) {
			t.Fatal(c.path, got, c.want, e)
		}
	}
	for _, test := range []struct {
		path string
		args []any
	}{{"fund.dividendList", []any{map[string]any{"year": "bad"}}}, {"kline.withIndicators", []any{"600519", map[string]any{"indicators": true}}}, {"marketEvent.ztPool", []any{1, 2, 3}}, {"northbound.history", []any{"north", 1}}, {"marketEvent.individualChanges", nil}, {"marketEvent.individualChanges", []any{"600519", 1}}} {
		if _, e := prepare(test.path, test.args); e == nil {
			t.Fatal(test)
		}
	}
	if _, e := decode[int](math.NaN()); e == nil {
		t.Fatal("NaN argument")
	}
	if _, e := decode[int]("bad"); e == nil {
		t.Fatal("invalid numeric argument")
	}
}
func TestTypedInvocationUsesOptionsAndHTTP(t *testing.T) {
	var query map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		switch r.URL.Path {
		case "/kline":
			w.Write([]byte(`{"data":{"code":"600519","klines":["2024-01-02,1,2,3,0,100,200,3,4,1,2"]}}`))
		case "/expire":
			w.Write([]byte(`cb({"result":{"data":{"expireDay":"2026-12-23","remainderDays":36}}});`))
		default:
			w.Write([]byte(`{"result":{"pages":1,"data":[{"SECURITY_CODE":"510050","TRADE_DATE":"2024-01-02"}]}}`))
		}
	}))
	defer srv.Close()
	sdk, e := stockgo.New(stockgo.Options{KlineURLs: stockgo.KlineEndpoints{CN: srv.URL + "/kline"}, OptionURLs: stockgo.OptionEndpoints{ETFExpire: srv.URL + "/expire", LHB: srv.URL + "/lhb"}})
	if e != nil {
		t.Fatal(e)
	}
	r, _ := New(sdk)
	out, e := r.Invoke(context.Background(), "kline.cn", []any{"600519", map[string]any{"adjust": "", "startDate": "20240101", "endDate": "20240131"}})
	if e != nil || len(out.([]stockgo.KlineBar)) != 1 || query["fqt"][0] != "0" {
		t.Fatal(out, query, e)
	}
	out, e = r.Invoke(context.Background(), "options.etf.expireDay", []any{"50ETF", "2026-12"})
	if e != nil || out.(*stockgo.ETFOptionExpireDay).RemainderDays != 36 {
		t.Fatal(out, e)
	}
	out, e = r.Invoke(context.Background(), "options.lhb", []any{"510050", "20240102"})
	if e != nil || len(out.([]stockgo.OptionLHBItem)) != 1 {
		t.Fatal(out, e)
	}
	out, e = r.Invoke(context.Background(), "calendar.marketStatus", []any{"US", "2024-07-09T13:30:00Z"})
	if e != nil || out != stockgo.MarketOpen {
		t.Fatal(out, e)
	}
	for _, test := range []struct {
		path string
		args []any
	}{{"search", nil}, {"search", []any{1}}, {"options.lhb", []any{"510050"}}, {"options.lhb", []any{1, "20240102"}}, {"options.lhb", []any{"510050", 1}}, {"kline.cn", nil}, {"kline.cn", []any{1}}, {"kline.cn", []any{"600519", 1}}, {"codes.cn", []any{1}}, {"codes.hk", []any{"extra"}}, {"calendar.marketStatus", []any{1}}, {"calendar.marketStatus", []any{"US", 1}}, {"calendar.marketStatus", []any{"US", "bad"}}} {
		if _, e := r.Invoke(context.Background(), test.path, test.args); e == nil {
			t.Fatal(test)
		}
	}
	b, _ := json.Marshal(out)
	if len(b) == 0 {
		t.Fatal("invalid result")
	}
}

package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestQuoteUpstreamParity(t *testing.T) {
	data, err := os.ReadFile("testdata/quote-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Kind, Label string
		Fields      []string
		Expected    json.RawMessage
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Label, func(t *testing.T) {
			var actual any
			switch fixture.Kind {
			case "FullQuote":
				actual = parseFullQuote(fixture.Fields)
			case "SimpleQuote":
				actual = parseSimpleQuote(fixture.Fields)
			case "HKQuote":
				actual = parseHKQuote(fixture.Fields)
			case "USQuote":
				actual = parseUSQuote(fixture.Fields)
			case "FundQuote":
				actual = parseFundQuote(fixture.Fields)
			case "PanelLargeOrder":
				actual = parsePanelLargeOrder(fixture.Fields)
			default:
				t.Fatal("unknown fixture kind")
			}
			encoded, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(fixture.Expected, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Go: %s\nTS: %s", encoded, fixture.Expected)
			}
		})
	}
}
func fields(code string, n int) []string {
	f := make([]string, n)
	for i := range f {
		f[i] = fmt.Sprint(i)
	}
	f[0] = "1"
	if n > 1 {
		f[1] = "贵州茅台"
	}
	if n > 2 {
		f[2] = code
	}
	if n > 30 {
		f[30] = "20240709153000"
	}
	return f
}
func line(key string, f []string) string { return "v_" + key + "=\"" + strings.Join(f, "~") + "\";" }
func gbkWrite(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(body))
	if err != nil {
		t.Error(err)
		return
	}
	w.Write(encoded)
}
func TestQuoteHTTP(t *testing.T) {
	tests := []struct {
		name, key string
		run       func(*SDK) (any, error)
		wantLen   int
	}{
		{"cn", "sh600519", func(s *SDK) (any, error) {
			return s.Quotes.CN(context.Background(), []string{"600519", "sh600519", "AAPL", "930955"})
		}, 1},
		{"simple", "s_sh600519", func(s *SDK) (any, error) { return s.Quotes.CNSimple(context.Background(), []string{"600519"}) }, 1},
		{"us", "usAAPL", func(s *SDK) (any, error) { return s.Quotes.US(context.Background(), []string{"105.AAPL", "usAAPL"}) }, 1},
		{"fund", "jj110011", func(s *SDK) (any, error) { return s.Quotes.Fund(context.Background(), []string{"110011", "110011"}) }, 1},
		{"largeOrder", "s_pksh600519", func(s *SDK) (any, error) { return s.Quotes.LargeOrder(context.Background(), []string{"600519"}) }, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if got := r.URL.Query().Get("q"); got != tt.key {
					t.Errorf("q=%q want %q", got, tt.key)
				}
				if r.URL.Query().Get("proxy") != "1" {
					t.Error("lost proxy query")
				}
				if r.Header.Get("Referer") == "" {
					t.Error("missing referer")
				}
				gbkWrite(t, w, line(tt.key, fields("600519", 74))+line(tt.key, []string{"1"})+line("pv_none_match", []string{"1"})+line("unrequested", fields("000001", 74)))
			}))
			defer srv.Close()
			sdk, err := New(Options{TencentURL: srv.URL + "/?proxy=1"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := tt.run(sdk)
			if err != nil {
				t.Fatal(err)
			}
			rv := reflect.ValueOf(result)
			if rv.Len() != tt.wantLen || calls != 1 {
				t.Fatalf("len=%d calls=%d", rv.Len(), calls)
			}
			if tt.name != "largeOrder" {
				encoded, _ := json.Marshal(result)
				if !strings.Contains(string(encoded), "贵州茅台") {
					t.Fatalf("GBK decode failed: %s", encoded)
				}
			}
		})
	}
}
func TestHKRealtimeFallback(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		requests = append(requests, q)
		switch q {
		case "r_hk00700,r_hk09988,hkHSI":
			gbkWrite(t, w, line("r_hk00700", fields("00700", 50))+line("r_hk09988", fields("09988", 37))+line("hkHSI", fields("HSI", 38)))
		case "hk09988":
			gbkWrite(t, w, line("hk09988", fields("09988", 38)))
		default:
			t.Errorf("unexpected request: %s", q)
		}
	}))
	defer srv.Close()
	sdk, _ := New(Options{TencentURL: srv.URL})
	quotes, err := sdk.Quotes.HK(context.Background(), []string{"700", "00700", "09988", "HSI"})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 3 || quotes[0].Code != "00700" || quotes[1].Code != "HSI" || quotes[1].AssetType != "index" || quotes[2].Code != "09988" {
		t.Fatalf("%+v", quotes)
	}
	if !reflect.DeepEqual(requests, []string{"r_hk00700,r_hk09988,hkHSI", "hk09988"}) {
		t.Fatal(requests)
	}
	if quotes[0].Currency != "" {
		t.Fatal("numeric tail misidentified as currency")
	}
}
func TestEmptyAndInvalidQuotesDoNotRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") }))
	defer srv.Close()
	sdk, _ := New(Options{TencentURL: srv.URL})
	ctx := context.Background()
	for _, run := range []func() (any, error){func() (any, error) { return sdk.Quotes.CN(ctx, nil) }, func() (any, error) { return sdk.Quotes.CNSimple(ctx, []string{"?"}) }, func() (any, error) { return sdk.Quotes.HK(ctx, nil) }, func() (any, error) { return sdk.Quotes.US(ctx, nil) }, func() (any, error) { return sdk.Quotes.Fund(ctx, nil) }, func() (any, error) { return sdk.Quotes.LargeOrder(ctx, nil) }} {
		result, err := run()
		if err != nil || reflect.ValueOf(result).IsNil() || reflect.ValueOf(result).Len() != 0 {
			t.Fatalf("result=%v err=%v", result, err)
		}
	}
	_, err := sdk.Quotes.Fund(ctx, []string{"1&bad"})
	if sdkerrors.CodeOf(err) != sdkerrors.InvalidSymbol {
		t.Fatal(err)
	}
}
func TestQuoteErrorPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) }))
	defer srv.Close()
	sdk, _ := New(Options{TencentURL: srv.URL})
	_, err := sdk.Quotes.CN(context.Background(), []string{"600519"})
	if sdkerrors.CodeOf(err) != sdkerrors.RateLimited {
		t.Fatal(err)
	}
}
func TestIndependentConcurrentSDKs(t *testing.T) {
	server := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f := fields("600519", 38)
			f[1] = name
			gbkWrite(t, w, line("sh600519", f))
		}))
	}
	a, b := server("甲"), server("乙")
	defer a.Close()
	defer b.Close()
	sa, _ := New(Options{TencentURL: a.URL})
	sb, _ := New(Options{TencentURL: b.URL})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		for _, v := range []struct {
			s    *SDK
			name string
		}{{sa, "甲"}, {sb, "乙"}} {
			wg.Add(1)
			go func(s *SDK, name string) {
				defer wg.Done()
				q, err := s.Quotes.CN(context.Background(), []string{"600519"})
				if err != nil || len(q) != 1 || q[0].Name != name {
					t.Errorf("%v %v", q, err)
				}
			}(v.s, v.name)
		}
	}
	wg.Wait()
}
func TestNewValidation(t *testing.T) {
	if _, err := New(); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{}, Options{}); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
	for _, options := range []Options{{TencentURL: "ftp://bad"}, {SearchURL: "/relative"}, {TimelineURL: "https://host/#fragment"}, {MaxRetries: -1}} {
		if _, err := New(options); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatalf("%+v: %v", options, err)
		}
	}
}

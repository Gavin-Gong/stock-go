package stockgo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestSDKRequestGovernanceWiring(t *testing.T) {
	seen := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/codes":
			if r.Header.Get("X-Provider") != "linkdiary" {
				t.Error(r.Header)
			}
			w.Write([]byte(`{"success":true,"list":["sh600519"]}`))
		case "/calendar":
			if r.Header.Get("X-Provider") != "linkdiary" {
				t.Error(r.Header)
			}
			w.Write([]byte("2024-01-02"))
		default:
			w.Write([]byte("invalid JSON"))
		}
	}))
	defer srv.Close()
	sdk, e := New(Options{CodeURLs: CodeEndpoints{CN: srv.URL + "/codes"}, TradingCalendarURL: srv.URL + "/calendar", KlineURLs: KlineEndpoints{CN: srv.URL + "/cn"}, Request: &RequestOptions{ProviderPolicies: map[string]ProviderPolicy{"linkdiary": {Headers: http.Header{"X-Provider": {"linkdiary"}}}, "eastmoney": {CircuitBreaker: &CircuitBreakerOptions{FailureThreshold: 1}}}, Hooks: &RequestHooks{Trace: func(event string, rc RequestContext) {
		if event == "request" {
			seen = append(seen, rc.Provider)
		}
	}}}})
	if e != nil {
		t.Fatal(e)
	}
	if sdk.Requests != sdk.client {
		t.Fatal("public and service request clients differ")
	}
	ctx := context.Background()
	if _, e = sdk.Codes.CN(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = sdk.Reference.TradingCalendar(ctx); e != nil {
		t.Fatal(e)
	}
	var target any
	e = sdk.Requests.GetJSON(ctx, "eastmoney", srv.URL+"/trip", &target)
	if sdkerrors.CodeOf(e) != sdkerrors.Parse {
		t.Fatal(e)
	}
	// CN's no-retry primary view must observe the same circuit rather than bypass it.
	if e = sdk.Kline.probe.GetJSON(ctx, "eastmoney", srv.URL+"/cn", &target); sdkerrors.CodeOf(e) != sdkerrors.CircuitOpen {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(seen, []string{"linkdiary", "linkdiary", "eastmoney"}) {
		t.Fatal(seen)
	}
	if _, e = New(Options{Request: &RequestOptions{RateLimit: &RateLimitOptions{RequestsPerSecond: -1}}}); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
		t.Fatal(e)
	}
}
func TestPublicRequestClientAndLegacyOverrides(t *testing.T) {
	c, e := NewRequestClient(RequestOptions{Timeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	if InferProvider("https://qt.gtimg.cn/") != "tencent" || c == nil || c.Timeout() != time.Second {
		t.Fatal("public request helpers")
	}
	lifetime, cancel := context.WithCancel(context.Background())
	cancel()
	sdk, e := New(Options{Request: &RequestOptions{ClientContext: lifetime}, Timeout: time.Second, MaxRetries: 1, BaseDelay: time.Millisecond, MaxBodyBytes: 100, HTTPClient: http.DefaultClient})
	if e != nil {
		t.Fatal(e)
	}
	_, e = sdk.Requests.Get(context.Background(), "test", "https://host/", nil)
	if sdkerrors.CodeOf(e) != sdkerrors.Aborted || !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

package stockgo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/cache"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestPublicFetchHelpers(t *testing.T) {
	var attempts, requests, hookErrors atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		if r.URL.Path == "/status" {
			w.WriteHeader(404)
			return
		}
		if r.URL.Path == "/retry" && attempts.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		if r.URL.Path == "/bad" {
			fmt.Fprint(w, `cb({bad})`)
			return
		}
		if r.URL.Path == "/big" {
			fmt.Fprint(w, strings.Repeat("x", 1000))
			return
		}
		if r.Header.Get("X-Test") != "custom" {
			t.Error("missing headers")
		}
		if r.URL.Path == "/vars" {
			fmt.Fprint(w, `var code="110011";var rows=[['single'],{"value":1}];var bad=run();`)
			return
		}
		cb := r.URL.Query().Get("cb")
		if cb == "" {
			cb = r.URL.Query().Get("callback")
		}
		if strings.HasPrefix(r.URL.Path, "/path/") {
			cb = strings.TrimPrefix(r.URL.Path, "/path/")
		}
		if !strings.HasPrefix(cb, "__stock_go_jsonp_") {
			t.Errorf("invalid callback %q", cb)
		}
		fmt.Fprintf(w, "/* comment */ %s({\"ok\":true});", cb)
	}))
	defer server.Close()
	client, e := NewRequestClient(RequestOptions{Headers: http.Header{"X-Test": {"custom"}}, MaxRetries: 1, BaseDelay: time.Millisecond, Hooks: &RequestHooks{OnRequest: func(RequestContext) { requests.Add(1) }, OnError: func(RequestContext, error) { hookErrors.Add(1) }}})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	vars, e := FetchJSVars(ctx, server.URL+"/vars", []string{"code", "rows", "bad", "missing"}, FetchJSVarsOptions{Client: client})
	if e != nil || len(vars) != 2 || string(vars["code"]) != `"110011"` {
		t.Fatal(vars, e)
	}
	for _, o := range []JSONPOptions{{Client: client}, {Client: client, CallbackParam: "cb"}, {Client: client, CallbackMode: "path"}, {Headers: http.Header{"X-Test": {"custom"}}}} {
		endpoint := server.URL + "/jsonp?original=yes"
		if o.CallbackMode == "path" {
			endpoint = server.URL + "/path/{callback}"
		}
		body, e := JSONPRequest(ctx, endpoint, o)
		if e != nil || string(body) != `{"ok":true}` {
			t.Fatal(string(body), e)
		}
	}
	if _, e := FetchJSVars(ctx, server.URL+"/vars", []string{"code"}, FetchJSVarsOptions{Headers: http.Header{"X-Test": {"custom"}}}); e != nil {
		t.Fatal(e)
	}
	if _, e := JSONPRequest(ctx, server.URL+"/retry", JSONPOptions{Client: client}); e != nil || attempts.Load() != 2 {
		t.Fatal(e, attempts.Load())
	}
	if _, e := JSONPRequest(ctx, server.URL+"/bad", JSONPOptions{Client: client}); sdkerrors.CodeOf(e) != sdkerrors.Parse {
		t.Fatal(e)
	}
	if requests.Load() < 7 || hookErrors.Load() < 1 {
		t.Fatal("client hooks bypassed", requests.Load(), hookErrors.Load())
	}
	if _, e := FetchJSVars(ctx, server.URL+"/status", nil); sdkerrors.CodeOf(e) != sdkerrors.HTTP {
		t.Fatal(e)
	}
	if _, e := JSONPRequest(ctx, server.URL+"/slow", JSONPOptions{Timeout: 10 * time.Millisecond}); sdkerrors.CodeOf(e) != sdkerrors.Timeout {
		t.Fatal(e)
	}
	if _, e := FetchJSVars(ctx, server.URL+"/slow", nil, FetchJSVarsOptions{Client: client, Timeout: 10 * time.Millisecond}); sdkerrors.CodeOf(e) != sdkerrors.Timeout {
		t.Fatal(e)
	}
	small, _ := NewRequestClient(RequestOptions{MaxBodyBytes: 10})
	if _, e := JSONPRequest(ctx, server.URL+"/big", JSONPOptions{Client: small}); sdkerrors.CodeOf(e) != sdkerrors.Parse {
		t.Fatal(e)
	}
	for _, o := range []JSONPOptions{{Timeout: -1}, {CallbackMode: "eval"}, {CallbackMode: "path"}} {
		if _, e := JSONPRequest(ctx, server.URL, o); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
			t.Fatal(e)
		}
	}
	if _, e := JSONPRequest(ctx, "%xx"); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
		t.Fatal(e)
	}
	if _, e := JSONPRequest(ctx, server.URL, JSONPOptions{}, JSONPOptions{}); e == nil {
		t.Fatal("extra options")
	}
	if _, e := FetchJSVars(ctx, server.URL, nil, FetchJSVarsOptions{}, FetchJSVarsOptions{}); e == nil {
		t.Fatal("extra options")
	}
	if _, e := FetchJSVars(ctx, server.URL, nil, FetchJSVarsOptions{Timeout: -1}); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := FetchJSVars(canceled, server.URL, nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := FetchJSVars(ctx, "ftp://bad", nil); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
		t.Fatal(e)
	}
}
func TestPublicTimeAndNumbers(t *testing.T) {
	for _, local := range []string{"", "bad", "2024/07/09 09:30"} {
		if ParseMarketTime(local, MarketTZCN) != nil {
			t.Fatal(local)
		}
	}
	for _, clock := range []string{"", "9:30", "bad"} {
		if BuildTimeMetaFromDateAndTime("20240709", clock, MarketTZCN).Timestamp != nil {
			t.Fatal(clock)
		}
	}
	if BuildTimeMetaFromDateAndTime(" 20240709 ", " 09:30 ", MarketTZCN).Timestamp == nil {
		t.Fatal("trimmed components")
	}
	for _, date := range []string{"", "bad", "2024-07-09 09:30"} {
		if BuildTimeMetaFromDateAndTime(date, "09:30", MarketTZCN).Timestamp != nil {
			t.Fatal(date)
		}
	}
	n := int64(1<<63 - 1)
	if FormatInTZ(&n, MarketTZCN) != "" || FormatInTZ(nil, MarketTZCN) != "" || FormatInTZ(new(int64), "unknown") != "" || ParseMarketTime("20240709", "unknown") != nil {
		t.Fatal("invalid time accepted")
	}
	if got := FormatInTZ(ParseMarketTime("2024-02-30", MarketTZCN), MarketTZCN); got != "2024-03-01 00:00" {
		t.Fatal(got)
	}
	if got := FormatInTZ(ParseMarketTime("0099-07-09", MarketTZCN), MarketTZCN); got != "1999-07-09 00:00" {
		t.Fatal(got)
	}
	for _, v := range []any{math.NaN(), true, []int{1}, "0x_bad", "0x+1", "0x-1", "0b2", "0x" + strings.Repeat("f", 400)} {
		if ToFiniteNumberOrNull(v) != nil {
			t.Fatal(v)
		}
	}
	for _, v := range []any{int64(3), uint64(7), float32(1.5)} {
		if ToFiniteNumberOrNull(v) == nil {
			t.Fatal(v)
		}
	}
	for _, v := range []any{math.Inf(1), []any{math.Inf(-1)}} {
		if got := ToNumberSafe(v); got == nil || !math.IsInf(*got, 0) {
			t.Fatal(v, got)
		}
	}
	num := 1.5
	if v := ToNumberSafe(&num); v == nil || *v != num {
		t.Fatal(v)
	}
	var nilPtr *int
	if ToNumberSafe(nilPtr) != nil {
		t.Fatal("nil pointer")
	}
	if ToNumberSafe(struct{ X int }{3}) != nil {
		t.Fatal("struct number")
	}
	if _, e := ExtractJSONFromJSONP(`cb({});run()`); sdkerrors.CodeOf(e) != sdkerrors.Parse {
		t.Fatal(e)
	}
}
func TestSDKPublicCacheClear(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"success":true,"list":["sh600519"]}`)
	}))
	defer srv.Close()
	sdk, e := New(Options{CodeURLs: CodeEndpoints{CN: srv.URL}})
	if e != nil {
		t.Fatal(e)
	}
	other, _ := New()
	foreign, _ := cache.GetClientScoped(other.Requests, "helper")
	foreign.Set("x", 10)
	if sdk.Requests.CacheScope() != sdk.Requests.WithoutRetries().CacheScope() {
		t.Fatal("probe scope differs")
	}
	for _, clear := range []func(){sdk.ClearCaches, sdk.Requests.ClearCaches, func() { cache.ClearClientScoped(sdk.Requests) }} {
		before := calls.Load()
		for i := 0; i < 2; i++ {
			if _, e := sdk.Codes.CN(context.Background()); e != nil {
				t.Fatal(e)
			}
		}
		if calls.Load() > before+1 {
			t.Fatal("memo missed")
		}
		scope, e := cache.GetClientScoped(sdk.Requests, "helper")
		if e != nil {
			t.Fatal(e)
		}
		scope.Set("x", 1)
		afterMemo := calls.Load()
		clear()
		if scope.Has("x") {
			t.Fatal("public cache remained")
		}
		if _, e := sdk.Codes.CN(context.Background()); e != nil {
			t.Fatal(e)
		}
		if calls.Load() != afterMemo+1 {
			t.Fatal("memo was not cleared")
		}
		if !foreign.Has("x") {
			t.Fatal("other instance cleared")
		}
	}
	sdk.Requests.RegisterCacheClearer(nil)
}
func TestAsyncPoolOrderingBoundAndCancellation(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		start2, start3 := make(chan struct{}), make(chan struct{})
		release0, release2 := make(chan struct{}), make(chan struct{})
		done := make(chan struct {
			values []int
			err    error
		}, 1)
		tasks := []func(context.Context) (int, error){func(context.Context) (int, error) { <-release0; return 0, nil }, func(context.Context) (int, error) { return 1, nil }, func(context.Context) (int, error) { close(start2); <-release2; return 2, nil }, func(context.Context) (int, error) { close(start3); return 3, nil }}
		go func() {
			v, e := AsyncPool(context.Background(), tasks, 2, ordered)
			done <- struct {
				values []int
				err    error
			}{v, e}
		}()
		<-start2
		close(release0)
		<-start3
		close(release2)
		result := <-done
		if result.err != nil {
			t.Fatal(result.err)
		}
		if ordered {
			if !reflect.DeepEqual(result.values, []int{0, 1, 2, 3}) {
				t.Fatal(result.values)
			}
		} else {
			if !reflect.DeepEqual(result.values[:2], []int{1, 0}) {
				t.Fatal("completion order", result.values)
			}
			sort.Ints(result.values)
			if !reflect.DeepEqual(result.values, []int{0, 1, 2, 3}) {
				t.Fatal(result.values)
			}
		}
	}
	var active, peak atomic.Int32
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	tasks := make([]func(context.Context) (int, error), 20)
	for i := range tasks {
		tasks[i] = func(context.Context) (int, error) {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
			}
			started <- struct{}{}
			<-release
			return 1, nil
		}
	}
	done := make(chan error, 1)
	go func() { _, e := AsyncPool(context.Background(), tasks, 3); done <- e }()
	for i := 0; i < 3; i++ {
		<-started
	}
	close(release)
	// Drain subsequent start notifications while the remaining tasks finish.
	go func() {
		for i := 0; i < 17; i++ {
			<-started
		}
	}()
	if e := <-done; e != nil || peak.Load() != 3 || active.Load() != 0 {
		t.Fatal(e, peak.Load(), active.Load())
	}
	marker := errors.New("pool failed")
	tasks = []func(context.Context) (int, error){func(context.Context) (int, error) { return 0, marker }, func(ctx context.Context) (int, error) { <-ctx.Done(); return 0, ctx.Err() }}
	if v, e := AsyncPool(context.Background(), tasks, 2); v != nil || !errors.Is(e, marker) {
		t.Fatal(v, e)
	}
	if _, e := AsyncPool(context.Background(), []func(context.Context) (int, error){func(context.Context) (int, error) { panic("pool") }}, 1); e == nil {
		t.Fatal("panic")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := AsyncPool(ctx, tasks, 1); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	ctx, cancel = context.WithCancel(context.Background())
	if _, e := AsyncPool(ctx, []func(context.Context) (int, error){func(context.Context) (int, error) { cancel(); return 1, nil }}, 1); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if v, e := AsyncPool[int](context.Background(), nil, 1); e != nil || v == nil || len(v) != 0 {
		t.Fatal(v, e)
	}
	if _, e := AsyncPool(context.Background(), tasks, 0); e == nil {
		t.Fatal("concurrency")
	}
	if _, e := AsyncPool(context.Background(), tasks, 1, false, true); e == nil {
		t.Fatal("extra flag")
	}
	if _, e := AsyncPool(context.Background(), []func(context.Context) (int, error){nil}, 1); e == nil {
		t.Fatal("nil task")
	}
	items := []int{1, 2, 3}
	chunks, e := ChunkArray(items, int(^uint(0)>>1))
	if e != nil {
		t.Fatal(e)
	}
	chunks[0][0] = 99
	if items[0] != 1 {
		t.Fatal("chunk mutated source")
	}
	if _, e := ChunkArray(items, 0); e == nil {
		t.Fatal("chunk size")
	}
}

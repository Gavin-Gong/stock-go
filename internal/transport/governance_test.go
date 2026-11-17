package transport

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func intPtr(n int) *int    { return &n }
func boolPtr(v bool) *bool { return &v }
func TestProviderPoliciesOverridesAndIsolation(t *testing.T) {
	header := http.Header{"x-base": {"base"}, "X-Override": {"global"}}
	statuses := []int{418}
	count := 2
	var seen []string
	var times []time.Duration
	doer := doerFunc(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.Header.Get("User-Agent")+":"+r.Header.Get("X-Base")+":"+r.Header.Get("X-Override"))
		deadline, _ := r.Context().Deadline()
		times = append(times, time.Until(deadline))
		return response(200, "ok"), nil
	})
	c, err := New(Config{HTTPClient: doer, Timeout: time.Second, Headers: header, UserAgent: "global", Retry: &RetryOptions{MaxRetries: &count, RetryableStatusCodes: statuses}, RateLimit: &RateLimitOptions{RequestsPerSecond: 1000}, CircuitBreaker: &CircuitBreakerOptions{FailureThreshold: 2}, ProviderPolicies: map[string]ProviderPolicy{"sina": {Timeout: 2 * time.Second, UserAgent: "sina", Headers: http.Header{"X-Override": {"provider"}}, DisableRateLimit: true, DisableCircuitBreaker: true}, "eastmoney": {Headers: http.Header{"uSeR-aGeNt": {"explicit"}}, CircuitBreaker: &CircuitBreakerOptions{HalfOpenRequests: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	header.Set("X-Base", "mutated")
	statuses[0] = 404
	count = 9
	for _, provider := range []string{"tencent", "sina", "eastmoney"} {
		if _, e := c.Get(context.Background(), provider, "https://host/", nil); e != nil {
			t.Fatal(e)
		}
	}
	if !reflect.DeepEqual(seen, []string{"global:base:global", "sina:base:provider", "explicit:base:global"}) {
		t.Fatal(seen)
	}
	if times[0] > time.Second || times[0] < time.Second/2 || times[1] > 2*time.Second || times[1] < time.Second {
		t.Fatal(times)
	}
	tx, si, em := c.state("tencent"), c.state("sina"), c.state("eastmoney")
	if tx == em || tx.limiter == em.limiter || tx.breaker == em.breaker || si.limiter != nil || si.breaker != nil || em.breaker.options.FailureThreshold != 2 || em.breaker.options.HalfOpenRequests != 2 {
		t.Fatal("provider state was shared or merged incorrectly")
	}
	if tx.policy.retry.count != 2 || !reflect.DeepEqual(tx.policy.retry.statuses, []int{418}) {
		t.Fatal(tx.policy.retry)
	}
}
func TestConfiguredRetryAndHookSequence(t *testing.T) {
	count := 0
	events := []string{}
	var delays []time.Duration
	var attempts []int
	hooks := &Hooks{OnRequest: func(rc RequestContext) {
		attempts = append(attempts, rc.Attempt)
		if rc.ResponseType != "json" {
			t.Error(rc)
		}
	}, OnResponse: func(rc RequestContext, m ResponseMeta) {
		if m.Status != 418 && m.Status != 200 {
			t.Error(m)
		}
	}, OnError: func(rc RequestContext, e error) {
		if sdkerrors.CodeOf(e) != sdkerrors.HTTP {
			t.Error(e)
		}
	}, OnRetry: func(rc RequestContext, e error, d time.Duration) { delays = append(delays, d) }, Trace: func(event string, rc RequestContext) { events = append(events, event) }}
	c, err := New(Config{HTTPClient: doerFunc(func(r *http.Request) (*http.Response, error) {
		count++
		if count < 3 {
			return response(418, "bad"), nil
		}
		return response(200, `{"ok":true}`), nil
	}), Retry: &RetryOptions{MaxRetries: intPtr(2), BaseDelay: time.Nanosecond, MaxDelay: time.Second, BackoffMultiplier: 3, RetryableStatusCodes: []int{418}}, Hooks: hooks})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]bool
	if e := c.GetJSON(context.Background(), "test", "https://host/", &result); e != nil || !result["ok"] {
		t.Fatal(result, e)
	}
	want := []string{"request", "response", "error", "retry", "request", "response", "error", "retry", "request", "response"}
	if !reflect.DeepEqual(events, want) || !reflect.DeepEqual(attempts, []int{0, 1, 2}) || !reflect.DeepEqual(delays, []time.Duration{1, 3}) {
		t.Fatal(events, attempts, delays)
	}
	// Hooks are copied, panic-safe, and cannot turn a successful request into a failure.
	hooks.Trace = func(string, RequestContext) { panic("mutation") }
	c.hooks = Hooks{OnRequest: func(RequestContext) { panic("request") }, OnResponse: func(RequestContext, ResponseMeta) { panic("response") }, Trace: func(string, RequestContext) { panic("trace") }}
	if _, e := c.Get(context.Background(), "test", "https://host/", nil); e != nil {
		t.Fatal(e)
	}
}
func TestRetryClassificationAndPerCallOverride(t *testing.T) {
	for _, mode := range []string{"timeoutOff", "networkOff", "statusesEmpty", "default501", "custom501", "perCallZero", "perCallDoer", "onRetryPanic"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			other := 0
			retry := &RetryOptions{MaxRetries: intPtr(1), BaseDelay: time.Nanosecond}
			switch mode {
			case "timeoutOff":
				retry.RetryOnTimeout = boolPtr(false)
			case "networkOff":
				retry.RetryOnNetworkError = boolPtr(false)
			case "statusesEmpty":
				retry.RetryableStatusCodes = []int{}
			case "custom501":
				retry.RetryableStatusCodes = []int{501}
			case "onRetryPanic":
				retry.OnRetry = func(int, error, time.Duration) { panic("retry") }
			}
			c, _ := New(Config{Timeout: time.Millisecond, Retry: retry, HTTPClient: doerFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				switch mode {
				case "timeoutOff":
					<-r.Context().Done()
					return nil, r.Context().Err()
				case "networkOff":
					return nil, errors.New("offline")
				case "default501", "custom501":
					return response(501, ""), nil
				}
				return response(503, ""), nil
			})})
			o := GetOptions{Provider: "test"}
			if mode == "perCallZero" {
				o.Retry = &RetryOptions{MaxRetries: intPtr(0)}
			}
			if mode == "perCallDoer" {
				o.HTTPClient = doerFunc(func(*http.Request) (*http.Response, error) { other++; return response(200, "ok"), nil })
			}
			_, e := c.GetWithOptions(context.Background(), "https://host/", o)
			want := 1
			if mode == "custom501" || mode == "onRetryPanic" {
				want = 2
			}
			if mode == "perCallDoer" {
				want = 0
				if e != nil || other != 1 {
					t.Fatal(e, other)
				}
			} else if e == nil {
				t.Fatal("lost error")
			}
			if calls != want {
				t.Fatal(calls, want)
			}
		})
	}
}
func TestFallbackBudgetJSONValidationAndCooldown(t *testing.T) {
	calls := []string{}
	events := []string{}
	c, _ := New(Config{MaxRetries: 2, BaseDelay: time.Nanosecond, HTTPClient: doerFunc(func(r *http.Request) (*http.Response, error) {
		host := r.URL.Hostname()
		calls = append(calls, host)
		if host == "push2his.eastmoney.com" {
			return response(503, "fail"), nil
		}
		if host == "7.push2his.eastmoney.com" {
			return response(200, `{"partial":1,broken`), nil
		}
		return response(200, `{"ok":true}`), nil
	}), Hooks: &Hooks{Trace: func(event string, rc RequestContext) { events = append(events, event) }}})
	target := map[string]any{"old": true}
	if e := c.GetJSON(context.Background(), "eastmoney", "https://push2his.eastmoney.com/path?type=1,2", &target); e != nil {
		t.Fatal(e)
	}
	want := []string{"push2his.eastmoney.com", "push2his.eastmoney.com", "push2his.eastmoney.com", "7.push2his.eastmoney.com", "33.push2his.eastmoney.com"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatal(calls)
	}
	if len(target) != 1 || target["ok"] != true {
		t.Fatal("failed parse mutated final target", target)
	}
	health := c.HostHealth("eastmoney")
	if len(health) != 3 {
		t.Fatal(health)
	}
	for _, h := range health {
		if h.Host == "33.push2his.eastmoney.com" {
			if h.SuccessCount != 1 || h.FailureCount != 0 {
				t.Fatal(h)
			}
		} else if h.FailureCount != 1 || h.CooldownUntil <= time.Now().UnixMilli() {
			t.Fatal(h)
		}
	}
	calls = nil
	if e := c.GetJSON(context.Background(), "eastmoney", "https://push2his.eastmoney.com/path?type=1,2", &target); e != nil || !reflect.DeepEqual(calls, []string{"33.push2his.eastmoney.com"}) {
		t.Fatal(calls, e)
	}
	fallbacks := 0
	for _, e := range events {
		if e == "fallback" {
			fallbacks++
		}
	}
	if fallbacks != 2 {
		t.Fatal(events)
	}
}
func TestFallbackDisabledAndProviderInference(t *testing.T) {
	for _, mode := range []string{"clientOff", "callOff", "400", "fakeHost", "customPort", "auth", "infer", "raw"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			endpoint := "https://push2.eastmoney.com/path"
			if mode == "fakeHost" {
				endpoint = "https://push2.eastmoney.com.evil.test/path"
			}
			if mode == "customPort" {
				endpoint = "https://push2.eastmoney.com:123/path"
			}
			if mode == "auth" {
				endpoint = "https://user:pass@push2.eastmoney.com/path"
			}
			c, _ := New(Config{DisableHostFallback: mode == "clientOff", HTTPClient: doerFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if mode == "raw" {
					return response(200, "not JSON"), nil
				}
				if calls > 1 {
					return response(200, "ok"), nil
				}
				status := 503
				if mode == "400" {
					status = 400
				}
				return response(status, "fail"), nil
			})})
			o := GetOptions{Provider: "eastmoney"}
			if mode == "callOff" {
				o.HostFallback = boolPtr(false)
			}
			if mode == "infer" {
				o.Provider = ""
			}
			_, e := c.GetWithOptions(context.Background(), endpoint, o)
			if mode == "infer" {
				if e != nil || calls != 2 {
					t.Fatal(calls, e)
				}
			} else if mode == "raw" {
				if e != nil || calls != 1 {
					t.Fatal(e, calls)
				}
			} else if e == nil || calls != 1 {
				t.Fatal(e, calls)
			}
		})
	}
	for raw, want := range map[string]string{"https://qt.gtimg.cn/": "tencent", "https://assets.linkdiary.cn/": "linkdiary", "https://stock.finance.sina.com.cn/": "sina", "https://push2.eastmoney.com/": "eastmoney", "https://eastmoney.com.evil.test/": "unknown", "bad": "unknown"} {
		if p := InferProvider(raw, ""); p != want {
			t.Fatal(raw, p, want)
		}
	}
}
func TestCircuitLogicalFailuresAbortAndSharedProbe(t *testing.T) {
	var count atomic.Int32
	var fail atomic.Bool
	fail.Store(true)
	c, _ := New(Config{MaxRetries: 1, BaseDelay: time.Nanosecond, HTTPClient: doerFunc(func(r *http.Request) (*http.Response, error) {
		count.Add(1)
		if fail.Load() {
			return response(503, "fail"), nil
		}
		return response(200, "ok"), nil
	}), CircuitBreaker: &CircuitBreakerOptions{FailureThreshold: 2, ResetTimeout: 2 * time.Millisecond}})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, e := c.Get(ctx, "test", "https://host/", nil); e == nil {
			t.Fatal("expected failure")
		}
	}
	if count.Load() != 4 {
		t.Fatal("retries counted as separate circuit failures", count.Load())
	}
	if _, e := c.Get(ctx, "test", "https://host/", nil); sdkerrors.CodeOf(e) != sdkerrors.CircuitOpen {
		t.Fatal(e)
	}
	stats, ok := c.CircuitStats("test")
	if !ok || stats.State != Open || stats.FailureCount != 2 {
		t.Fatal(stats, ok)
	}
	if _, ok := c.CircuitStats("other"); !ok {
		t.Fatal("global circuit wasn't inherited")
	}
	// Probe view shares circuit/rate/host state, while suppressing configured retries.
	probe := c.WithoutRetries()
	if probe.state("test") != c.state("test") {
		t.Fatal("state not shared")
	}
	time.Sleep(3 * time.Millisecond)
	fail.Store(false)
	if _, e := probe.Get(ctx, "test", "https://host/", nil); e != nil {
		t.Fatal(e)
	}
	stats, _ = c.CircuitStats("test")
	if stats.State != Closed {
		t.Fatal(stats)
	}
	aborted, cancel := context.WithCancel(ctx)
	cancel()
	before := count.Load()
	if _, e := c.Get(aborted, "test", "https://host/", nil); sdkerrors.CodeOf(e) != sdkerrors.Aborted || count.Load() != before {
		t.Fatal(e, count.Load())
	}
}
func TestLimiterConcurrentCancellationAndProviderIsolation(t *testing.T) {
	var mu sync.Mutex
	times := []time.Time{}
	c, _ := New(Config{HTTPClient: doerFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		return response(200, "ok"), nil
	}), RateLimit: &RateLimitOptions{RequestsPerSecond: 100, MaxBurst: 1}})
	ctx := context.Background()
	if _, e := c.Get(ctx, "test", "https://host/", nil); e != nil {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := c.Get(canceled, "test", "https://host/", nil); sdkerrors.CodeOf(e) != sdkerrors.Aborted {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := c.Get(ctx, "test", "https://host/", nil); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if len(times) != 5 {
		t.Fatal(times)
	}
	for i := 1; i < len(times); i++ {
		if times[i].Sub(times[i-1]) < 8*time.Millisecond {
			t.Fatal("concurrent token bucket burst", times)
		}
	}
	a, b := c.state("test"), c.state("other")
	if a.limiter == b.limiter {
		t.Fatal("rate limiter shared across providers")
	}
	l := newLimiter(RateLimitOptions{RequestsPerSecond: 1, MaxBurst: 1})
	if e := l.acquire(ctx); e != nil {
		t.Fatal(e)
	}
	deadline, stop := context.WithTimeout(ctx, 2*time.Millisecond)
	defer stop()
	if e := l.acquire(deadline); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if len(l.gate) != 1 {
		t.Fatal("canceled waiter leaked gate")
	}
}
func TestGovernanceInvalidConfig(t *testing.T) {
	configs := []Config{{MaxBodyBytes: math.MaxInt64}, {Retry: &RetryOptions{MaxRetries: intPtr(11)}}, {Retry: &RetryOptions{BaseDelay: -1}}, {Retry: &RetryOptions{MaxDelay: -1}}, {Retry: &RetryOptions{BackoffMultiplier: 0.5}}, {Retry: &RetryOptions{BackoffMultiplier: math.NaN()}}, {Retry: &RetryOptions{Jitter: -1}}, {Retry: &RetryOptions{RetryableStatusCodes: []int{99}}}, {RateLimit: &RateLimitOptions{RequestsPerSecond: -1}}, {RateLimit: &RateLimitOptions{RequestsPerSecond: math.Inf(1)}}, {RateLimit: &RateLimitOptions{MaxBurst: 0.5}}, {CircuitBreaker: &CircuitBreakerOptions{FailureThreshold: -1}}, {CircuitBreaker: &CircuitBreakerOptions{HalfOpenRequests: -1}}, {CircuitBreaker: &CircuitBreakerOptions{ResetTimeout: -1}}, {CircuitBreaker: &CircuitBreakerOptions{ProbeRecycleTimeout: -1}}, {CircuitBreaker: &CircuitBreakerOptions{ResetTimeout: time.Duration(math.MaxInt64)}}, {ProviderPolicies: map[string]ProviderPolicy{"": {}}}, {ProviderPolicies: map[string]ProviderPolicy{"test": {Timeout: -1}}}}
	for i, cfg := range configs {
		if _, e := New(cfg); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
			t.Fatalf("config %d: %v", i, e)
		}
	}
}

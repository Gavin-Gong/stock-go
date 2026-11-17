package transport

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func assertSource(t *testing.T, got, want any) {
	t.Helper()
	switch expected := want.(type) {
	case map[string]any:
		actual, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("got %T want map", got)
		}
		for k, v := range expected {
			if _, exists := actual[k]; !exists {
				t.Fatalf("missing field %s: %v", k, got)
			}
			assertSource(t, actual[k], v)
		}
	case []any:
		actual, ok := got.([]any)
		if !ok || len(actual) != len(expected) {
			t.Fatalf("got %v want %v", got, want)
		}
		for i, v := range expected {
			assertSource(t, actual[i], v)
		}
	case float64:
		actual, ok := got.(float64)
		if !ok || math.Abs(actual-expected) > 1e-9 {
			t.Fatalf("got %v want %v", got, want)
		}
	default:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
func jsonValue(v any) any { b, _ := json.Marshal(v); var out any; json.Unmarshal(b, &out); return out }
func parityPolicy(raw json.RawMessage) ProviderPolicy {
	var v struct {
		Timeout         int
		Retry           *RetryOptions
		Headers         map[string]string
		UserAgent       string
		RotateUserAgent *bool
		RateLimit       *RateLimitOptions
		CircuitBreaker  *CircuitBreakerOptions
	}
	json.Unmarshal(raw, &v)
	o := ProviderPolicy{Timeout: time.Duration(v.Timeout) * time.Millisecond, Retry: v.Retry, UserAgent: v.UserAgent, RotateUserAgent: v.RotateUserAgent, RateLimit: v.RateLimit, CircuitBreaker: v.CircuitBreaker, Headers: make(http.Header)}
	for k, val := range v.Headers {
		o.Headers.Set(k, val)
	}
	if o.Retry != nil {
		o.Retry.BaseDelay *= time.Millisecond
		o.Retry.MaxDelay *= time.Millisecond
	}
	if o.CircuitBreaker != nil {
		o.CircuitBreaker.ResetTimeout *= time.Millisecond
		o.CircuitBreaker.ProbeRecycleTimeout *= time.Millisecond
	}
	return o
}
func TestGovernanceUpstreamParity(t *testing.T) {
	body, e := os.ReadFile("../../testdata/governance-parity.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixtures []struct {
		Kind, Name, Endpoint, Provider, Code string
		Status                               int
		Options, Base, Override, Expected    json.RawMessage
		Steps                                []string
	}
	if e = json.Unmarshal(body, &fixtures); e != nil {
		t.Fatal(e)
	}
	for i, f := range fixtures {
		t.Run(fmt.Sprintf("%s/%s/%d", f.Kind, f.Name, i), func(t *testing.T) {
			now := time.UnixMilli(1_000_000)
			var actual any
			switch f.Kind {
			case "breaker":
				var o CircuitBreakerOptions
				json.Unmarshal(f.Options, &o)
				o.ResetTimeout *= time.Millisecond
				o.ProbeRecycleTimeout *= time.Millisecond
				if o.FailureThreshold == 0 {
					o.FailureThreshold = 5
				}
				if o.HalfOpenRequests == 0 {
					o.HalfOpenRequests = 1
				}
				if o.ResetTimeout == 0 {
					o.ResetTimeout = 30 * time.Second
				}
				if o.ProbeRecycleTimeout == 0 {
					o.ProbeRecycleTimeout = max(4*o.ResetTimeout, 120*time.Second)
				}
				b := newBreaker(o)
				held := []permit{}
				out := []any{}
				for _, step := range f.Steps {
					var allowed *bool
					if strings.HasPrefix(step, "advance:") {
						n, _ := strconv.Atoi(strings.TrimPrefix(step, "advance:"))
						now = now.Add(time.Duration(n) * time.Millisecond)
					} else {
						switch step {
						case "can":
							p, ok := b.acquire(now)
							allowed = &ok
							if ok {
								held = append(held, p)
							}
						case "success", "failure", "release":
							p := permit{epoch: b.epoch}
							for len(held) > 0 {
								p = held[0]
								held = held[1:]
								if b.state != HalfOpen {
									break
								}
								if _, ok := b.probes[p.id]; ok {
									break
								}
							}
							b.finish(p, step == "success", step == "release", now)
						case "reset":
							b.reset()
							held = nil
						}
					}
					item := jsonValue(b.stats(now)).(map[string]any)
					if allowed != nil {
						item["allowed"] = *allowed
					}
					out = append(out, item)
				}
				actual = out
			case "limiter":
				var o RateLimitOptions
				json.Unmarshal(f.Options, &o)
				l := newLimiter(o)
				l.last = now
				out := []any{}
				for _, step := range f.Steps {
					var acquired *bool
					if strings.HasPrefix(step, "advance:") {
						n, _ := strconv.Atoi(strings.TrimPrefix(step, "advance:"))
						now = now.Add(time.Duration(n) * time.Millisecond)
					}
					if step == "try" {
						ok, _ := l.take(now)
						acquired = &ok
					} else {
						l.tokens = math.Min(l.burst, l.tokens+now.Sub(l.last).Seconds()*l.rate)
						l.last = now
					}
					wait := 0.0
					if l.tokens < 1 {
						wait = math.Ceil((1 - l.tokens) / (l.rate / 1000))
					}
					item := map[string]any{"tokens": l.tokens, "wait": wait}
					if acquired != nil {
						item["acquired"] = *acquired
					}
					out = append(out, item)
				}
				actual = out
			case "hosts":
				h := hostManager{states: map[string]HostHealthStats{}}
				out := []any{}
				for _, step := range f.Steps {
					switch step {
					case "failure":
						h.record(f.Endpoint, &sdkerrors.Error{Code: sdkerrors.Network}, now)
					case "success":
						h.record(f.Endpoint, nil, now)
					}
					if strings.HasPrefix(step, "advance:") {
						n, _ := strconv.Atoi(strings.TrimPrefix(step, "advance:"))
						now = now.Add(time.Duration(n) * time.Millisecond)
					}
					out = append(out, map[string]any{"urls": h.candidates(f.Endpoint, f.Provider, now), "stats": h.stats("")})
				}
				actual = out
			case "fallback":
				code := sdkerrors.Code(f.Code)
				if f.Status != 0 {
					code = sdkerrors.HTTP
				}
				actual = shouldFallback(&sdkerrors.Error{Code: code, Status: f.Status})
			case "policy":
				base := policy{timeout: 30 * time.Second, retry: retryPolicy{count: 3, base: time.Second, max: 30 * time.Second, multiplier: 2, statuses: []int{408, 429, 500, 502, 503, 504}, network: true, timeout: true}, headers: make(http.Header)}
				base, e = applyPolicy(base, parityPolicy(f.Base))
				if e != nil {
					t.Fatal(e)
				}
				p, e := applyPolicy(base, parityPolicy(f.Override))
				if e != nil {
					t.Fatal(e)
				}
				headers := map[string]string{}
				for k, v := range p.headers {
					headers[k] = v[0]
				}
				if _, ok := headers["User-Agent"]; !ok && p.userAgent != "" {
					headers["User-Agent"] = p.userAgent
				}
				actual = map[string]any{"timeout": p.timeout.Milliseconds(), "retry": map[string]any{"maxRetries": p.retry.count, "baseDelay": p.retry.base.Milliseconds(), "maxDelay": p.retry.max.Milliseconds(), "backoffMultiplier": p.retry.multiplier, "retryableStatusCodes": p.retry.statuses, "retryOnNetworkError": p.retry.network, "retryOnTimeout": p.retry.timeout}, "headers": headers, "rotateUserAgent": p.rotate, "rateLimit": map[string]any{"requestsPerSecond": p.rate.RequestsPerSecond, "maxBurst": p.rate.MaxBurst}, "circuitBreaker": map[string]any{"failureThreshold": p.circuit.FailureThreshold, "resetTimeout": p.circuit.ResetTimeout.Milliseconds(), "halfOpenRequests": p.circuit.HalfOpenRequests}}
			}
			var expected any
			json.Unmarshal(f.Expected, &expected)
			assertSource(t, jsonValue(actual), expected)
		})
	}
}

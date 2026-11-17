package transport

import (
	"context"
	"errors"
	"math"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestHalfOpenAdmissionRecoveryAndStaleResults(t *testing.T) {
	now := time.Unix(1000, 0)
	transitions := [][2]CircuitState{}
	b := newBreaker(CircuitBreakerOptions{FailureThreshold: 1, ResetTimeout: 10 * time.Millisecond, HalfOpenRequests: 2, ProbeRecycleTimeout: 20 * time.Millisecond, OnStateChange: func(from, to CircuitState) { transitions = append(transitions, [2]CircuitState{from, to}) }})
	p, _ := b.acquire(now)
	b.finish(p, false, false, now)
	if _, ok := b.acquire(now.Add(time.Millisecond)); ok {
		t.Fatal("open breaker admitted request")
	}
	now = now.Add(10 * time.Millisecond)
	p1, ok := b.acquire(now)
	if !ok {
		t.Fatal("first probe denied")
	}
	p2, ok := b.acquire(now.Add(15 * time.Millisecond))
	if !ok {
		t.Fatal("second probe denied")
	}
	if _, ok = b.acquire(now.Add(19 * time.Millisecond)); ok {
		t.Fatal("half-open concurrency exceeded")
	}
	p3, ok := b.acquire(now.Add(20 * time.Millisecond))
	if !ok {
		t.Fatal("expired probe not recycled")
	}
	b.finish(p1, false, false, now.Add(21*time.Millisecond))
	if s := b.stats(now.Add(21 * time.Millisecond)); s.State != HalfOpen || s.InFlight != 2 {
		t.Fatal("stale completion stole a live probe", s)
	}
	b.finish(p2, true, false, now.Add(22*time.Millisecond))
	if s := b.stats(now.Add(22 * time.Millisecond)); s.State != HalfOpen || s.InFlight != 1 || s.HalfOpenSuccessCount != 1 {
		t.Fatal(s)
	}
	b.finish(p3, true, false, now.Add(23*time.Millisecond))
	if s := b.stats(now.Add(23 * time.Millisecond)); s.State != Closed || s.FailureCount != 0 {
		t.Fatal(s)
	}
	b.finish(p1, false, false, now.Add(24*time.Millisecond))
	if s := b.stats(now.Add(24 * time.Millisecond)); s.State != Closed || s.FailureCount != 0 {
		t.Fatal("stale generation changed circuit", s)
	}
	want := [][2]CircuitState{{Closed, Open}, {Open, HalfOpen}, {HalfOpen, Closed}}
	if !reflect.DeepEqual(transitions, want) {
		t.Fatal(transitions)
	}
}
func TestHalfOpenAbortReleasesExactProbe(t *testing.T) {
	now := time.Unix(1000, 0)
	b := newBreaker(CircuitBreakerOptions{FailureThreshold: 1, ResetTimeout: time.Second, HalfOpenRequests: 2, ProbeRecycleTimeout: time.Minute})
	p, _ := b.acquire(now)
	b.finish(p, false, false, now)
	now = now.Add(time.Second)
	first, _ := b.acquire(now)
	second, _ := b.acquire(now)
	b.finish(second, false, true, now)
	if _, ok := b.probes[first.id]; !ok {
		t.Fatal("aborting second probe released first")
	}
	third, ok := b.acquire(now)
	if !ok {
		t.Fatal("abort leaked probe slot")
	}
	b.finish(first, true, false, now)
	b.finish(third, false, false, now)
	if s := b.stats(now); s.State != Open {
		t.Fatal(s)
	}
	b.reset()
	s := b.stats(now)
	if s.State != Closed || s.LastFailureTime != 0 || s.InFlight != 0 {
		t.Fatal(s)
	}
}
func TestClientCancellationReleasesProbeAndDoesNotPoisonHealth(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	c, _ := New(Config{HTTPClient: doerFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return response(200, "ok"), nil
	}), CircuitBreaker: &CircuitBreakerOptions{FailureThreshold: 1, ResetTimeout: time.Second}})
	b := c.state("test").breaker
	now := time.Now().Add(-2 * time.Second)
	p, _ := b.acquire(now)
	b.finish(p, false, false, now)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := c.Get(ctx, "test", "https://host/", nil); done <- e }()
	<-started
	if _, e := c.Get(context.Background(), "test", "https://host/", nil); sdkerrors.CodeOf(e) != sdkerrors.CircuitOpen {
		t.Fatal(e)
	}
	cancel()
	if e := <-done; sdkerrors.CodeOf(e) != sdkerrors.Aborted {
		t.Fatal(e)
	}
	if len(c.HostHealth("")) != 0 {
		t.Fatal(c.HostHealth(""))
	}
	if _, e := c.Get(context.Background(), "test", "https://host/", nil); e != nil {
		t.Fatal(e)
	}
	s, _ := c.CircuitStats("test")
	if s.State != Closed {
		t.Fatal(s)
	}
}
func TestRetryAfterBoundedAndDateParsing(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for value, want := range map[string]time.Duration{"2": 2 * time.Second, "0": 0, "-1": 0, "invalid": 0, "999999": 24 * time.Hour, now.Add(3 * time.Second).Format(http.TimeFormat): 3 * time.Second, now.Add(-time.Second).Format(http.TimeFormat): 0} {
		if got := parseRetryAfter(value, now); got != want {
			t.Fatal(value, got, want)
		}
	}
	calls := 0
	var delay time.Duration
	c, _ := New(Config{Retry: &RetryOptions{MaxRetries: intPtr(1), BaseDelay: time.Nanosecond, MaxDelay: 2 * time.Millisecond}, HTTPClient: doerFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			r := response(429, "")
			r.Header.Set("Retry-After", "999")
			return r, nil
		}
		return response(200, "ok"), nil
	}), Hooks: &Hooks{OnRetry: func(rc RequestContext, e error, d time.Duration) { delay = d }}})
	if _, e := c.Get(context.Background(), "test", "https://host/", nil); e != nil || delay != 2*time.Millisecond || calls != 2 {
		t.Fatal(e, delay, calls)
	}
	p := retryPolicy{base: time.Millisecond, max: 2 * time.Millisecond, multiplier: 2, jitter: time.Millisecond}
	for i := 0; i < 5; i++ {
		d := backoff(10, p)
		if d < 2*time.Millisecond || d >= 3*time.Millisecond {
			t.Fatal(d)
		}
	}
}
func TestClientLifetimeAndPerCallValidation(t *testing.T) {
	lifetime, stop := context.WithCancel(context.Background())
	var calls atomic.Int32
	c, _ := New(Config{ClientContext: lifetime, HTTPClient: doerFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		stop()
		<-r.Context().Done()
		return nil, r.Context().Err()
	}), CircuitBreaker: &CircuitBreakerOptions{FailureThreshold: 1}})
	if _, e := c.Get(context.Background(), "test", "https://host/", nil); sdkerrors.CodeOf(e) != sdkerrors.Aborted {
		t.Fatal(e)
	}
	if _, e := c.Get(context.Background(), "test", "https://host/", nil); sdkerrors.CodeOf(e) != sdkerrors.Aborted || calls.Load() != 1 {
		t.Fatal(e, calls.Load())
	}
	if len(c.HostHealth("")) != 0 {
		t.Fatal(c.HostHealth(""))
	}
	stats, _ := c.CircuitStats("test")
	if stats.FailureCount != 0 {
		t.Fatal(stats)
	}
	c, _ = New(Config{HTTPClient: doerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request performed HTTP")
		return nil, nil
	})})
	if _, e := c.GetWithOptions(context.Background(), "https://host/", GetOptions{Retry: &RetryOptions{MaxRetries: intPtr(-1)}}); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
		t.Fatal(e)
	}
	for _, target := range []any{nil, (*int)(nil), 123} {
		if e := c.GetJSON(context.Background(), "test", "https://host/", target); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
			t.Fatal(e)
		}
	}
	if _, e := c.Get(context.Background(), "test", ":bad", nil); sdkerrors.CodeOf(e) != sdkerrors.InvalidArgument {
		t.Fatal(e)
	}
}
func TestJSONParseFailureAndPanicSafeErrorHooks(t *testing.T) {
	calls := 0
	events := []string{}
	c, _ := New(Config{MaxRetries: 2, HTTPClient: doerFunc(func(*http.Request) (*http.Response, error) { calls++; return response(200, `{"leaked":1,bad`), nil }), CircuitBreaker: &CircuitBreakerOptions{FailureThreshold: 1}, Hooks: &Hooks{OnError: func(RequestContext, error) { panic("error") }, OnRetry: func(RequestContext, error, time.Duration) { panic("retry") }, Trace: func(event string, rc RequestContext) { events = append(events, event) }}})
	target := map[string]int{"original": 1}
	if e := c.GetJSON(context.Background(), "test", "https://host/", &target); sdkerrors.CodeOf(e) != sdkerrors.Parse {
		t.Fatal(e)
	}
	if calls != 1 || !reflect.DeepEqual(target, map[string]int{"original": 1}) {
		t.Fatal(calls, target)
	}
	s, _ := c.CircuitStats("test")
	if s.State != Open {
		t.Fatal(s)
	}
	if !reflect.DeepEqual(events, []string{"request", "response", "error"}) {
		t.Fatal(events)
	}
	c.ResetCircuit("test")
	s, _ = c.CircuitStats("test")
	if s.State != Closed {
		t.Fatal(s)
	}
	if _, ok := c.CircuitStats("disabled"); !ok {
		t.Fatal("global policy missing")
	}
	noBreaker, _ := New(Config{})
	if _, ok := noBreaker.CircuitStats("test"); ok {
		t.Fatal("unexpected default breaker")
	}
	noBreaker.ResetCircuit("test")
}

type failingBody struct{ closed bool }

func (b *failingBody) Read([]byte) (int, error) { return 0, errors.New("body failure") }
func (b *failingBody) Close() error             { b.closed = true; return nil }
func TestDoerAndBodyFailures(t *testing.T) {
	for _, mode := range []string{"nilResponse", "nilBody", "readFailure", "responseAndError", "bodyTimeout"} {
		t.Run(mode, func(t *testing.T) {
			body := &failingBody{}
			c, _ := New(Config{Timeout: time.Millisecond, HTTPClient: doerFunc(func(r *http.Request) (*http.Response, error) {
				switch mode {
				case "nilResponse":
					return nil, nil
				case "nilBody":
					return &http.Response{StatusCode: 200}, nil
				case "responseAndError":
					return &http.Response{Body: body}, errors.New("fail")
				case "bodyTimeout":
					<-r.Context().Done()
					return &http.Response{StatusCode: 200, Body: body}, nil
				}
				return &http.Response{StatusCode: 200, Body: body}, nil
			})})
			_, e := c.Get(context.Background(), "test", "https://host/", nil)
			want := sdkerrors.Network
			if mode == "bodyTimeout" {
				want = sdkerrors.Timeout
			}
			if sdkerrors.CodeOf(e) != want {
				t.Fatal(e)
			}
			if mode != "nilResponse" && mode != "nilBody" && !body.closed {
				t.Fatal("body not closed")
			}
		})
	}
}
func TestRotationConcurrencyAndDefaultPolicyMerging(t *testing.T) {
	var mu sync.Mutex
	agents := []string{}
	doer := doerFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		agents = append(agents, r.Header.Get("User-Agent"))
		mu.Unlock()
		if len(r.Header.Values("User-Agent")) != 1 {
			t.Error(r.Header)
		}
		return response(200, "ok"), nil
	})
	c, _ := New(Config{HTTPClient: doer, RotateUserAgent: true, Headers: http.Header{"user-agent": {"override"}}, RateLimit: &RateLimitOptions{}, CircuitBreaker: &CircuitBreakerOptions{}, ProviderPolicies: map[string]ProviderPolicy{"sina": {RateLimit: &RateLimitOptions{RequestsPerSecond: 10}, CircuitBreaker: &CircuitBreakerOptions{ResetTimeout: time.Minute}}}})
	si := c.state("sina")
	if si.policy.rate.MaxBurst != 10 || si.policy.circuit.ProbeRecycleTimeout != 4*time.Minute {
		t.Fatal(si.policy)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := c.Get(context.Background(), "sina", "https://host/", nil); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	counts := map[string]int{}
	for _, a := range agents {
		counts[a]++
	}
	if len(counts) != len(userAgents) {
		t.Fatal(len(counts))
	}
	for _, n := range counts {
		if n != 2 {
			t.Fatal(counts)
		}
	}
	c2, _ := New(Config{HTTPClient: doer, RotateUserAgent: true})
	if _, e := c2.Get(context.Background(), "sina", "https://host/", nil); e != nil || agents[len(agents)-1] != userAgents[0] {
		t.Fatal(e, agents)
	}
	// Callback reentrancy and panics cannot deadlock the breaker mutex.
	var b *breaker
	b = newBreaker(CircuitBreakerOptions{FailureThreshold: 1, ResetTimeout: time.Second, HalfOpenRequests: 1, ProbeRecycleTimeout: time.Minute, OnStateChange: func(CircuitState, CircuitState) { _ = b.stats(time.Now()); panic("callback") }})
	p, _ := b.acquire(time.Now())
	b.finish(p, false, false, time.Now())
	if b.stats(time.Now()).State != Open {
		t.Fatal("callback changed state")
	}
}
func TestRateLimitEveryAttemptAndQueuedAbort(t *testing.T) {
	times := []time.Time{}
	c, _ := New(Config{MaxRetries: 1, BaseDelay: time.Nanosecond, RateLimit: &RateLimitOptions{RequestsPerSecond: 100, MaxBurst: 1}, HTTPClient: doerFunc(func(*http.Request) (*http.Response, error) {
		times = append(times, time.Now())
		return response(503, ""), nil
	})})
	if _, e := c.Get(context.Background(), "test", "https://host/", nil); e == nil {
		t.Fatal("missing failure")
	}
	if len(times) != 2 || times[1].Sub(times[0]) < 8*time.Millisecond {
		t.Fatal(times)
	}
	l := newLimiter(RateLimitOptions{RequestsPerSecond: math.SmallestNonzeroFloat64, MaxBurst: 1})
	l.tokens = 0
	ok, wait := l.take(time.Now())
	if ok || wait != 24*time.Hour {
		t.Fatal(ok, wait)
	}
	l.gate = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := l.acquire(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

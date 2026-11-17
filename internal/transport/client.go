// Package transport provides the SDK's shared HTTP request lifecycle.
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Gavin-Gong/stock-go/cache"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}
type Config struct {
	HTTPClient          Doer
	Timeout             time.Duration
	MaxRetries          int
	BaseDelay           time.Duration
	MaxBodyBytes        int64
	Retry               *RetryOptions
	Headers             http.Header
	UserAgent           string
	RotateUserAgent     bool
	RateLimit           *RateLimitOptions
	CircuitBreaker      *CircuitBreakerOptions
	ProviderPolicies    map[string]ProviderPolicy
	Hooks               *Hooks
	ClientContext       context.Context
	DisableHostFallback bool
}
type runtime struct {
	policy  policy
	limiter *limiter
	breaker *breaker
}
type shared struct {
	scope    *cache.Scope
	clearers []func()
	mu       sync.Mutex
	states   map[string]*runtime
	hosts    hostManager
	ua       atomic.Uint64
}

// Client is safe for concurrent use when its Doer and callbacks are. Configuration is copied at construction.
type Client struct {
	http                  Doer
	defaultPolicy         policy
	policies              map[string]policy
	shared                *shared
	maxBody               int64
	hooks                 Hooks
	clientContext         context.Context
	noFallback, noRetries bool
}

func New(cfg Config) (*Client, error) {
	if cfg.Timeout < 0 || cfg.MaxRetries < 0 || cfg.MaxRetries > 10 || cfg.BaseDelay < 0 || cfg.MaxBodyBytes < 0 || cfg.MaxBodyBytes == math.MaxInt64 {
		return nil, invalid("invalid HTTP configuration (retries 0..10; nonnegative durations and size)")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.BaseDelay == 0 {
		cfg.BaseDelay = 500 * time.Millisecond
	}
	if cfg.MaxBodyBytes == 0 {
		cfg.MaxBodyBytes = 32 << 20
	}
	base := policy{timeout: cfg.Timeout, retry: retryPolicy{count: cfg.MaxRetries, base: cfg.BaseDelay, max: 30 * time.Second, multiplier: 2, statuses: []int{408, 429, 500, 502, 503, 504}, network: true, timeout: true}, headers: make(http.Header)}
	p, err := applyPolicy(base, ProviderPolicy{Timeout: cfg.Timeout, Retry: cfg.Retry, Headers: cfg.Headers, UserAgent: cfg.UserAgent, RotateUserAgent: &cfg.RotateUserAgent, RateLimit: cfg.RateLimit, CircuitBreaker: cfg.CircuitBreaker})
	if err != nil {
		return nil, err
	}
	c := &Client{http: cfg.HTTPClient, defaultPolicy: p, policies: map[string]policy{}, shared: &shared{scope: cache.NewScope(), states: map[string]*runtime{}, hosts: hostManager{states: map[string]HostHealthStats{}}}, maxBody: cfg.MaxBodyBytes, clientContext: cfg.ClientContext, noFallback: cfg.DisableHostFallback}
	if cfg.Hooks != nil {
		c.hooks = *cfg.Hooks
	}
	for provider, o := range cfg.ProviderPolicies {
		if strings.TrimSpace(provider) == "" {
			return nil, invalid("provider policy name must not be empty")
		}
		resolved, e := applyPolicy(p, o)
		if e != nil {
			return nil, e
		}
		c.policies[provider] = resolved
	}
	return c, nil
}

// WithoutRetries shares provider state and host health, while suppressing all host retry budgets.
func (c *Client) WithoutRetries() *Client { out := *c; out.noRetries = true; return &out }
func (c *Client) state(provider string) *runtime {
	c.shared.mu.Lock()
	defer c.shared.mu.Unlock()
	if s := c.shared.states[provider]; s != nil {
		return s
	}
	p, ok := c.policies[provider]
	if !ok {
		p = c.defaultPolicy
	}
	s := &runtime{policy: p}
	if p.rate != nil {
		s.limiter = newLimiter(*p.rate)
	}
	if p.circuit != nil {
		s.breaker = newBreaker(*p.circuit)
	}
	c.shared.states[provider] = s
	return s
}
func (c *Client) HostHealth(provider string) []HostHealthStats { return c.shared.hosts.stats(provider) }
func (c *Client) CircuitStats(provider string) (CircuitStats, bool) {
	s := c.state(provider)
	if s.breaker == nil {
		return CircuitStats{}, false
	}
	return s.breaker.stats(time.Now()), true
}
func (c *Client) Get(ctx context.Context, provider, endpoint string, headers http.Header) ([]byte, error) {
	return c.get(ctx, endpoint, GetOptions{Provider: provider, Headers: headers}, "bytes", nil)
}
func (c *Client) GetWithOptions(ctx context.Context, endpoint string, o GetOptions) ([]byte, error) {
	return c.get(ctx, endpoint, o, "bytes", nil)
}

// GetParsed validates a payload inside the lifecycle so parse failures participate in hooks, host fallback and circuits.
func (c *Client) GetParsed(ctx context.Context, provider, endpoint string, headers http.Header, validate func([]byte) error) ([]byte, error) {
	return c.get(ctx, endpoint, GetOptions{Provider: provider, Headers: headers}, "bytes", validate)
}
func (c *Client) GetJSON(ctx context.Context, provider, endpoint string, target any) error {
	return c.GetJSONWithOptions(ctx, endpoint, target, GetOptions{Provider: provider})
}
func (c *Client) GetJSONWithOptions(ctx context.Context, endpoint string, target any, o GetOptions) error {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return invalid("JSON target must be a non-nil pointer")
	}
	var decoded reflect.Value
	_, err := c.get(ctx, endpoint, o, "json", func(body []byte) error {
		next := reflect.New(value.Elem().Type())
		if err := json.Unmarshal(body, next.Interface()); err != nil {
			return err
		}
		decoded = next.Elem()
		return nil
	})
	if err != nil {
		return err
	}
	value.Elem().Set(decoded)
	return nil
}
func (c *Client) emit(event string, rc RequestContext, fn func()) {
	if fn != nil {
		safe(fn)
	}
	if c.hooks.Trace != nil {
		safe(func() { c.hooks.Trace(event, rc) })
	}
}
func (c *Client) get(ctx context.Context, endpoint string, o GetOptions, responseType string, validate func([]byte) error) ([]byte, error) {
	provider := InferProvider(endpoint, o.Provider)
	if err := ctx.Err(); err != nil {
		return nil, contextError(err, provider, endpoint)
	}
	if c.clientContext != nil {
		merged, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(c.clientContext, cancel)
		defer func() { stop(); cancel() }()
		if c.clientContext.Err() != nil {
			cancel()
		}
		ctx = merged
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(err, provider, endpoint)
	}
	s := c.state(provider)
	retry, err := applyRetry(s.policy.retry, o.Retry)
	if err != nil {
		return nil, err
	}
	if c.noRetries {
		retry.count = 0
	}
	var token permit
	if s.breaker != nil {
		var ok bool
		token, ok = s.breaker.acquire(time.Now())
		if !ok {
			return nil, &sdkerrors.Error{Code: sdkerrors.CircuitOpen, Message: "circuit breaker rejected request", Provider: provider, URL: endpoint}
		}
	}
	// Classify the logical request only once, after every retry and fallback has settled.
	successful, aborted := false, false
	defer func() {
		if s.breaker != nil {
			s.breaker.finish(token, successful, aborted, time.Now())
		}
	}()
	candidates := []string{endpoint}
	if !c.noFallback && (o.HostFallback == nil || *o.HostFallback) {
		candidates = c.shared.hosts.candidates(endpoint, provider, time.Now())
	}
	for index, candidate := range candidates {
		budget := retry
		if index > 0 {
			budget.count = 0
		}
		body, e := c.execute(ctx, provider, candidate, s, budget, o, responseType, validate)
		if e == nil {
			successful = true
			c.shared.hosts.record(candidate, nil, time.Now())
			return body, nil
		}
		aborted = sdkerrors.CodeOf(e) == sdkerrors.Aborted || (sdkerrors.CodeOf(e) == sdkerrors.Timeout && ctx.Err() != nil)
		if aborted {
			return nil, e
		}
		c.shared.hosts.record(candidate, e, time.Now())
		if ctx.Err() != nil || index == len(candidates)-1 || !shouldFallback(e) {
			return nil, e
		}
		c.emit("fallback", RequestContext{Provider: provider, URL: candidate, Timeout: s.policy.timeout, ResponseType: responseType}, nil)
	}
	return nil, invalid("request has no candidate URLs")
}
func (c *Client) execute(ctx context.Context, provider, endpoint string, s *runtime, p retryPolicy, o GetOptions, responseType string, validate func([]byte) error) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		rc := RequestContext{Provider: provider, URL: endpoint, Timeout: s.policy.timeout, Attempt: attempt, ResponseType: responseType}
		body, after, err := c.once(ctx, s, rc, o, validate)
		if err == nil {
			return body, nil
		}
		c.emit("error", rc, func() {
			if c.hooks.OnError != nil {
				c.hooks.OnError(rc, err)
			}
		})
		if ctx.Err() != nil || attempt >= p.count || !shouldRetry(err, p) {
			return nil, err
		}
		delay := backoff(attempt, p)
		if after > delay {
			delay = min(after, p.max)
		}
		if p.onRetry != nil {
			safe(func() { p.onRetry(attempt+1, err, delay) })
		}
		c.emit("retry", rc, func() {
			if c.hooks.OnRetry != nil {
				c.hooks.OnRetry(rc, err, delay)
			}
		})
		if err := sleepContext(ctx, delay); err != nil {
			return nil, contextError(err, provider, endpoint)
		}
	}
}
func backoff(attempt int, p retryPolicy) time.Duration {
	n := float64(p.base) * math.Pow(p.multiplier, float64(attempt))
	if n >= float64(p.max) {
		n = float64(p.max)
	}
	delay := time.Duration(n)
	if p.jitter > 0 {
		delay += time.Duration(rand.Int64N(int64(p.jitter)))
	}
	return delay
}
func shouldRetry(err error, p retryPolicy) bool {
	var e *sdkerrors.Error
	if !asError(err, &e) {
		return false
	}
	if e.Code == sdkerrors.Aborted {
		return false
	}
	if e.Code == sdkerrors.Timeout {
		return p.timeout
	}
	if e.Code == sdkerrors.Network {
		return p.network
	}
	if e.Status != 0 {
		for _, status := range p.statuses {
			if e.Status == status {
				return true
			}
		}
	}
	return false
}
func asError(err error, target **sdkerrors.Error) bool { return errors.As(err, target) }

func contextError(err error, provider, endpoint string) *sdkerrors.Error {
	code := sdkerrors.Aborted
	if errors.Is(err, context.DeadlineExceeded) {
		code = sdkerrors.Timeout
	}
	return &sdkerrors.Error{Code: code, Message: err.Error(), Provider: provider, URL: endpoint, Cause: err}
}
func parseRetryAfter(value string, now time.Time) time.Duration {
	if n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
		if n <= 0 {
			return 0
		}
		if n > int64((24*time.Hour)/time.Second) {
			return 24 * time.Hour
		}
		return time.Duration(n) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(0, at.Sub(now))
	}
	return 0
}
func (c *Client) once(ctx context.Context, s *runtime, rc RequestContext, o GetOptions, validate func([]byte) error) ([]byte, time.Duration, error) {
	if s.limiter != nil {
		if err := s.limiter.acquire(ctx); err != nil {
			return nil, 0, contextError(err, rc.Provider, rc.URL)
		}
	}
	attemptCtx, cancel := context.WithTimeout(ctx, s.policy.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, rc.URL, nil)
	if err != nil {
		return nil, 0, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "invalid request URL", Provider: rc.Provider, URL: rc.URL, Cause: err}
	}
	req.Header = mergeHeaders(s.policy.headers, o.Headers)
	if s.policy.rotate {
		n := c.shared.ua.Add(1) - 1
		req.Header.Set("User-Agent", userAgents[n%uint64(len(userAgents))])
	}
	if req.Header.Get("User-Agent") == "" && s.policy.userAgent != "" {
		req.Header.Set("User-Agent", s.policy.userAgent)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 stock-go")
	}
	c.emit("request", rc, func() {
		if c.hooks.OnRequest != nil {
			c.hooks.OnRequest(rc)
		}
	})
	started := time.Now()
	doer := c.http
	if o.HTTPClient != nil {
		doer = o.HTTPClient
	}
	resp, err := doer.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if attemptCtx.Err() != nil {
			return nil, 0, contextError(attemptCtx.Err(), rc.Provider, rc.URL)
		}
		return nil, 0, &sdkerrors.Error{Code: sdkerrors.Network, Message: "request failed", Provider: rc.Provider, URL: rc.URL, Cause: err}
	}
	if resp == nil || resp.Body == nil {
		return nil, 0, &sdkerrors.Error{Code: sdkerrors.Network, Message: "HTTP client returned no response/body", Provider: rc.Provider, URL: rc.URL}
	}
	defer resp.Body.Close()
	c.emit("response", rc, func() {
		if c.hooks.OnResponse != nil {
			c.hooks.OnResponse(rc, ResponseMeta{Status: resp.StatusCode, Duration: time.Since(started)})
		}
	})
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := sdkerrors.HTTP
		if resp.StatusCode == 429 {
			code = sdkerrors.RateLimited
		}
		return nil, parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()), &sdkerrors.Error{Code: code, Message: resp.Status, Provider: rc.Provider, URL: rc.URL, Status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		if attemptCtx.Err() != nil {
			return nil, 0, contextError(attemptCtx.Err(), rc.Provider, rc.URL)
		}
		return nil, 0, &sdkerrors.Error{Code: sdkerrors.Network, Message: "reading response body failed", Provider: rc.Provider, URL: rc.URL, Cause: err}
	}
	if int64(len(body)) > c.maxBody {
		return nil, 0, &sdkerrors.Error{Code: sdkerrors.Parse, Message: fmt.Sprintf("response exceeds %d bytes", c.maxBody), Provider: rc.Provider, URL: rc.URL}
	}
	if validate != nil {
		if err := validate(body); err != nil {
			return nil, 0, &sdkerrors.Error{Code: sdkerrors.Parse, Message: "invalid response payload", Provider: rc.Provider, URL: rc.URL, Cause: err}
		}
	}
	return body, 0, nil
}

// ResetCircuit clears failures and invalidates in-flight probes for a provider.
func (c *Client) ResetCircuit(provider string) {
	s := c.state(provider)
	if s.breaker != nil {
		s.breaker.reset()
	}
}

// Timeout returns the default per-attempt timeout before provider overrides.
func (c *Client) Timeout() time.Duration { return c.defaultPolicy.timeout }

// CacheScope is shared with WithoutRetries copies and isolated from other clients.
func (c *Client) CacheScope() *cache.Scope { return c.shared.scope }

// RegisterCacheClearer attaches instance-owned caches to ClearCaches.
func (c *Client) RegisterCacheClearer(clear func()) {
	if clear == nil {
		return
	}
	c.shared.mu.Lock()
	defer c.shared.mu.Unlock()
	c.shared.clearers = append(c.shared.clearers, clear)
}

// ClearCaches invalidates public scopes and SDK-owned caches, including pending writes.
func (c *Client) ClearCaches() {
	c.shared.scope.Clear()
	c.shared.mu.Lock()
	callbacks := append([]func(){}, c.shared.clearers...)
	c.shared.mu.Unlock()
	for _, clear := range callbacks {
		clear()
	}
}

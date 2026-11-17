package transport

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type RetryOptions struct {
	MaxRetries                          *int
	BaseDelay, MaxDelay                 time.Duration
	BackoffMultiplier                   float64
	RetryableStatusCodes                []int
	RetryOnNetworkError, RetryOnTimeout *bool
	Jitter                              time.Duration
	OnRetry                             func(attempt int, err error, delay time.Duration)
}
type RateLimitOptions struct{ RequestsPerSecond, MaxBurst float64 }
type CircuitBreakerOptions struct {
	FailureThreshold, HalfOpenRequests int
	ResetTimeout, ProbeRecycleTimeout  time.Duration
	OnStateChange                      func(from, to CircuitState)
}
type ProviderPolicy struct {
	Timeout                                 time.Duration
	Retry                                   *RetryOptions
	Headers                                 http.Header
	UserAgent                               string
	RotateUserAgent                         *bool
	RateLimit                               *RateLimitOptions
	CircuitBreaker                          *CircuitBreakerOptions
	DisableRateLimit, DisableCircuitBreaker bool
}
type RequestContext struct {
	Provider, URL string
	Timeout       time.Duration
	Attempt       int
	ResponseType  string
}
type ResponseMeta struct {
	Status   int
	Duration time.Duration
}
type Hooks struct {
	OnRequest  func(RequestContext)
	OnResponse func(RequestContext, ResponseMeta)
	OnError    func(RequestContext, error)
	OnRetry    func(RequestContext, error, time.Duration)
	Trace      func(event string, ctx RequestContext)
}
type GetOptions struct {
	Provider     string
	Headers      http.Header
	Retry        *RetryOptions
	HostFallback *bool
	HTTPClient   Doer
}
type retryPolicy struct {
	count            int
	base, max        time.Duration
	multiplier       float64
	statuses         []int
	network, timeout bool
	jitter           time.Duration
	onRetry          func(int, error, time.Duration)
}
type policy struct {
	rateBurstDefault, probeRecycleDefault bool
	userAgent                             string
	timeout                               time.Duration
	retry                                 retryPolicy
	headers                               http.Header
	rotate                                bool
	rate                                  *RateLimitOptions
	circuit                               *CircuitBreakerOptions
}

func invalid(message string) error {
	return &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: message}
}
func applyRetry(base retryPolicy, o *RetryOptions) (retryPolicy, error) {
	if o == nil {
		return base, nil
	}
	if o.MaxRetries != nil {
		base.count = *o.MaxRetries
	}
	if o.BaseDelay != 0 {
		base.base = o.BaseDelay
	}
	if o.MaxDelay != 0 {
		base.max = o.MaxDelay
	}
	if o.BackoffMultiplier != 0 {
		base.multiplier = o.BackoffMultiplier
	}
	if o.RetryableStatusCodes != nil {
		base.statuses = append([]int{}, o.RetryableStatusCodes...)
	}
	if o.RetryOnNetworkError != nil {
		base.network = *o.RetryOnNetworkError
	}
	if o.RetryOnTimeout != nil {
		base.timeout = *o.RetryOnTimeout
	}
	if o.Jitter != 0 {
		base.jitter = o.Jitter
	}
	if o.OnRetry != nil {
		base.onRetry = o.OnRetry
	}
	if base.count < 0 || base.count > 10 || base.base < 0 || base.max < 0 || base.multiplier < 1 || math.IsNaN(base.multiplier) || math.IsInf(base.multiplier, 0) || base.jitter < 0 || base.jitter > time.Hour || base.max > 24*time.Hour || base.base > 24*time.Hour {
		return base, invalid("invalid retry configuration")
	}
	for _, status := range base.statuses {
		if status < 100 || status > 599 {
			return base, invalid("retry status must be 100..599")
		}
	}
	return base, nil
}
func mergeHeaders(base, override http.Header) http.Header {
	out := make(http.Header)
	for _, h := range []http.Header{base, override} {
		for key, values := range h {
			out[http.CanonicalHeaderKey(key)] = append([]string{}, values...)
		}
	}
	return out
}
func applyPolicy(base policy, o ProviderPolicy) (policy, error) {
	var err error
	base.headers = mergeHeaders(base.headers, o.Headers)
	if o.Timeout != 0 {
		base.timeout = o.Timeout
	}
	if base.timeout <= 0 {
		return base, invalid("timeout must be positive")
	}
	base.retry, err = applyRetry(base.retry, o.Retry)
	if err != nil {
		return base, err
	}
	if o.UserAgent != "" {
		base.userAgent = o.UserAgent
	}
	if o.RotateUserAgent != nil {
		base.rotate = *o.RotateUserAgent
	}
	if o.DisableRateLimit {
		base.rate = nil
		base.rateBurstDefault = false
	} else if o.RateLimit != nil {
		r := RateLimitOptions{RequestsPerSecond: 5, MaxBurst: 0}
		if base.rate != nil {
			r = *base.rate
		}
		if o.RateLimit.RequestsPerSecond != 0 {
			r.RequestsPerSecond = o.RateLimit.RequestsPerSecond
		}
		if o.RateLimit.MaxBurst != 0 {
			r.MaxBurst = o.RateLimit.MaxBurst
		}
		if (base.rate == nil || base.rateBurstDefault) && o.RateLimit.MaxBurst == 0 {
			r.MaxBurst = 0
			base.rateBurstDefault = true
		} else if o.RateLimit.MaxBurst != 0 {
			base.rateBurstDefault = false
		}
		if r.MaxBurst == 0 {
			r.MaxBurst = math.Max(1, r.RequestsPerSecond)
		}
		if r.RequestsPerSecond <= 0 || math.IsNaN(r.RequestsPerSecond) || math.IsInf(r.RequestsPerSecond, 0) || r.MaxBurst < 1 || math.IsNaN(r.MaxBurst) || math.IsInf(r.MaxBurst, 0) {
			return base, invalid("invalid rate limit")
		}
		base.rate = &r
	}
	if o.DisableCircuitBreaker {
		base.circuit = nil
		base.probeRecycleDefault = false
	} else if o.CircuitBreaker != nil {
		b := CircuitBreakerOptions{FailureThreshold: 5, HalfOpenRequests: 1, ResetTimeout: 30 * time.Second}
		if base.circuit != nil {
			b = *base.circuit
		}
		v := o.CircuitBreaker
		if v.FailureThreshold != 0 {
			b.FailureThreshold = v.FailureThreshold
		}
		if v.HalfOpenRequests != 0 {
			b.HalfOpenRequests = v.HalfOpenRequests
		}
		if v.ResetTimeout != 0 {
			b.ResetTimeout = v.ResetTimeout
		}
		if v.ProbeRecycleTimeout != 0 {
			b.ProbeRecycleTimeout = v.ProbeRecycleTimeout
		}
		if v.OnStateChange != nil {
			b.OnStateChange = v.OnStateChange
		}
		if b.ResetTimeout > time.Duration(math.MaxInt64/4) {
			return base, invalid("circuit reset timeout is too large")
		}
		if (base.circuit == nil || base.probeRecycleDefault) && v.ProbeRecycleTimeout == 0 {
			b.ProbeRecycleTimeout = 0
			base.probeRecycleDefault = true
		} else if v.ProbeRecycleTimeout != 0 {
			base.probeRecycleDefault = false
		}
		if b.ProbeRecycleTimeout == 0 {
			b.ProbeRecycleTimeout = max(b.ResetTimeout*4, 120*time.Second)
		}
		if b.FailureThreshold < 1 || b.HalfOpenRequests < 1 || b.ResetTimeout <= 0 || b.ResetTimeout > time.Duration(math.MaxInt64/4) || b.ProbeRecycleTimeout < 0 {
			return base, invalid("invalid circuit breaker configuration")
		}
		base.circuit = &b
	}
	return base, nil
}

// InferProvider uses hostname boundaries so unrelated lookalike hosts are not trusted.
func InferProvider(endpoint, explicit string) string {
	if explicit != "" {
		return explicit
	}
	u, err := url.Parse(endpoint)
	if err == nil {
		host := strings.ToLower(u.Hostname())
		for _, p := range []struct{ Domain, Name string }{{"eastmoney.com", "eastmoney"}, {"gtimg.cn", "tencent"}, {"sina.com.cn", "sina"}, {"linkdiary.cn", "linkdiary"}} {
			if host == p.Domain || strings.HasSuffix(host, "."+p.Domain) {
				return p.Name
			}
		}
	}
	return "unknown"
}
func sleepContext(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func safe(fn func()) { defer func() { _ = recover() }(); fn() }

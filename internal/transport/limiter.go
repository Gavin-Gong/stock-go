package transport

import (
	"context"
	"math"
	"sync"
	"time"
)

// A gate serializes token consumption. Canceled waiters leave the queue immediately.
type limiter struct {
	gate                chan struct{}
	mu                  sync.Mutex
	rate, burst, tokens float64
	last                time.Time
}

func newLimiter(o RateLimitOptions) *limiter {
	l := &limiter{gate: make(chan struct{}, 1), rate: o.RequestsPerSecond, burst: o.MaxBurst, tokens: o.MaxBurst, last: time.Now()}
	l.gate <- struct{}{}
	return l
}
func (l *limiter) take(now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tokens = math.Min(l.burst, l.tokens+now.Sub(l.last).Seconds()*l.rate)
	l.last = now
	if l.tokens >= 1 {
		l.tokens--
		return true, 0
	}
	seconds := (1 - l.tokens) / l.rate
	if seconds >= float64((24*time.Hour)/time.Second) {
		return false, 24 * time.Hour
	}
	return false, max(time.Nanosecond, time.Duration(math.Ceil(seconds*float64(time.Second))))
}
func (l *limiter) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.gate:
	}
	defer func() { l.gate <- struct{}{} }()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ok, wait := l.take(time.Now())
		if ok {
			return nil
		}
		if err := sleepContext(ctx, wait); err != nil {
			return err
		}
	}
}

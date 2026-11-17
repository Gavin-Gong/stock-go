package transport

import (
	"sync"
	"time"
)

type CircuitState string

const (
	Closed   CircuitState = "CLOSED"
	Open     CircuitState = "OPEN"
	HalfOpen CircuitState = "HALF_OPEN"
)

type CircuitStats struct {
	State                CircuitState `json:"state"`
	FailureCount         int          `json:"failureCount"`
	LastFailureTime      int64        `json:"lastFailureTime"`
	HalfOpenSuccessCount int          `json:"halfOpenSuccessCount"`
	InFlight             int          `json:"inFlight"`
}
type permit struct{ epoch, id uint64 }
type transition struct{ from, to CircuitState }
type breaker struct {
	mu                  sync.Mutex
	options             CircuitBreakerOptions
	state               CircuitState
	failures, successes int
	failedAt            time.Time
	epoch, next         uint64
	probes              map[uint64]time.Time
}

func newBreaker(o CircuitBreakerOptions) *breaker {
	return &breaker{options: o, state: Closed, probes: map[uint64]time.Time{}}
}
func (b *breaker) transition(to CircuitState) *transition {
	if b.state == to {
		return nil
	}
	t := &transition{b.state, to}
	b.state = to
	b.epoch++
	b.probes = map[uint64]time.Time{}
	if to == Closed {
		b.failures, b.successes = 0, 0
	}
	if to == HalfOpen {
		b.successes = 0
	}
	return t
}
func (b *breaker) notify(t *transition) {
	if t != nil && b.options.OnStateChange != nil {
		safe(func() { b.options.OnStateChange(t.from, t.to) })
	}
}
func (b *breaker) advance(now time.Time) *transition {
	if b.state == Open && now.Sub(b.failedAt) >= b.options.ResetTimeout {
		return b.transition(HalfOpen)
	}
	return nil
}
func (b *breaker) acquire(now time.Time) (permit, bool) {
	b.mu.Lock()
	change := b.advance(now)
	allowed := b.state != Open
	token := permit{epoch: b.epoch}
	if b.state == HalfOpen {
		for id, at := range b.probes {
			if now.Sub(at) >= b.options.ProbeRecycleTimeout {
				delete(b.probes, id)
			}
		}
		if len(b.probes)+b.successes >= b.options.HalfOpenRequests {
			allowed = false
		} else {
			b.next++
			token.id = b.next
			b.probes[token.id] = now
		}
	}
	b.mu.Unlock()
	b.notify(change)
	return token, allowed
}

// Each completion is matched to its generation and probe; stale responses cannot settle another request's slot.
func (b *breaker) finish(p permit, success, aborted bool, now time.Time) {
	b.mu.Lock()
	var change *transition
	if p.epoch == b.epoch {
		valid := true
		if b.state == HalfOpen {
			_, valid = b.probes[p.id]
			delete(b.probes, p.id)
		}
		if valid && !aborted {
			if success {
				if b.state == HalfOpen {
					b.successes++
					if b.successes >= b.options.HalfOpenRequests {
						change = b.transition(Closed)
					}
				} else if b.state == Closed {
					b.failures = 0
				}
			} else {
				b.failedAt = now
				if b.state == HalfOpen {
					change = b.transition(Open)
				} else if b.state == Closed {
					b.failures++
					if b.failures >= b.options.FailureThreshold {
						change = b.transition(Open)
					}
				}
			}
		}
	}
	b.mu.Unlock()
	b.notify(change)
}
func (b *breaker) stats(now time.Time) CircuitStats {
	b.mu.Lock()
	change := b.advance(now)
	last := int64(0)
	if !b.failedAt.IsZero() {
		last = b.failedAt.UnixMilli()
	}
	out := CircuitStats{State: b.state, FailureCount: b.failures, LastFailureTime: last, HalfOpenSuccessCount: b.successes, InFlight: len(b.probes)}
	b.mu.Unlock()
	b.notify(change)
	return out
}

func (b *breaker) reset() {
	b.mu.Lock()
	change := b.transition(Closed)
	b.epoch++
	b.failures, b.successes = 0, 0
	b.failedAt = time.Time{}
	b.probes = map[uint64]time.Time{}
	b.mu.Unlock()
	b.notify(change)
}

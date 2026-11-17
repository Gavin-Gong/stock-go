// Package memo provides instance-scoped, bounded memoization with shared in-flight loads.
package memo

import (
	"context"
	"sync"
	"time"
)

type entry[T any] struct {
	value   T
	expires time.Time
	used    uint64
}
type flight[T any] struct {
	done  chan struct{}
	value T
	err   error
}

// Store must be constructed with New. Values should be immutable or copied by callers.
// A load uses its initiating caller's context; waiting callers may cancel independently.
type Store[T any] struct {
	mu                sync.Mutex
	entries           map[string]entry[T]
	flights           map[string]*flight[T]
	max               int
	generation, clock uint64
}

func New[T any](max int) *Store[T] {
	if max < 1 {
		max = 1
	}
	return &Store[T]{entries: map[string]entry[T]{}, flights: map[string]*flight[T]{}, max: max}
}
func (s *Store[T]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	s.entries = map[string]entry[T]{}
	s.flights = map[string]*flight[T]{}
}
func (s *Store[T]) Load(ctx context.Context, key string, ttl time.Duration, fetch func(context.Context) (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	s.mu.Lock()
	if e, ok := s.entries[key]; ok {
		if time.Now().Before(e.expires) {
			s.clock++
			e.used = s.clock
			s.entries[key] = e
			s.mu.Unlock()
			return e.value, nil
		}
		delete(s.entries, key)
	}
	if f, ok := s.flights[key]; ok {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-f.done:
			return f.value, f.err
		}
	}
	f := &flight[T]{done: make(chan struct{})}
	s.flights[key] = f
	generation := s.generation
	s.mu.Unlock()
	value, err := fetch(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	f.value, f.err = value, err
	if generation == s.generation {
		delete(s.flights, key)
		if err == nil && ttl > 0 {
			if len(s.entries) >= s.max {
				var oldest string
				min := ^uint64(0)
				for k, e := range s.entries {
					if e.used < min {
						oldest, min = k, e.used
					}
				}
				delete(s.entries, oldest)
			}
			s.clock++
			s.entries[key] = entry[T]{value, time.Now().Add(ttl), s.clock}
		}
	}
	close(f.done)
	return value, err
}

package cache

import (
	"context"
	"reflect"
	"sync"
	"time"
)

// Store supports memory, files, Redis or other context-aware storage. found distinguishes cached nil from a miss.
// A store used with CacheThrough must have a comparable identity (normally a pointer).
type Store[T any] interface {
	Get(context.Context, string) (T, bool, error)
	Set(context.Context, string, T, ...time.Duration) error
	Delete(context.Context, string) error
	Clear(context.Context) error
}
type MemoryStore[T any] struct{ Cache *MemoryCache[T] }

func NewMemoryStore[T any](o ...Options) (*MemoryStore[T], error) {
	c, e := New[T](o...)
	if e != nil {
		return nil, e
	}
	return &MemoryStore[T]{Cache: c}, nil
}
func (s *MemoryStore[T]) Get(ctx context.Context, key string) (T, bool, error) {
	var zero T
	if e := ctx.Err(); e != nil {
		return zero, false, e
	}
	value, found := s.Cache.Get(key)
	return value, found, nil
}
func (s *MemoryStore[T]) Set(ctx context.Context, key string, value T, ttl ...time.Duration) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	return s.Cache.Set(key, value, ttl...)
}
func (s *MemoryStore[T]) Delete(ctx context.Context, key string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	s.Cache.Delete(key)
	return nil
}
func (s *MemoryStore[T]) Clear(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	s.Cache.Clear()
	return nil
}

type ThroughOptions[T any] struct {
	TTL         *time.Duration
	ShouldCache func(T) bool
}
type flightKey struct {
	store any
	key   string
}

// Only in-flight entries retain stores; completed calls leave no global store references.
var throughFlights = struct {
	sync.Mutex
	entries map[flightKey]any
}{entries: map[flightKey]any{}}

func CacheThrough[T any](ctx context.Context, store Store[T], key string, fetch func(context.Context) (T, error), ttl ...time.Duration) (T, error) {
	var zero T
	duration, e := ttlValue(ttl, 0)
	if e != nil {
		return zero, e
	}
	o := ThroughOptions[T]{}
	if len(ttl) > 0 {
		o.TTL = &duration
	}
	return CacheThroughWithOptions(ctx, store, key, fetch, o)
}
func CacheThroughWithOptions[T any](ctx context.Context, store Store[T], key string, fetch func(context.Context) (T, error), o ThroughOptions[T]) (T, error) {
	var zero T
	if e := ctx.Err(); e != nil {
		return zero, e
	}
	identity := reflect.ValueOf(store)
	if !identity.IsValid() || !identity.Comparable() || (identity.Kind() == reflect.Pointer && identity.IsNil()) || fetch == nil {
		return zero, invalid("store must have a non-nil comparable identity and fetch must not be nil")
	}
	if o.TTL != nil && *o.TTL < 0 {
		return zero, invalid("TTL must be nonnegative")
	}
	if memory, ok := store.(*MemoryStore[T]); ok {
		if memory.Cache == nil {
			return zero, invalid("memory store must be constructed with NewMemoryStore")
		}
		return memory.Cache.load(ctx, key, fetch, o)
	}
	found := false
	value, e := fetchSafely(ctx, func(ctx context.Context) (T, error) { v, ok, e := store.Get(ctx, key); found = ok; return v, e })
	if e != nil || found {
		return value, e
	}
	k := flightKey{store: store, key: key}
	throughFlights.Lock()
	if pending, ok := throughFlights.entries[k]; ok {
		f := pending.(*flight[T])
		throughFlights.Unlock()
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-f.done:
			return f.value, f.err
		}
	}
	f := &flight[T]{done: make(chan struct{})}
	throughFlights.entries[k] = f
	throughFlights.Unlock()
	// All paths, including a custom store panic, complete the shared flight.
	value, e = fetchSafely(ctx, func(ctx context.Context) (T, error) {
		if v, ok, e := store.Get(ctx, key); e != nil || ok {
			return v, e
		}
		v, e := fetch(ctx)
		if e != nil {
			return v, e
		}
		if e = ctx.Err(); e != nil {
			return v, e
		}
		if o.ShouldCache == nil || o.ShouldCache(v) {
			ttl := []time.Duration{}
			if o.TTL != nil {
				ttl = append(ttl, *o.TTL)
			}
			e = store.Set(ctx, key, v, ttl...)
		}
		return v, e
	})
	throughFlights.Lock()
	f.value, f.err = value, e
	delete(throughFlights.entries, k)
	close(f.done)
	throughFlights.Unlock()
	return value, e
}

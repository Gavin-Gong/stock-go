// Package cache provides bounded TTL/LRU caches and context-aware injectable stores.
package cache

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type Options struct {
	DefaultTTL time.Duration
	MaxSize    int
}
type UpdateOptions struct {
	DefaultTTL *time.Duration
	MaxSize    *int
}
type entry[T any] struct {
	key     string
	value   T
	expires time.Time
}
type flight[T any] struct {
	done  chan struct{}
	value T
	err   error
}

// MemoryCache must be created with New. Values retain their identity and are not deep-copied.
type MemoryCache[T any] struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	flights map[string]*flight[T]
	options Options
	now     func() time.Time
}

func invalid(message string) error {
	return &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: message}
}
func options(values []Options) (Options, error) {
	if len(values) > 1 {
		return Options{}, invalid("at most one cache Options is allowed")
	}
	o := Options{}
	if len(values) == 1 {
		o = values[0]
	}
	if o.MaxSize == 0 {
		o.MaxSize = 1000
	}
	if o.MaxSize < 1 || o.DefaultTTL < 0 {
		return o, invalid("cache size must be positive and TTL nonnegative")
	}
	return o, nil
}
func New[T any](values ...Options) (*MemoryCache[T], error) {
	o, e := options(values)
	if e != nil {
		return nil, e
	}
	return &MemoryCache[T]{entries: map[string]*list.Element{}, order: list.New(), flights: map[string]*flight[T]{}, options: o, now: time.Now}, nil
}
func (c *MemoryCache[T]) get(key string) (T, bool) {
	var zero T
	e := c.entries[key]
	if e == nil {
		return zero, false
	}
	item := e.Value.(entry[T])
	if !item.expires.IsZero() && c.now().After(item.expires) {
		delete(c.entries, key)
		c.order.Remove(e)
		return zero, false
	}
	c.order.MoveToBack(e)
	return item.value, true
}
func (c *MemoryCache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.get(key)
}
func (c *MemoryCache[T]) Has(key string) bool { _, ok := c.Get(key); return ok }
func ttlValue(values []time.Duration, defaultTTL time.Duration) (time.Duration, error) {
	if len(values) > 1 {
		return 0, invalid("at most one TTL is allowed")
	}
	ttl := defaultTTL
	if len(values) == 1 {
		ttl = values[0]
	}
	if ttl < 0 {
		return 0, invalid("TTL must be nonnegative")
	}
	return ttl, nil
}
func (c *MemoryCache[T]) set(key string, value T, ttl time.Duration) {
	if old := c.entries[key]; old != nil {
		c.order.Remove(old)
	}
	item := entry[T]{key: key, value: value}
	if ttl > 0 {
		item.expires = c.now().Add(ttl)
	}
	c.entries[key] = c.order.PushBack(item)
	for c.order.Len() > c.options.MaxSize {
		e := c.order.Front()
		delete(c.entries, e.Value.(entry[T]).key)
		c.order.Remove(e)
	}
}
func (c *MemoryCache[T]) Set(key string, value T, ttl ...time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	duration, e := ttlValue(ttl, c.options.DefaultTTL)
	if e != nil {
		return e
	}
	delete(c.flights, key)
	c.set(key, value, duration)
	return nil
}
func (c *MemoryCache[T]) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.flights, key)
	e := c.entries[key]
	if e == nil {
		return false
	}
	c.order.Remove(e)
	delete(c.entries, key)
	return true
}
func (c *MemoryCache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*list.Element{}
	c.flights = map[string]*flight[T]{}
	c.order.Init()
}

// Len includes expired entries until Get or Cleanup removes them, matching the source cache.
func (c *MemoryCache[T]) Len() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.entries) }
func (c *MemoryCache[T]) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for key, e := range c.entries {
		item := e.Value.(entry[T])
		if !item.expires.IsZero() && c.now().After(item.expires) {
			delete(c.entries, key)
			c.order.Remove(e)
			n++
		}
	}
	return n
}
func (c *MemoryCache[T]) Options() Options { c.mu.Lock(); defer c.mu.Unlock(); return c.options }
func (c *MemoryCache[T]) MatchesOptions(o UpdateOptions) bool {
	current := c.Options()
	return (o.DefaultTTL == nil || *o.DefaultTTL == current.DefaultTTL) && (o.MaxSize == nil || *o.MaxSize == current.MaxSize)
}
func (c *MemoryCache[T]) Configure(o UpdateOptions) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if o.DefaultTTL != nil && *o.DefaultTTL < 0 || o.MaxSize != nil && *o.MaxSize < 1 {
		return invalid("cache size must be positive and TTL nonnegative")
	}
	if o.DefaultTTL != nil {
		c.options.DefaultTTL = *o.DefaultTTL
	}
	if o.MaxSize != nil {
		c.options.MaxSize = *o.MaxSize
	}
	for c.order.Len() > c.options.MaxSize {
		e := c.order.Front()
		delete(c.entries, e.Value.(entry[T]).key)
		c.order.Remove(e)
	}
	return nil
}
func fetchSafely[T any](ctx context.Context, fetch func(context.Context) (T, error)) (value T, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("cache fetch panicked: %v", p)
		}
	}()
	return fetch(ctx)
}

// GetOrFetch shares in-flight work. The initiating context owns fetch; waiters may cancel independently.
// Clear/Delete/Set invalidate pending writes without stranding existing waiters.
func (c *MemoryCache[T]) GetOrFetch(ctx context.Context, key string, fetch func(context.Context) (T, error), ttl ...time.Duration) (T, error) {
	o := ThroughOptions[T]{}
	duration, err := ttlValue(ttl, 0)
	if err != nil {
		var zero T
		return zero, err
	}
	if len(ttl) > 0 {
		o.TTL = &duration
	}
	return c.load(ctx, key, fetch, o)
}
func (c *MemoryCache[T]) load(ctx context.Context, key string, fetch func(context.Context) (T, error), o ThroughOptions[T]) (T, error) {
	var zero T
	if e := ctx.Err(); e != nil {
		return zero, e
	}
	if fetch == nil {
		return zero, invalid("fetch must not be nil")
	}
	c.mu.Lock()
	ttls := []time.Duration{}
	if o.TTL != nil {
		ttls = append(ttls, *o.TTL)
	}
	duration, e := ttlValue(ttls, c.options.DefaultTTL)
	if e != nil {
		c.mu.Unlock()
		return zero, e
	}
	if value, ok := c.get(key); ok {
		c.mu.Unlock()
		return value, nil
	}
	if pending := c.flights[key]; pending != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-pending.done:
			return pending.value, pending.err
		}
	}
	pending := &flight[T]{done: make(chan struct{})}
	c.flights[key] = pending
	c.mu.Unlock()
	shouldCache := true
	value, e := fetchSafely(ctx, func(ctx context.Context) (T, error) {
		v, e := fetch(ctx)
		if e == nil && o.ShouldCache != nil {
			shouldCache = o.ShouldCache(v)
		}
		return v, e
	})
	if e == nil {
		e = ctx.Err()
	}
	c.mu.Lock()
	pending.value, pending.err = value, e
	if c.flights[key] == pending {
		delete(c.flights, key)
		if e == nil && shouldCache {
			c.set(key, value, duration)
		}
	}
	close(pending.done)
	c.mu.Unlock()
	return value, e
}

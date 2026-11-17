package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func mustCache[T any](t *testing.T, o ...Options) *MemoryCache[T] {
	t.Helper()
	c, e := New[T](o...)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestUpstreamCacheTrace(t *testing.T) {
	body, e := os.ReadFile("../testdata/cache-parity.json")
	if e != nil {
		t.Fatal(e)
	}
	var trace []struct {
		Op, Key  string
		Value    any
		TTL      *int64
		MS, Now  int64
		MaxSize  int
		Expected json.RawMessage
	}
	if e = json.Unmarshal(body, &trace); e != nil {
		t.Fatal(e)
	}
	c := mustCache[any](t, Options{DefaultTTL: 10 * time.Millisecond, MaxSize: 2})
	now := time.UnixMilli(100000)
	c.now = func() time.Time { return now }
	for i, a := range trace {
		now = time.UnixMilli(a.Now)
		var got any
		var err error
		switch a.Op {
		case "set":
			var ttl []time.Duration
			if a.TTL != nil {
				ttl = []time.Duration{time.Duration(*a.TTL) * time.Millisecond}
			}
			err = c.Set(a.Key, a.Value, ttl...)
		case "get":
			v, ok := c.Get(a.Key)
			got = map[string]any{"found": ok, "value": v}
		case "advance":
		case "configure":
			err = c.Configure(UpdateOptions{MaxSize: &a.MaxSize})
		case "size":
			got = c.Len()
		case "cleanup":
			got = c.Cleanup()
		case "delete":
			got = c.Delete(a.Key)
		case "clear":
			c.Clear()
		default:
			t.Fatal(a.Op)
		}
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(got)
		var x, y any
		json.Unmarshal(raw, &x)
		json.Unmarshal(a.Expected, &y)
		if !reflect.DeepEqual(x, y) {
			t.Fatalf("step %d %s: got %s want %s", i, a.Op, raw, a.Expected)
		}
	}
	t.Logf("%d upstream cache operations passed", len(trace))
}
func TestCacheTTLBoundaryAndConfiguration(t *testing.T) {
	c := mustCache[int](t, Options{DefaultTTL: 10 * time.Millisecond, MaxSize: 2})
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	c.Set("a", 1)
	now = now.Add(10 * time.Millisecond)
	if !c.Has("a") {
		t.Fatal("expiry boundary is inclusive")
	}
	now = now.Add(time.Nanosecond)
	if c.Has("a") {
		t.Fatal("expired")
	}
	c.Set("old", 2)
	if e := c.Configure(UpdateOptions{DefaultTTL: ptr(0 * time.Second)}); e != nil {
		t.Fatal(e)
	}
	c.Set("forever", 3)
	now = now.Add(time.Second)
	if c.Len() != 2 || c.Cleanup() != 1 || !c.Has("forever") {
		t.Fatal("existing expiry was rewritten")
	}
	if !c.MatchesOptions(UpdateOptions{DefaultTTL: ptr(0 * time.Second), MaxSize: ptr(2)}) || c.MatchesOptions(UpdateOptions{MaxSize: ptr(3)}) {
		t.Fatal("options mismatch")
	}
	c.Set("new", 4)
	c.Get("forever")
	c.Configure(UpdateOptions{MaxSize: ptr(1)})
	if !c.Has("forever") || c.Has("new") {
		t.Fatal("LRU shrink")
	}
	c.Set("forever", 5)
	if v, _ := c.Get("forever"); v != 5 || c.Len() != 1 {
		t.Fatal("overwrite")
	}
	for _, o := range []Options{{MaxSize: -1}, {DefaultTTL: -1}} {
		if _, e := New[int](o); e == nil {
			t.Fatal("invalid accepted")
		}
	}
	if _, e := New[int](Options{}, Options{}); e == nil {
		t.Fatal("extra options")
	}
	for _, o := range []UpdateOptions{{MaxSize: ptr(0)}, {DefaultTTL: ptr(-time.Second)}} {
		if e := c.Configure(o); e == nil {
			t.Fatal("bad config")
		}
	}
	if e := c.Set("bad", 1, -1); e == nil {
		t.Fatal("negative ttl")
	}
	if e := c.Set("bad", 1, 0, 0); e == nil {
		t.Fatal("extra ttl")
	}
	if _, e := c.GetOrFetch(context.Background(), "bad", func(context.Context) (int, error) { return 0, nil }, -1); e == nil {
		t.Fatal("negative fetch ttl")
	}
	if _, e := c.GetOrFetch(context.Background(), "bad", nil); e == nil {
		t.Fatal("nil fetch")
	}
	if _, e := c.GetOrFetch(context.Background(), "bad", func(context.Context) (int, error) { return 0, nil }, 0, 0); e == nil {
		t.Fatal("extra fetch ttl")
	}
}
func TestCacheSingleflightAndInvalidation(t *testing.T) {
	for _, action := range []string{"clear", "delete", "set", "none"} {
		t.Run(action, func(t *testing.T) {
			c := mustCache[int](t)
			started, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			fetch := func(context.Context) (int, error) { calls.Add(1); close(started); <-release; return 10, nil }
			done := make(chan error, 1)
			go func() {
				v, e := c.GetOrFetch(context.Background(), "x", fetch)
				if v != 10 {
					e = fmt.Errorf("value %d", v)
				}
				done <- e
			}()
			<-started
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, e := c.GetOrFetch(ctx, "x", fetch); !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
			// Waiter is known to be waiting by a custom context whose Done is observed.
			waiting := newObservedContext()
			waitDone := make(chan error, 1)
			go func() {
				v, e := c.GetOrFetch(waiting, "x", fetch)
				if v != 10 {
					e = fmt.Errorf("wait value %d", v)
				}
				waitDone <- e
			}()
			<-waiting.observed
			switch action {
			case "clear":
				c.Clear()
			case "delete":
				c.Delete("x")
			case "set":
				c.Set("x", 99)
			}
			close(release)
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			if e := <-waitDone; e != nil {
				t.Fatal(e)
			}
			value, ok := c.Get("x")
			if action == "none" {
				if !ok || value != 10 {
					t.Fatal(value, ok)
				}
			} else if action == "set" {
				if !ok || value != 99 {
					t.Fatal(value, ok)
				}
			} else if ok {
				t.Fatal("stale write")
			}
			if calls.Load() != 1 {
				t.Fatal(calls.Load())
			}
		})
	}
}

type observedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func newObservedContext() *observedContext {
	return &observedContext{Context: context.Background(), observed: make(chan struct{})}
}
func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}
func TestFetchFailuresCancellationAndNil(t *testing.T) {
	c := mustCache[*int](t)
	fetch := func(context.Context) (*int, error) { return nil, nil }
	if _, e := c.GetOrFetch(context.Background(), "nil", fetch); e != nil {
		t.Fatal(e)
	}
	if v, ok := c.Get("nil"); v != nil || !ok {
		t.Fatal("nil must be cacheable")
	}
	marker := errors.New("failed")
	for _, fetch := range []func(context.Context) (*int, error){func(context.Context) (*int, error) { return nil, marker }, func(context.Context) (*int, error) { panic("failed") }} {
		if _, e := c.GetOrFetch(context.Background(), "bad", fetch); e == nil {
			t.Fatal("missing error")
		}
		if c.Has("bad") {
			t.Fatal("failed cached")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, e := c.GetOrFetch(ctx, "cancel", func(context.Context) (*int, error) { cancel(); return nil, nil })
	if !errors.Is(e, context.Canceled) || c.Has("cancel") {
		t.Fatal(e)
	}
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		c.GetOrFetch(context.Background(), "wait", func(context.Context) (*int, error) { close(started); <-release; return nil, nil })
		close(done)
	}()
	<-started
	ctx, cancel = context.WithCancel(context.Background())
	wait := &observedContext{Context: ctx, observed: make(chan struct{})}
	ended := make(chan error, 1)
	go func() { _, e := c.GetOrFetch(wait, "wait", fetch); ended <- e }()
	<-wait.observed
	cancel()
	if e := <-ended; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	close(release)
	<-done
}

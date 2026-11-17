package cache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type externalStore struct {
	memory    *MemoryCache[int]
	gets      atomic.Int32
	getFailAt int32
	panicGet  bool
	setError  error
}

func (s *externalStore) Get(ctx context.Context, key string) (int, bool, error) {
	n := s.gets.Add(1)
	if n == s.getFailAt {
		if s.panicGet {
			panic("custom get panic")
		}
		return 0, false, errors.New("store read failed")
	}
	v, ok := s.memory.Get(key)
	return v, ok, nil
}
func (s *externalStore) Set(ctx context.Context, key string, v int, ttl ...time.Duration) error {
	if s.setError != nil {
		return s.setError
	}
	return s.memory.Set(key, v, ttl...)
}
func (s *externalStore) Delete(ctx context.Context, key string) error {
	s.memory.Delete(key)
	return nil
}
func (s *externalStore) Clear(ctx context.Context) error { s.memory.Clear(); return nil }
func TestInjectableStoreFlights(t *testing.T) {
	store := &externalStore{memory: mustCache[int](t)}
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	fetch := func(context.Context) (int, error) { calls.Add(1); close(started); <-release; return 42, nil }
	done := make(chan error, 1)
	go func() {
		v, e := CacheThrough(context.Background(), store, "x", fetch)
		if v != 42 {
			t.Error(v)
		}
		done <- e
	}()
	<-started
	observed := newObservedContext()
	waiting := make(chan error, 1)
	go func() {
		v, e := CacheThrough(observed, store, "x", fetch)
		if v != 42 {
			t.Error(v)
		}
		waiting <- e
	}()
	<-observed.observed
	ctx, cancel := context.WithCancel(context.Background())
	waitCtx := &observedContext{Context: ctx, observed: make(chan struct{})}
	canceled := make(chan error, 1)
	go func() { _, e := CacheThrough(waitCtx, store, "x", fetch); canceled <- e }()
	<-waitCtx.observed
	cancel()
	if e := <-canceled; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := <-waiting; e != nil {
		t.Fatal(e)
	}
	if v, e := CacheThrough(context.Background(), store, "x", fetch); e != nil || v != 42 || calls.Load() != 1 {
		t.Fatal(v, e, calls.Load())
	}
	throughFlights.Lock()
	n := len(throughFlights.entries)
	throughFlights.Unlock()
	if n != 0 {
		t.Fatal("store retained globally")
	}
}
func TestThroughPoliciesAndErrors(t *testing.T) {
	ctx := context.Background()
	fetch := func(context.Context) (int, error) { return 7, nil }
	for _, memory := range []bool{true, false} {
		var store Store[int]
		if memory {
			store, _ = NewMemoryStore[int]()
		} else {
			store = &externalStore{memory: mustCache[int](t)}
		}
		v, e := CacheThroughWithOptions(ctx, store, "skip", fetch, ThroughOptions[int]{ShouldCache: func(int) bool { return false }})
		if e != nil || v != 7 {
			t.Fatal(v, e)
		}
		_, ok, _ := store.Get(ctx, "skip")
		if ok {
			t.Fatal("predicate ignored")
		}
		if _, e = CacheThroughWithOptions(ctx, store, "panic", fetch, ThroughOptions[int]{ShouldCache: func(int) bool { panic("predicate") }}); e == nil {
			t.Fatal("predicate panic escaped")
		}
		if _, e = CacheThroughWithOptions(ctx, store, "badttl", fetch, ThroughOptions[int]{TTL: ptr(-time.Second)}); e == nil {
			t.Fatal("bad ttl")
		}
		if _, e = CacheThrough(ctx, store, "fetch-error", func(context.Context) (int, error) { return 0, errors.New("fetch") }); e == nil {
			t.Fatal("fetch error lost")
		}
		if _, e = CacheThrough(ctx, store, "fetch-panic", func(context.Context) (int, error) { panic("fetch") }); e == nil {
			t.Fatal("fetch panic")
		}
		if _, e = CacheThrough(ctx, store, "ttl", fetch, time.Minute); e != nil {
			t.Fatal(e)
		}
	}
	for _, bad := range []Store[int]{nil, (*externalStore)(nil), &MemoryStore[int]{}} {
		if _, e := CacheThrough(ctx, bad, "x", fetch); e == nil {
			t.Fatal("bad store")
		}
	}
	store := &externalStore{memory: mustCache[int](t)}
	if _, e := CacheThrough(ctx, store, "x", nil); e == nil {
		t.Fatal("nil fetch")
	}
	if _, e := CacheThrough(ctx, store, "x", fetch, -1); e == nil {
		t.Fatal("bad ttl")
	}
	if _, e := CacheThrough(ctx, store, "x", fetch, 0, 0); e == nil {
		t.Fatal("extra ttl")
	}
	for _, at := range []int32{1, 2} {
		for _, panicGet := range []bool{false, true} {
			s := &externalStore{memory: mustCache[int](t), getFailAt: at, panicGet: panicGet}
			if _, e := CacheThrough(ctx, s, "x", fetch); e == nil {
				t.Fatal("store read error lost")
			}
		}
	}
	store.setError = errors.New("write")
	if _, e := CacheThrough(ctx, store, "x", fetch); !errors.Is(e, store.setError) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := CacheThrough(canceled, store, "x", fetch); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	if _, e := CacheThrough(ctx2, store, "cancel", func(context.Context) (int, error) { cancel2(); return 8, nil }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	throughFlights.Lock()
	n := len(throughFlights.entries)
	throughFlights.Unlock()
	if n != 0 {
		t.Fatal("flight leaked")
	}
}
func TestMemoryStoreTTLAndInvalidation(t *testing.T) {
	store, e := NewMemoryStore[int](Options{DefaultTTL: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	now := time.Unix(1000, 0)
	store.Cache.now = func() time.Time { return now }
	fetch := func(context.Context) (int, error) { return 3, nil }
	CacheThrough(ctx, store, "default", fetch)
	now = now.Add(time.Second + time.Nanosecond)
	if _, ok, _ := store.Get(ctx, "default"); ok {
		t.Fatal("default TTL ignored")
	}
	if e := store.Set(ctx, "manual", 5, 0); e != nil {
		t.Fatal(e)
	}
	store.Delete(ctx, "manual")
	if _, ok, _ := store.Get(ctx, "manual"); ok {
		t.Fatal("delete")
	}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		CacheThrough(ctx, store, "pending", func(context.Context) (int, error) { close(started); <-release; return 3, nil })
		close(done)
	}()
	<-started
	store.Clear(ctx)
	close(release)
	<-done
	if _, ok, _ := store.Get(ctx, "pending"); ok {
		t.Fatal("clear allowed stale write")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, e := store.Get(ctx, "x"); e == nil {
		t.Fatal("canceled read")
	}
	if e := store.Set(ctx, "x", 1); e == nil {
		t.Fatal("canceled write")
	}
	if e := store.Delete(ctx, "x"); e == nil {
		t.Fatal("canceled delete")
	}
	if e := store.Clear(ctx); e == nil {
		t.Fatal("canceled clear")
	}
	if _, e := NewMemoryStore[int](Options{MaxSize: -1}); e == nil {
		t.Fatal("invalid options")
	}
}

type testOwner struct{ scope *Scope }

func (o *testOwner) CacheScope() *Scope { return o.scope }
func (o *testOwner) ClearCaches()       { o.scope.Clear() }
func TestScopesAndKeys(t *testing.T) {
	ClearShared()
	defer ClearShared()
	a, e := GetShared("test", Options{MaxSize: 2})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := GetShared("test", Options{MaxSize: 9})
	if a != b || a.Options().MaxSize != 2 {
		t.Fatal("first options must win")
	}
	if ok, e := ConfigureShared("test", UpdateOptions{MaxSize: ptr(3)}); !ok || e != nil {
		t.Fatal(ok, e)
	}
	a.Set("x", 1)
	ClearShared()
	b, _ = GetShared("test")
	if a != b || b.Has("x") {
		t.Fatal("shared clear")
	}
	scope := NewScope()
	if ok, e := scope.Configure("missing", UpdateOptions{}); ok || e != nil {
		t.Fatal(ok, e)
	}
	if _, e := scope.Get("bad", Options{MaxSize: -1}); e == nil {
		t.Fatal("bad config")
	}
	owner := &testOwner{scope}
	c, _ := GetClientScoped(owner, "c")
	c.Set("x", 1)
	ClearClientScoped(owner)
	d, _ := GetClientScoped(owner, "c")
	if c == d || c.Has("x") {
		t.Fatal("scoped clear must remove instance")
	}
	var nilOwner *testOwner
	if _, e := GetClientScoped(nilOwner, "x"); e == nil {
		t.Fatal("nil owner")
	}
	ClearClientScoped(nilOwner)
	ClearClientScoped(nil)
	for _, parts := range [][]any{{float64(0), float32(1.5)}, {true, false, 1, nil}, {"a:b", "c"}} {
		if CreateKey(parts...) == "" {
			t.Fatal(parts)
		}
	}
	if numberKey(1.0/float64(1)) != "1" {
		t.Fatal("number key")
	}
}

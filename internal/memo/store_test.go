package memo

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSingleFlightAndClear(t *testing.T) {
	store := New[string](2)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	fetch := func(context.Context) (string, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return "value", nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := store.Load(context.Background(), "key", time.Hour, fetch)
			if err != nil || v != "value" {
				t.Errorf("%s %v", v, err)
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	store.Clear()
	if _, err := store.Load(context.Background(), "key", time.Hour, fetch); err != nil || calls.Load() != 2 {
		t.Fatal(err, calls.Load())
	}
}
func TestClearDuringLoad(t *testing.T) {
	store := New[string](1)
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		store.Load(context.Background(), "k", time.Hour, func(context.Context) (string, error) { close(started); <-release; return "old", nil })
	}()
	<-started
	store.Clear()
	got, _ := store.Load(context.Background(), "k", time.Hour, func(context.Context) (string, error) { return "new", nil })
	if got != "new" {
		t.Fatal(got)
	}
	close(release)
	<-done
	got, _ = store.Load(context.Background(), "k", time.Hour, func(context.Context) (string, error) { t.Fatal("unexpected load"); return "", nil })
	if got != "new" {
		t.Fatal(got)
	}
}
func TestFailureExpiryEvictionAndCancel(t *testing.T) {
	store := New[int](1)
	calls := 0
	fetch := func(context.Context) (int, error) { calls++; return calls, nil }
	ctx := context.Background()
	store.Load(ctx, "a", time.Nanosecond, fetch)
	time.Sleep(time.Millisecond)
	store.Load(ctx, "a", time.Hour, fetch)
	store.Load(ctx, "b", time.Hour, fetch)
	store.Load(ctx, "a", time.Hour, fetch)
	if calls != 4 {
		t.Fatal(calls)
	}
	for i := 0; i < 2; i++ {
		_, err := store.Load(ctx, "bad", time.Hour, func(context.Context) (int, error) { return 0, errors.New("bad") })
		if err == nil {
			t.Fatal("error missing")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Load(canceled, "a", time.Hour, fetch); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		store.Load(ctx, "slow", time.Hour, func(context.Context) (int, error) { close(started); <-release; return 1, nil })
	}()
	<-started
	waiter, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	if _, err := store.Load(waiter, "slow", time.Hour, fetch); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	close(release)
	<-done
}

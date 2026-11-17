package cache_test

import (
	"context"
	"fmt"
	"github.com/Gavin-Gong/stock-go/cache"
	"time"
)

func ExampleCacheThrough() {
	store, err := cache.NewMemoryStore[string](cache.Options{DefaultTTL: time.Minute, MaxSize: 100})
	if err != nil {
		panic(err)
	}
	calls := 0
	fetch := func(context.Context) (string, error) { calls++; return "value", nil }
	for i := 0; i < 2; i++ {
		v, err := cache.CacheThrough(context.Background(), store, cache.CreateKey("quote", "600519"), fetch)
		if err != nil {
			panic(err)
		}
		fmt.Println(v)
	}
	fmt.Println("fetches:", calls)
	// Output:
	// value
	// value
	// fetches: 1
}

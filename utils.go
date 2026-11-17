package stockgo

import (
	"context"
	"fmt"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"sync"
)

// ChunkArray returns copied chunks; mutating a chunk does not change the input slice.
func ChunkArray[T any](items []T, size int) ([][]T, error) {
	if size < 1 {
		return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "chunk size must be positive"}
	}
	chunks := make([][]T, 0)
	for start := 0; start < len(items); {
		end := len(items)
		if size < end-start {
			end = start + size
		}
		chunks = append(chunks, append([]T{}, items[start:end]...))
		start = end
	}
	return chunks, nil
}

// AsyncPool runs at most concurrency tasks. By default results follow completion
// order; preserveOrder=true retains task order. First failure cancels remaining
// work, waits for running tasks to return and returns no partial result.
// Tasks must respect context cancellation for prompt shutdown.
func AsyncPool[T any](ctx context.Context, tasks []func(context.Context) (T, error), concurrency int, preserveOrder ...bool) ([]T, error) {
	if concurrency < 1 || len(preserveOrder) > 1 {
		return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "concurrency must be positive; at most one preserveOrder flag is allowed"}
	}
	for _, task := range tasks {
		if task == nil {
			return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "pool task must not be nil"}
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	ordered := len(preserveOrder) == 1 && preserveOrder[0]
	result := make([]T, 0, len(tasks))
	if ordered {
		result = make([]T, len(tasks))
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	var first error
	next := 0
	for w := 0; w < concurrency && w < len(tasks); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				if next == len(tasks) || workCtx.Err() != nil {
					mu.Unlock()
					return
				}
				i := next
				next++
				mu.Unlock()
				v, e := runPoolTask(workCtx, tasks[i])
				mu.Lock()
				if e != nil {
					if first == nil {
						first = e
						cancel()
					}
					mu.Unlock()
					return
				}
				if ordered {
					result[i] = v
				} else {
					result = append(result, v)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if first != nil {
		return nil, first
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return result, nil
}
func runPoolTask[T any](ctx context.Context, task func(context.Context) (T, error)) (v T, e error) {
	defer func() {
		if p := recover(); p != nil {
			e = fmt.Errorf("async pool task panicked: %v", p)
		}
	}()
	return task(ctx)
}

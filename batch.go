package stockgo

import (
	"context"
	"strings"
	"sync"
)

// BatchOptions limits simultaneous requests. Zero values select batchSize=500, concurrency=7.
// Progress runs serially in completion order. It must not panic or block indefinitely.
type BatchOptions struct {
	BatchSize, Concurrency int
	Market                 string
	Progress               func(completed, total int)
}
type BatchService struct {
	quotes *QuoteService
	codes  *CodeService
}

func batchOption(options []BatchOptions) (BatchOptions, error) {
	if len(options) > 1 {
		return BatchOptions{}, argument("at most one BatchOptions is allowed")
	}
	var o BatchOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.BatchSize < 0 || o.Concurrency < 0 {
		return o, argument("batch size and concurrency must be nonnegative")
	}
	if o.BatchSize == 0 {
		o.BatchSize = 500
	}
	if o.BatchSize > 500 {
		o.BatchSize = 500
	}
	if o.Concurrency == 0 {
		o.Concurrency = 7
	}
	return o, nil
}
func batchFetch[T any](ctx context.Context, codes []string, o BatchOptions, fetch func(context.Context, []string) ([]T, error)) ([]T, error) {
	if err := ctx.Err(); err != nil {
		return nil, wrapContext(err)
	}
	total := (len(codes) + o.BatchSize - 1) / o.BatchSize
	result := make([][]T, total)
	if total == 0 {
		return []T{}, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	next, completed := 0, 0
	var first error
	var wg sync.WaitGroup
	workers := min(o.Concurrency, total)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				if next == total || ctx.Err() != nil {
					mu.Unlock()
					return
				}
				index := next
				next++
				mu.Unlock()
				start := index * o.BatchSize
				rows, err := fetch(ctx, codes[start:min(start+o.BatchSize, len(codes))])
				mu.Lock()
				if err != nil {
					if first == nil {
						first = err
						cancel()
					}
					mu.Unlock()
					return
				}
				result[index] = rows
				completed++
				if o.Progress != nil {
					o.Progress(completed, total)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if first != nil {
		return nil, first
	}
	if err := ctx.Err(); err != nil {
		return nil, wrapContext(err)
	}
	out := []T{}
	for _, rows := range result {
		out = append(out, rows...)
	}
	return out, nil
}
func (s *BatchService) ByCodes(ctx context.Context, codes []string, options ...BatchOptions) ([]FullQuote, error) {
	o, err := batchOption(options)
	if err != nil {
		return nil, err
	}
	return batchFetch(ctx, codes, o, s.quotes.CN)
}
func (s *BatchService) HKByCodes(ctx context.Context, codes []string, options ...BatchOptions) ([]HKQuote, error) {
	o, err := batchOption(options)
	if err != nil {
		return nil, err
	}
	return batchFetch(ctx, codes, o, s.quotes.HK)
}
func (s *BatchService) USByCodes(ctx context.Context, codes []string, options ...BatchOptions) ([]USQuote, error) {
	o, err := batchOption(options)
	if err != nil {
		return nil, err
	}
	return batchFetch(ctx, codes, o, s.quotes.US)
}
func (s *BatchService) CN(ctx context.Context, options ...BatchOptions) ([]FullQuote, error) {
	o, err := batchOption(options)
	if err != nil {
		return nil, err
	}
	codes, err := s.codes.CN(ctx, CodeOptions{Market: o.Market})
	if err != nil {
		return nil, err
	}
	return batchFetch(ctx, codes, o, s.quotes.CN)
}
func (s *BatchService) HK(ctx context.Context, options ...BatchOptions) ([]HKQuote, error) {
	o, err := batchOption(options)
	if err != nil {
		return nil, err
	}
	if o.Market != "" {
		return nil, argument("HK batch does not accept a market filter")
	}
	codes, err := s.codes.HK(ctx)
	if err != nil {
		return nil, err
	}
	return batchFetch(ctx, codes, o, s.quotes.HK)
}
func (s *BatchService) US(ctx context.Context, options ...BatchOptions) ([]USQuote, error) {
	o, err := batchOption(options)
	if err != nil {
		return nil, err
	}
	codes, err := s.codes.US(ctx, CodeOptions{Market: o.Market, Simple: true})
	if err != nil {
		return nil, err
	}
	return batchFetch(ctx, codes, o, s.quotes.US)
}
func (s *BatchService) Raw(ctx context.Context, params string) ([]RawQuote, error) {
	if params == "" {
		return []RawQuote{}, nil
	}
	return s.quotes.raw(ctx, strings.Split(params, ","))
}

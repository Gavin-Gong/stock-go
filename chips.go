package stockgo

import (
	"context"
	"math"
	"time"

	"github.com/Gavin-Gong/stock-go/indicators"
	"github.com/Gavin-Gong/stock-go/symbols"
)

type ChipRequestOptions struct {
	Days             int
	Range            *int
	Adjust           AdjustType
	IncludeHistogram indicators.HistogramMode
	Decimals         *int
}
type ChipService struct{ kline *KlineService }

func (s *ChipService) CN(ctx context.Context, code string, options ...ChipRequestOptions) ([]indicators.ChipDistributionItem, error) {
	return s.compute(ctx, code, symbols.CN, options)
}
func (s *ChipService) HK(ctx context.Context, code string, options ...ChipRequestOptions) ([]indicators.ChipDistributionItem, error) {
	return s.compute(ctx, code, symbols.HK, options)
}
func (s *ChipService) US(ctx context.Context, code string, options ...ChipRequestOptions) ([]indicators.ChipDistributionItem, error) {
	return s.compute(ctx, code, symbols.US, options)
}
func (s *ChipService) compute(ctx context.Context, code string, market symbols.Market, options []ChipRequestOptions) ([]indicators.ChipDistributionItem, error) {
	if len(options) > 1 {
		return nil, argument("at most one ChipRequestOptions is allowed")
	}
	var o ChipRequestOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Days == 0 {
		o.Days = 90
	}
	if o.Days < 1 {
		return nil, argument("days must be positive")
	}
	period := 120
	if o.Range != nil {
		period = *o.Range
	}
	if period < 0 {
		return nil, argument("range must be nonnegative")
	}
	calc := indicators.ChipOptions{Range: &period, Tail: &o.Days, IncludeHistogram: o.IncludeHistogram, Decimals: o.Decimals}
	if _, err := indicators.CalcChipDistribution(nil, calc); err != nil {
		return nil, err
	}
	if _, _, err := adjustCode(o.Adjust); err != nil {
		return nil, err
	}
	// Copy the immutable service configuration to disable fallback without mutating other users.
	kline := *s.kline
	kline.fallback = false
	fetch := func(start string) ([]KlineBar, error) {
		return kline.history(ctx, code, market, []KlineOptions{{Period: "daily", Adjust: o.Adjust, StartDate: start}})
	}
	start := ""
	if period > 0 {
		needed := float64(o.Days) + float64(period)
		natural := math.Ceil(needed * 1.5)
		if natural > 3_000_000 {
			return nil, argument("chip lookback exceeds supported calendar range")
		}
		loc, _ := time.LoadLocation(marketZone(market))
		start = dateAdd(time.Now().In(loc).Format("2006-01-02"), -int(natural))
		if start < "1970-01-01" {
			start = "1970-01-01"
		}
	}
	bars, err := fetch(start)
	if err != nil {
		return nil, err
	}
	if period > 0 && float64(len(bars)) < float64(o.Days)+float64(period) && (len(bars) == 0 || bars[0].Date <= dateAdd(start, 30)) {
		bars, err = fetch("")
		if err != nil {
			return nil, err
		}
	}
	input := make([]indicators.ChipBar, len(bars))
	for i, b := range bars {
		input[i] = indicators.ChipBar{Date: b.Date, Open: b.Open, Close: b.Close, High: b.High, Low: b.Low, TurnoverRate: b.TurnoverRate}
	}
	return indicators.CalcChipDistribution(input, calc)
}

package stockgo

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/indicators"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"github.com/Gavin-Gong/stock-go/signals"
	"github.com/Gavin-Gong/stock-go/symbols"
)

type KlineWithIndicators struct {
	KlineBar
	indicators.Result
}
type KlineIndicatorOptions struct {
	KlineOptions
	Market      symbols.Market
	Indicators  indicators.Options
	LeadingBars int
}
type KlineSignalOptions struct {
	KlineOptions
	Market         symbols.Market
	MAFast, MASlow int
}
type KlineSignal struct {
	Type      string             `json:"type"`
	Date      string             `json:"date"`
	Timestamp int64              `json:"timestamp"`
	Close     *float64           `json:"close"`
	Detail    map[string]float64 `json:"detail,omitempty"`
}

func (s *CodeService) calendar(ctx context.Context) ([]string, error) {
	value, err := s.cache.Load(ctx, s.calendarURL, 12*time.Hour, func(ctx context.Context) ([]string, error) {
		body, err := s.client.Get(ctx, "linkdiary", s.calendarURL, nil)
		if err != nil {
			return nil, err
		}
		dates := []string{}
		for _, part := range strings.Split(string(body), ",") {
			d, err := dateISO(strings.TrimSpace(part))
			if err == nil {
				dates = append(dates, d)
			}
		}
		if len(dates) == 0 {
			return nil, &sdkerrors.Error{Code: sdkerrors.UpstreamEmpty, Message: "empty trading calendar", Provider: "linkdiary", URL: s.calendarURL}
		}
		sort.Strings(dates)
		return dates, nil
	})
	if err != nil {
		return nil, wrapContext(err)
	}
	return append([]string{}, value...), nil
}
func (s *KlineService) WithIndicators(ctx context.Context, code string, options ...KlineIndicatorOptions) ([]KlineWithIndicators, error) {
	if len(options) > 1 {
		return nil, argument("at most one KlineIndicatorOptions is allowed")
	}
	var o KlineIndicatorOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.LeadingBars < 0 {
		return nil, argument("leadingBars must be nonnegative")
	}
	h, err := historyOptions([]KlineOptions{o.KlineOptions})
	if err != nil {
		return nil, err
	}
	estimate, err := indicators.EstimateLookback(o.Indicators)
	if err != nil {
		return nil, err
	}
	market := o.Market
	if market == "" {
		ns, err := symbols.Normalize(code)
		if err != nil {
			return nil, err
		}
		market = ns.Market
	}
	if market != symbols.CN && market != symbols.HK && market != symbols.US {
		return nil, argument("no history route for market " + string(market))
	}
	fetch := s.CN
	if market == symbols.HK {
		fetch = s.HK
	} else if market == symbols.US {
		fetch = s.US
	}
	requestedStart := ""
	if o.StartDate != "" {
		requestedStart = h.StartDate
	}
	actualStart := ""
	if requestedStart != "" {
		ratio := 1.5
		if market == symbols.HK {
			ratio = 1.46
		} else if market == symbols.US {
			ratio = 1.45
		}
		step := 1
		if h.Period == "weekly" {
			ratio = 7.5
			step = 5
		} else if h.Period == "monthly" {
			ratio = 31.5
			step = 21
		}
		naturalDays := math.Ceil(float64(estimate.RequiredBars) * ratio)
		if naturalDays > 3_000_000 {
			return nil, argument("indicator lookback exceeds supported calendar range")
		}
		actualStart = dateAdd(requestedStart, -int(naturalDays))
		if actualStart < "1970-01-01" {
			actualStart = "1970-01-01"
		}
		if market == symbols.CN && estimate.RequiredBars > 0 {
			calendar, err := s.codes.calendar(ctx)
			if err == nil {
				index := sort.SearchStrings(calendar, requestedStart)
				if index == len(calendar) {
					index = len(calendar) - 1
				}
				index = max(0, index-estimate.RequiredBars*step)
				actualStart = calendar[index]
			}
			if ctx.Err() != nil {
				return nil, wrapContext(ctx.Err())
			}
		}
		h.StartDate = actualStart
	}
	bars, err := fetch(ctx, code, h)
	if err != nil {
		return nil, err
	}
	tolerance := 30
	if h.Period == "weekly" {
		tolerance += 7
	} else if h.Period == "monthly" {
		tolerance += 31
	}
	earlierPossible := len(bars) == 0 || actualStart == "" || bars[0].Date <= dateAdd(actualStart, tolerance)
	refetched := false
	if requestedStart != "" && len(bars) < estimate.RequiredBars && earlierPossible {
		refetched = true
		h.StartDate = ""
		bars, err = fetch(ctx, code, h)
		if err != nil {
			return nil, err
		}
	}
	windowIndex := 0
	if requestedStart != "" {
		windowIndex = sort.Search(len(bars), func(i int) bool { return bars[i].Date >= requestedStart })
		if windowIndex == len(bars) {
			return []KlineWithIndicators{}, nil
		}
	}
	if refetched && !estimate.HasCumulative {
		lookback := estimate.RequiredBars
		if estimate.MaxRecursiveLookback > 0 {
			lookback = max(lookback, 500, 15*estimate.MaxRecursiveLookback)
		}
		sliceStart := max(0, windowIndex-lookback)
		bars = bars[sliceStart:]
		windowIndex -= sliceStart
	}
	data := make([]indicators.OHLCV, len(bars))
	for i, b := range bars {
		data[i] = indicators.OHLCV{Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume}
	}
	results, err := indicators.Compute(data, o.Indicators)
	if err != nil {
		return nil, err
	}
	out := []KlineWithIndicators{}
	startIndex := max(0, windowIndex-o.LeadingBars)
	for i := startIndex; i < len(bars); i++ {
		if bars[i].Date <= h.EndDate {
			out = append(out, KlineWithIndicators{bars[i], results[i]})
		}
	}
	return out, nil
}
func (s *KlineService) Signals(ctx context.Context, code string, options ...KlineSignalOptions) ([]KlineSignal, error) {
	if len(options) > 1 {
		return nil, argument("at most one KlineSignalOptions is allowed")
	}
	var o KlineSignalOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.MAFast == 0 {
		o.MAFast = 5
	}
	if o.MASlow == 0 {
		o.MASlow = 20
	}
	if o.MAFast < 1 || o.MAFast >= o.MASlow {
		return nil, argument("MA fast must be positive and less than slow")
	}
	leading := 0
	if o.StartDate != "" {
		leading = 1
	}
	bars, err := s.WithIndicators(ctx, code, KlineIndicatorOptions{KlineOptions: o.KlineOptions, Market: o.Market, LeadingBars: leading, Indicators: indicators.Options{MA: &indicators.MAOptions{Periods: []int{o.MAFast, o.MASlow}}, MACD: &indicators.MACDOptions{}, KDJ: &indicators.KDJOptions{}, RSI: &indicators.RSIOptions{Periods: []int{6}}, BOLL: &indicators.BOLLOptions{}, SAR: &indicators.SAROptions{}}})
	if err != nil {
		return nil, err
	}
	local := make([]signals.Bar, len(bars))
	for i, b := range bars {
		local[i] = signals.Bar{Timestamp: b.Timestamp, Close: b.Close, Result: b.Result}
	}
	events, err := signals.CalcSignals(local, signals.Options{MA: &signals.MACross{Fast: o.MAFast, Slow: o.MASlow}, MACD: true, KDJ: &signals.Thresholds{}, RSI: &signals.RSIOptions{Period: 6}, BOLL: true, SAR: true})
	if err != nil {
		return nil, err
	}
	start := ""
	if o.StartDate != "" {
		start, err = dateISO(o.StartDate)
		if err != nil {
			return nil, err
		}
	}
	out := []KlineSignal{}
	for _, event := range events {
		b := bars[event.Index]
		if start == "" || b.Date >= start {
			out = append(out, KlineSignal{event.Type, b.Date, event.At, b.Close, event.Detail})
		}
	}
	return out, nil
}

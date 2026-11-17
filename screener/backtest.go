package screener

import (
	"math"
	"reflect"
)

type StrategySignal string

const (
	Buy  StrategySignal = "buy"
	Sell StrategySignal = "sell"
	Hold StrategySignal = "hold"
)

type Fee struct{ Buy, Sell float64 }

func FeeRate(rate float64) Fee { return Fee{rate, rate} }
func Float(v float64) *float64 { return &v }

// BacktestOptions uses explicit price/date accessors so arbitrary Go bar types can be used.
// Strategy sees the complete series; reading future bars introduces look-ahead bias.
type BacktestOptions[T any] struct {
	Klines                       []T
	Strategy                     func(bar T, index int, series []T) StrategySignal
	GetClose                     func(T) *float64
	GetDate                      func(T) any
	InitialCapital, PositionSize *float64
	Fee                          Fee
}
type Trade struct {
	EntryIndex    int     `json:"entryIndex"`
	ExitIndex     int     `json:"exitIndex"`
	EntryPrice    float64 `json:"entryPrice"`
	ExitPrice     float64 `json:"exitPrice"`
	ReturnPercent float64 `json:"returnPercent"`
	Forced        bool    `json:"forced"`
	EntryDate     any     `json:"entryDate,omitempty"`
	ExitDate      any     `json:"exitDate,omitempty"`
}
type BacktestReport struct {
	InitialCapital float64   `json:"initialCapital"`
	FinalEquity    float64   `json:"finalEquity"`
	TotalReturn    float64   `json:"totalReturn"`
	WinRate        float64   `json:"winRate"`
	MaxDrawdown    float64   `json:"maxDrawdown"`
	BuyHoldReturn  float64   `json:"buyHoldReturn"`
	ValidBars      int       `json:"validBars"`
	TradeCount     int       `json:"tradeCount"`
	Trades         []Trade   `json:"trades"`
	EquityCurve    []float64 `json:"equityCurve"`
}

func isHole[T any](value T) bool {
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return v.IsNil()
	}
	return false
}

// Backtest implements same-bar closing-price execution, fractional long-only positions,
// deferred signals on invalid prices, and liquidation at the final valid closing price.
func Backtest[T any](o BacktestOptions[T]) (*BacktestReport, error) {
	initial, size := 100000.0, 1.0
	if o.InitialCapital != nil {
		initial = *o.InitialCapital
	}
	if o.PositionSize != nil {
		size = *o.PositionSize
	}
	if !valid(&initial) || initial <= 0 {
		return nil, invalid("initial capital must be positive and finite")
	}
	if !valid(&size) || size <= 0 || size > 1 {
		return nil, invalid("position size must be in (0,1]")
	}
	for _, rate := range []float64{o.Fee.Buy, o.Fee.Sell} {
		if !valid(&rate) || rate < 0 || rate >= 1 {
			return nil, invalid("fee rates must be in [0,1)")
		}
	}
	if o.Strategy == nil || o.GetClose == nil {
		return nil, invalid("strategy and getClose are required")
	}
	cash, position, entryPrice, lastPrice, firstPrice := initial, 0.0, 0.0, 0.0, 0.0
	entryIndex, lastIndex, validBars := -1, -1, 0
	pending := Hold
	trades := []Trade{}
	curve := make([]float64, 0, len(o.Klines))
	dateOf := func(index int) any {
		if o.GetDate == nil || index < 0 || isHole(o.Klines[index]) {
			return nil
		}
		return o.GetDate(o.Klines[index])
	}
	record := func(exit int, price float64, forced bool) {
		ret := ((price/entryPrice)*(1-o.Fee.Buy)*(1-o.Fee.Sell) - 1) * 100
		trades = append(trades, Trade{entryIndex, exit, entryPrice, price, ret, forced, dateOf(entryIndex), dateOf(exit)})
	}
	for i, bar := range o.Klines {
		var price *float64
		signal := Hold
		if !isHole(bar) {
			price = o.GetClose(bar)
			signal = o.Strategy(bar, i, o.Klines)
			if signal != Buy && signal != Sell && signal != Hold {
				return nil, invalid("strategy must return buy, sell or hold")
			}
		}
		if !valid(price) || (price != nil && *price <= 0) {
			price = nil
		}
		if price != nil {
			validBars++
			lastPrice = *price
			lastIndex = i
			if firstPrice == 0 {
				firstPrice = *price
			}
			effective := pending
			if signal != Hold {
				effective = signal
			}
			pending = Hold
			if effective == Buy && position == 0 {
				invest := cash * size
				position = invest * (1 - o.Fee.Buy) / *price
				entryPrice = *price
				entryIndex = i
				cash -= invest
			} else if effective == Sell && position > 0 {
				cash += position * *price * (1 - o.Fee.Sell)
				record(i, *price, false)
				position = 0
			}
		} else if signal != Hold {
			pending = signal
		}
		mark := lastPrice
		if price != nil {
			mark = *price
		}
		equity := cash + position*mark
		if math.IsInf(equity, 0) || math.IsNaN(equity) {
			return nil, invalid("backtest equity overflow")
		}
		curve = append(curve, equity)
	}
	if position > 0 && lastPrice > 0 {
		cash += position * lastPrice * (1 - o.Fee.Sell)
		record(lastIndex, lastPrice, pending != Sell)
		for i := max(lastIndex, 0); i < len(curve); i++ {
			curve[i] = cash
		}
	}
	final := initial
	if len(curve) > 0 {
		final = curve[len(curve)-1]
	}
	wins := 0
	for _, trade := range trades {
		if trade.ReturnPercent > 0 {
			wins++
		}
	}
	winRate, buyHold := 0.0, 0.0
	if len(trades) > 0 {
		winRate = float64(wins) / float64(len(trades)) * 100
	}
	if firstPrice > 0 {
		buyHold = (lastPrice/firstPrice - 1) * 100
	}
	peak, drawdown := initial, 0.0
	for _, e := range curve {
		peak = math.Max(peak, e)
		drawdown = math.Max(drawdown, (peak-e)/peak*100)
	}
	return &BacktestReport{initial, final, (final/initial - 1) * 100, winRate, drawdown, buyHold, validBars, len(trades), trades, curve}, nil
}

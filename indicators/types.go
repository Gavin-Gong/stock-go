// Package indicators implements stock-sdk's pure local technical indicators.
// Results stay aligned with input bars; nil represents an unavailable value.
package indicators

import (
	"fmt"
	"math"
	"strconv"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type OHLCV struct {
	Open   *float64 `json:"open"`
	High   *float64 `json:"high"`
	Low    *float64 `json:"low"`
	Close  *float64 `json:"close"`
	Volume *float64 `json:"volume"`
}
type Values map[string]*float64

// Float is a convenience constructor for optional input values and options.
func Float(v float64) *float64 { return &v }
func Int(v int) *int           { return &v }
func invalid(s string) error   { return &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: s} }
func one[T any](options []T) (T, error) {
	var o T
	if len(options) > 1 {
		return o, invalid("at most one options value is allowed")
	}
	if len(options) == 1 {
		o = options[0]
	}
	return o, nil
}
func def(n, fallback int) int {
	if n == 0 {
		return fallback
	}
	return n
}
func validate(period int, decimals *int) error {
	if period < 1 || period > int(^uint(0)>>1)/64 {
		return invalid("indicator periods must be positive")
	}
	if decimals != nil && (*decimals < -15 || *decimals > 15) {
		return invalid("decimals must be in -15..15")
	}
	return nil
}
func precision(d *int) int {
	if d == nil {
		return 3
	}
	return *d
}
func rounded(v float64, d *int) *float64 {
	factor := math.Pow10(precision(d))
	// Explicit conversion prevents Go from fusing the following subtraction on ARM64.
	scaled := float64(v * factor)
	floor := math.Floor(scaled)
	if scaled-floor >= 0.5 {
		floor++
	}
	v = floor / factor
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return Float(v)
}
func finite(v *float64) bool { return v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) }
func clean(data []*float64) []*float64 {
	out := make([]*float64, len(data))
	for i, v := range data {
		if finite(v) {
			out[i] = v
		}
	}
	return out
}
func cleanBars(data []OHLCV) []OHLCV {
	out := append([]OHLCV{}, data...)
	for i := range out {
		for _, v := range []**float64{&out[i].Open, &out[i].High, &out[i].Low, &out[i].Close, &out[i].Volume} {
			if !finite(*v) {
				*v = nil
			}
		}
	}
	return out
}
func closes(data []OHLCV) []*float64 {
	out := make([]*float64, len(data))
	for i, b := range data {
		out[i] = b.Close
	}
	return out
}
func key(prefix string, period int) string { return prefix + strconv.Itoa(period) }
func maps(n int) []Values {
	out := make([]Values, n)
	for i := range out {
		out[i] = Values{}
	}
	return out
}
func validatePeriods(periods []int, d *int) error {
	if err := validate(1, d); err != nil {
		return err
	}
	for _, p := range periods {
		if err := validate(p, d); err != nil {
			return fmt.Errorf("period %d: %w", p, err)
		}
	}
	return nil
}

type MAOptions struct {
	Periods  []int
	Type     string
	Decimals *int
}

type MACDOptions struct {
	Short, Long, Signal int
	Decimals            *int
}

type BOLLOptions struct {
	Period   int
	StdDev   *float64
	Decimals *int
}

type KDJOptions struct {
	Period, KPeriod, DPeriod int
	Decimals                 *int
}

type RSIOptions struct {
	Periods  []int
	Decimals *int
}

type WROptions struct {
	Periods  []int
	Decimals *int
}

type BIASOptions struct {
	Periods  []int
	Decimals *int
}

type CCIOptions struct {
	Period   int
	Decimals *int
}

type ATROptions struct {
	Period   int
	Decimals *int
}

type OBVOptions struct{ MAPeriod int }

type ROCOptions struct{ Period, SignalPeriod int }

type DMIOptions struct{ Period, ADXPeriod int }

type SAROptions struct{ AFStart, AFIncrement, AFMax *float64 }

type KCOptions struct {
	EMAPeriod, ATRPeriod int
	Multiplier           *float64
}

type MACDResult struct {
	DIF  *float64 `json:"dif"`
	DEA  *float64 `json:"dea"`
	MACD *float64 `json:"macd"`
}

type BOLLResult struct {
	Mid       *float64 `json:"mid"`
	Upper     *float64 `json:"upper"`
	Lower     *float64 `json:"lower"`
	Bandwidth *float64 `json:"bandwidth"`
}

type KDJResult struct {
	K *float64 `json:"k"`
	D *float64 `json:"d"`
	J *float64 `json:"j"`
}

type CCIResult struct {
	CCI *float64 `json:"cci"`
}

type ATRResult struct {
	TR  *float64 `json:"tr"`
	ATR *float64 `json:"atr"`
}

type OBVResult struct {
	OBV   *float64 `json:"obv"`
	OBVMA *float64 `json:"obvMa"`
}

type ROCResult struct {
	ROC    *float64 `json:"roc"`
	Signal *float64 `json:"signal"`
}

type DMIResult struct {
	PDI  *float64 `json:"pdi"`
	MDI  *float64 `json:"mdi"`
	ADX  *float64 `json:"adx"`
	ADXR *float64 `json:"adxr"`
}

type SARResult struct {
	SAR   *float64 `json:"sar"`
	Trend *int     `json:"trend"`
	EP    *float64 `json:"ep"`
	AF    *float64 `json:"af"`
}

type KCResult struct {
	Mid   *float64 `json:"mid"`
	Upper *float64 `json:"upper"`
	Lower *float64 `json:"lower"`
	Width *float64 `json:"width"`
}

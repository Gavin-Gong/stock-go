package indicators

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// ChipBar is a daily bar with turnover rate expressed as a percentage.
type ChipBar struct {
	Date         string   `json:"date"`
	Open         *float64 `json:"open"`
	Close        *float64 `json:"close"`
	High         *float64 `json:"high"`
	Low          *float64 `json:"low"`
	TurnoverRate *float64 `json:"turnoverRate"`
}
type HistogramMode string

const (
	HistogramLast HistogramMode = "last"
	HistogramAll  HistogramMode = "all"
)

type ChipOptions struct {
	Range            *int          `json:"range"`
	Tail             *int          `json:"tail"`
	IncludeHistogram HistogramMode `json:"includeHistogram"`
	Decimals         *int          `json:"decimals"`
}
type ChipHistogram struct {
	Prices []float64 `json:"prices"`
	Ratios []float64 `json:"ratios"`
}
type ChipDistributionItem struct {
	Date            string         `json:"date"`
	ProfitRatio     *float64       `json:"profitRatio"`
	AvgCost         *float64       `json:"avgCost"`
	Cost90Low       *float64       `json:"cost90Low"`
	Cost90High      *float64       `json:"cost90High"`
	Concentration90 *float64       `json:"concentration90"`
	Cost70Low       *float64       `json:"cost70Low"`
	Cost70High      *float64       `json:"cost70High"`
	Concentration70 *float64       `json:"concentration70"`
	Histogram       *ChipHistogram `json:"histogram,omitempty"`
}

// decimalRound implements JS decimal formatting's half-away-from-zero rule using
// the exact binary input, followed by correctly rounded conversion back to float64.
func decimalRound(v float64, places int) float64 {
	r := new(big.Rat).SetFloat64(v)
	if r == nil {
		return v
	}
	negative := r.Sign() < 0
	r.Abs(r)
	exponent := places
	if exponent < 0 {
		exponent = -exponent
	}
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
	scale := new(big.Rat).SetInt(factor)
	if places >= 0 {
		r.Mul(r, scale)
	} else {
		r.Quo(r, scale)
	}
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(r.Num(), r.Denom(), remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(r.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	r.SetInt(quotient)
	if places >= 0 {
		r.Quo(r, scale)
	} else {
		r.Mul(r, scale)
	}
	if negative {
		r.Neg(r)
	}
	out, _ := r.Float64()
	return out
}

// jsPrice follows Number(value.toFixed(2)), including binary-representation half ties.
func jsPrice(v float64) float64 {
	if math.Abs(v) >= 1e21 {
		return v
	}
	return decimalRound(v, 2)
}
func prec12(v float64) float64 {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	scientific := strconv.FormatFloat(math.Abs(v), 'e', -1, 64)
	index := strings.LastIndexByte(scientific, 'e')
	exponent, _ := strconv.Atoi(scientific[index+1:])
	return decimalRound(v, 11-exponent)
}
func validChipBar(b ChipBar) bool {
	return finite(b.Open) && finite(b.Close) && finite(b.High) && finite(b.Low)
}

// CalcChipDistribution estimates daily circulating cost distribution using 150 price buckets.
// Range defaults to 120; an explicit zero accumulates from the first bar. Tail limits outputs.
func CalcChipDistribution(bars []ChipBar, options ...ChipOptions) ([]ChipDistributionItem, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	period := 120
	if o.Range != nil {
		period = *o.Range
	}
	if period < 0 {
		return nil, invalid("chip range must be nonnegative")
	}
	if err := validate(1, o.Decimals); err != nil {
		return nil, err
	}
	if o.IncludeHistogram != "" && o.IncludeHistogram != HistogramLast && o.IncludeHistogram != HistogramAll {
		return nil, invalid("histogram must be empty, last or all")
	}
	start := 0
	if o.Tail != nil {
		start = len(bars) - min(len(bars), max(0, *o.Tail))
	}
	out := make([]ChipDistributionItem, 0, len(bars)-start)
	for i := start; i < len(bars); i++ {
		histogram := o.IncludeHistogram == HistogramAll || (o.IncludeHistogram == HistogramLast && i == len(bars)-1)
		out = append(out, chipAt(bars, i, period, o.Decimals, histogram))
	}
	return out, nil
}
func chipAt(bars []ChipBar, index, period int, decimals *int, histogram bool) ChipDistributionItem {
	item := ChipDistributionItem{Date: bars[index].Date}
	start := 0
	if period > 0 {
		start = max(0, index-period+1)
	}
	window := bars[start : index+1]
	high, low := math.Inf(-1), math.Inf(1)
	valid := false
	for _, b := range window {
		if !validChipBar(b) {
			continue
		}
		valid = true
		high = math.Max(high, *b.High)
		low = math.Min(low, *b.Low)
	}
	if !valid {
		return item
	}
	accuracy := math.Max(0.01, (high-low)/149)
	if math.IsInf(accuracy, 0) {
		return item
	}
	var chips [150]float64
	bucket := func(v float64) int {
		if math.IsNaN(v) || v <= 0 {
			return 0
		}
		if v >= 149 {
			return 149
		}
		return int(v)
	}
	for _, b := range window {
		if !validChipBar(b) {
			continue
		}
		open, close, h, l := *b.Open, *b.Close, *b.High, *b.Low
		avg := (open + close + h + l) / 4
		turnover := 0.0
		if finite(b.TurnoverRate) {
			turnover = math.Max(0, math.Min(1, *b.TurnoverRate/100))
		}
		hi, lo := bucket(math.Floor((h-low)/accuracy)), bucket(math.Ceil((l-low)/accuracy))
		gIndex := bucket(math.Floor((avg - low) / accuracy))
		factor := 149.0
		if h != l {
			factor = 2 / (h - l)
		}
		for i := range chips {
			chips[i] *= 1 - turnover
		}
		if h == l {
			chips[gIndex] += float64(factor*turnover) / 2
			continue
		}
		for j := lo; j <= hi; j++ {
			price := low + float64(accuracy*float64(j))
			addition := factor * turnover
			if price <= avg {
				if math.Abs(avg-l) >= 1e-8 {
					addition = float64(float64((price-l)/(avg-l))*factor) * turnover
				}
			} else {
				if math.Abs(h-avg) >= 1e-8 {
					addition = float64(float64((h-price)/(h-avg))*factor) * turnover
				}
			}
			chips[j] += addition
		}
	}
	total := 0.0
	for i := range chips {
		chips[i] = prec12(chips[i])
		total += chips[i]
	}
	if total == 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return item
	}
	cost := func(target float64) float64 {
		sum := 0.0
		for i, x := range chips {
			if sum+x > target {
				return low + float64(float64(i)*accuracy)
			}
			sum += x
		}
		return 0
	}
	percentage := func(percent float64) (float64, float64, float64) {
		a, b := cost(total*((1-percent)/2)), cost(total*((1+percent)/2))
		concentration := 0.0
		if a+b != 0 {
			concentration = (b - a) / (a + b)
		}
		return jsPrice(a), jsPrice(b), concentration
	}
	lo90, hi90, c90 := percentage(0.9)
	lo70, hi70, c70 := percentage(0.7)
	item.AvgCost = Float(jsPrice(cost(total * 0.5)))
	item.Cost90Low = Float(lo90)
	item.Cost90High = Float(hi90)
	item.Concentration90 = rounded(c90, decimals)
	item.Cost70Low = Float(lo70)
	item.Cost70High = Float(hi70)
	item.Concentration70 = rounded(c70, decimals)
	if finite(bars[index].Close) {
		below := 0.0
		for i, x := range chips {
			if *bars[index].Close >= low+float64(float64(i)*accuracy) {
				below += x
			}
		}
		item.ProfitRatio = rounded(below/total, decimals)
	}
	if histogram {
		h := &ChipHistogram{Prices: make([]float64, 150), Ratios: make([]float64, 150)}
		six := 6
		for i, x := range chips {
			h.Prices[i] = jsPrice(low + float64(accuracy*float64(i)))
			if value := rounded(x/total, &six); value != nil {
				h.Ratios[i] = *value
			}
		}
		item.Histogram = h
	}
	return item
}

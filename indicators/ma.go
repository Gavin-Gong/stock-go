package indicators

// windowSum uses Kahan compensation, matching the source's rolling arithmetic.
type windowSum struct {
	data      []*float64
	period    int
	sum, comp float64
	count     int
}

func (w *windowSum) add(v float64) {
	y := v - w.comp
	t := w.sum + y
	w.comp = (t - w.sum) - y
	w.sum = t
}
func (w *windowSum) advance(i int) {
	if w.data[i] != nil {
		w.add(*w.data[i])
		w.count++
	}
	if i >= w.period && w.data[i-w.period] != nil {
		w.add(-*w.data[i-w.period])
		w.count--
	}
}
func moving(data []*float64, period int, kind string, d *int) []*float64 {
	out := make([]*float64, len(data))
	data = clean(data)
	if kind == "sma" {
		w := windowSum{data: data, period: period}
		for i := range data {
			w.advance(i)
			if i >= period-1 && w.count == period {
				out[i] = rounded(w.sum/float64(period), d)
			}
		}
		return out
	}
	if kind == "wma" {
		for i := period - 1; i < len(data); i++ {
			sum := 0.0
			valid := true
			for j := 0; j < period; j++ {
				v := data[i-period+1+j]
				if v == nil {
					valid = false
					break
				}
				sum += float64(*v * float64(j+1))
			}
			if valid {
				out[i] = rounded(sum/(float64(period)*(float64(period)+1)/2), d)
			}
		}
		return out
	}
	alpha := 2 / (float64(period) + 1)
	initialized := false
	ema := 0.0
	for i := range data {
		if i < period-1 {
			continue
		}
		if !initialized {
			sum, count := 0.0, 0
			for j := i - period + 1; j <= i; j++ {
				if data[j] != nil {
					sum += *data[j]
					count++
				}
			}
			if count == period {
				ema = sum / float64(period)
				initialized = true
			}
		} else if data[i] != nil {
			ema = float64(alpha**data[i]) + float64((1-alpha)*ema)
		}
		if initialized {
			out[i] = rounded(ema, d)
		}
	}
	return out
}
func singleMA(data []*float64, period int, kind string, decimals []int) ([]*float64, error) {
	if len(decimals) > 1 {
		return nil, invalid("at most one decimals value is allowed")
	}
	var d *int
	if len(decimals) == 1 {
		d = &decimals[0]
	}
	if err := validate(period, d); err != nil {
		return nil, err
	}
	return moving(data, period, kind, d), nil
}
func CalcSMA(data []*float64, period int, decimals ...int) ([]*float64, error) {
	return singleMA(data, period, "sma", decimals)
}
func CalcEMA(data []*float64, period int, decimals ...int) ([]*float64, error) {
	return singleMA(data, period, "ema", decimals)
}
func CalcWMA(data []*float64, period int, decimals ...int) ([]*float64, error) {
	return singleMA(data, period, "wma", decimals)
}
func CalcMA(data []*float64, options ...MAOptions) ([]Values, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	if o.Periods == nil {
		o.Periods = []int{5, 10, 20, 30, 60, 120, 250}
	}
	if o.Type == "" {
		o.Type = "sma"
	}
	if o.Type != "sma" && o.Type != "ema" && o.Type != "wma" {
		return nil, invalid("MA type must be sma, ema or wma")
	}
	if err := validatePeriods(o.Periods, o.Decimals); err != nil {
		return nil, err
	}
	out := maps(len(data))
	for _, p := range o.Periods {
		values := moving(data, p, o.Type, o.Decimals)
		for i := range data {
			out[i][key("ma", p)] = values[i]
		}
	}
	return out, nil
}

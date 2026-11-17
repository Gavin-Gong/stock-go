package indicators

import "math"

func CalcRSI(data []*float64, options ...RSIOptions) ([]Values, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	if o.Periods == nil {
		o.Periods = []int{6, 12, 24}
	}
	if err := validatePeriods(o.Periods, o.Decimals); err != nil {
		return nil, err
	}
	data = clean(data)
	changes := make([]*float64, len(data))
	for i := 1; i < len(data); i++ {
		if data[i] != nil && data[i-1] != nil {
			changes[i] = Float(*data[i] - *data[i-1])
		}
	}
	out := maps(len(data))
	for _, period := range o.Periods {
		gain, loss := 0.0, 0.0
		p := float64(period)
		for i := range data {
			out[i][key("rsi", period)] = nil
			c := 0.0
			if changes[i] != nil {
				c = *changes[i]
			}
			if i <= period {
				if c > 0 {
					gain += c
				} else {
					loss += math.Abs(c)
				}
				if i < period {
					continue
				}
				gain /= p
				loss /= p
			} else {
				gain = (gain*(p-1) + math.Max(c, 0)) / p
				loss = (loss*(p-1) + math.Max(-c, 0)) / p
			}
			v := 100.0
			if loss != 0 {
				if gain == 0 {
					v = 0
				} else {
					v = 100 - 100/(1+gain/loss)
				}
			}
			out[i][key("rsi", period)] = rounded(v, o.Decimals)
		}
	}
	return out, nil
}
func CalcWR(data []OHLCV, options ...WROptions) ([]Values, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	if o.Periods == nil {
		o.Periods = []int{6, 10}
	}
	if err := validatePeriods(o.Periods, o.Decimals); err != nil {
		return nil, err
	}
	data = cleanBars(data)
	out := maps(len(data))
	for _, period := range o.Periods {
		for i, b := range data {
			k := key("wr", period)
			out[i][k] = nil
			if i < period-1 || b.Close == nil {
				continue
			}
			high, low, valid := math.Inf(-1), math.Inf(1), true
			for j := i - period + 1; j <= i; j++ {
				if data[j].High == nil || data[j].Low == nil {
					valid = false
					break
				}
				high = math.Max(high, *data[j].High)
				low = math.Min(low, *data[j].Low)
			}
			if valid && high != low {
				out[i][k] = rounded((high-*b.Close)/(high-low)*100, o.Decimals)
			}
		}
	}
	return out, nil
}
func CalcBIAS(data []*float64, options ...BIASOptions) ([]Values, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	if o.Periods == nil {
		o.Periods = []int{6, 12, 24}
	}
	if err := validatePeriods(o.Periods, o.Decimals); err != nil {
		return nil, err
	}
	data = clean(data)
	out := maps(len(data))
	for _, p := range o.Periods {
		ma := moving(data, p, "sma", o.Decimals)
		for i, v := range data {
			k := key("bias", p)
			out[i][k] = nil
			if v != nil && ma[i] != nil && *ma[i] != 0 {
				out[i][k] = rounded((*v-*ma[i]) / *ma[i] * 100, o.Decimals)
			}
		}
	}
	return out, nil
}
func CalcCCI(data []OHLCV, options ...CCIOptions) ([]CCIResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	o.Period = def(o.Period, 14)
	if err := validate(o.Period, o.Decimals); err != nil {
		return nil, err
	}
	data = cleanBars(data)
	tp := make([]*float64, len(data))
	for i, b := range data {
		if b.High != nil && b.Low != nil && b.Close != nil {
			tp[i] = Float((*b.High + *b.Low + *b.Close) / 3)
		}
	}
	out := make([]CCIResult, len(data))
	for i := o.Period - 1; i < len(data); i++ {
		sum, count := 0.0, 0
		for j := i - o.Period + 1; j <= i; j++ {
			if tp[j] != nil {
				sum += *tp[j]
				count++
			}
		}
		if count != o.Period || tp[i] == nil {
			continue
		}
		ma := sum / float64(o.Period)
		md := 0.0
		for j := i - o.Period + 1; j <= i; j++ {
			md += math.Abs(*tp[j] - ma)
		}
		md /= float64(o.Period)
		v := 0.0
		if md != 0 {
			v = (*tp[i] - ma) / (0.015 * md)
		}
		out[i].CCI = rounded(v, o.Decimals)
	}
	return out, nil
}
func CalcOBV(data []OHLCV, options ...OBVOptions) ([]OBVResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	if o.MAPeriod < 0 || o.MAPeriod > int(^uint(0)>>1)/64 {
		return nil, invalid("OBV maPeriod must be nonnegative")
	}
	data = cleanBars(data)
	out := make([]OBVResult, len(data))
	if len(data) == 0 {
		return out, nil
	}
	value := 0.0
	if data[0].Volume != nil {
		value = *data[0].Volume
	}
	out[0].OBV = Float(value)
	for i := 1; i < len(data); i++ {
		cur, prev := data[i], data[i-1]
		if cur.Close == nil || prev.Close == nil || cur.Volume == nil {
			continue
		}
		if *cur.Close > *prev.Close {
			value += *cur.Volume
		} else if *cur.Close < *prev.Close {
			value -= *cur.Volume
		}
		out[i].OBV = Float(value)
	}
	if o.MAPeriod > 0 {
		series := make([]*float64, len(data))
		for i := range out {
			series[i] = out[i].OBV
		}
		w := windowSum{data: series, period: o.MAPeriod}
		for i := range out {
			w.advance(i)
			if i >= o.MAPeriod-1 && w.count == o.MAPeriod {
				out[i].OBVMA = Float(w.sum / float64(o.MAPeriod))
			}
		}
	}
	return out, nil
}
func CalcROC(data []OHLCV, options ...ROCOptions) ([]ROCResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	o.Period = def(o.Period, 12)
	if err := validate(o.Period, nil); err != nil {
		return nil, err
	}
	if o.SignalPeriod < 0 || o.SignalPeriod > int(^uint(0)>>1)/64 {
		return nil, invalid("ROC signalPeriod must be nonnegative")
	}
	data = cleanBars(data)
	out := make([]ROCResult, len(data))
	series := make([]*float64, len(data))
	for i := o.Period; i < len(data); i++ {
		cur, prev := data[i].Close, data[i-o.Period].Close
		if cur == nil || prev == nil || *prev == 0 {
			continue
		}
		out[i].ROC = Float((*cur - *prev) / *prev * 100)
		series[i] = out[i].ROC
	}
	if o.SignalPeriod > 0 {
		w := windowSum{data: series, period: o.SignalPeriod}
		for i := range out {
			w.advance(i)
			if i >= o.Period && i-o.Period >= o.SignalPeriod-1 && w.count == o.SignalPeriod {
				out[i].Signal = Float(w.sum / float64(o.SignalPeriod))
			}
		}
	}
	return out, nil
}

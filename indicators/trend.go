package indicators

import "math"

func CalcMACD(data []*float64, options ...MACDOptions) ([]MACDResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	o.Short = def(o.Short, 12)
	o.Long = def(o.Long, 26)
	o.Signal = def(o.Signal, 9)
	if err := validatePeriods([]int{o.Short, o.Long, o.Signal}, o.Decimals); err != nil {
		return nil, err
	}
	short, long := moving(data, o.Short, "ema", o.Decimals), moving(data, o.Long, "ema", o.Decimals)
	dif := make([]*float64, len(data))
	for i := range data {
		if short[i] != nil && long[i] != nil {
			dif[i] = Float(*short[i] - *long[i])
		}
	}
	dea := moving(dif, o.Signal, "ema", o.Decimals)
	out := make([]MACDResult, len(data))
	for i := range out {
		out[i].DEA = dea[i]
		if dif[i] != nil {
			out[i].DIF = rounded(*dif[i], o.Decimals)
			if dea[i] != nil {
				out[i].MACD = rounded((*dif[i]-*dea[i])*2, o.Decimals)
			}
		}
	}
	return out, nil
}
func CalcBOLL(data []*float64, options ...BOLLOptions) ([]BOLLResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	o.Period = def(o.Period, 20)
	if err := validate(o.Period, o.Decimals); err != nil {
		return nil, err
	}
	k := 2.0
	if o.StdDev != nil {
		if !finite(o.StdDev) || *o.StdDev < 0 {
			return nil, invalid("stdDev must be finite and nonnegative")
		}
		k = *o.StdDev
	}
	data = clean(data)
	mid := moving(data, o.Period, "sma", o.Decimals)
	squares := make([]*float64, len(data))
	for i, v := range data {
		if v != nil {
			squares[i] = Float(*v * *v)
		}
	}
	sums, sqs := windowSum{data: data, period: o.Period}, windowSum{data: squares, period: o.Period}
	out := make([]BOLLResult, len(data))
	p := float64(o.Period)
	for i := range data {
		sums.advance(i)
		sqs.advance(i)
		if mid[i] == nil || sums.count != o.Period {
			continue
		}
		m := *mid[i]
		std := math.Sqrt(math.Max(0, sqs.sum-2*m*sums.sum+p*m*m) / p)
		upper, lower := m+k*std, m-k*std
		out[i] = BOLLResult{Mid: mid[i], Upper: rounded(upper, o.Decimals), Lower: rounded(lower, o.Decimals)}
		if m != 0 {
			out[i].Bandwidth = rounded((upper-lower)/m*100, o.Decimals)
		}
	}
	return out, nil
}
func CalcKDJ(data []OHLCV, options ...KDJOptions) ([]KDJResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	o.Period = def(o.Period, 9)
	o.KPeriod = def(o.KPeriod, 3)
	o.DPeriod = def(o.DPeriod, 3)
	if err := validatePeriods([]int{o.Period, o.KPeriod, o.DPeriod}, o.Decimals); err != nil {
		return nil, err
	}
	data = cleanBars(data)
	out := make([]KDJResult, len(data))
	k, d := 50.0, 50.0
	maxQ, minQ := []int{}, []int{}
	maxHead, minHead, bad := 0, 0, 0
	for i, b := range data {
		if b.High == nil || b.Low == nil {
			bad++
		} else {
			for len(maxQ) > maxHead && *data[maxQ[len(maxQ)-1]].High <= *b.High {
				maxQ = maxQ[:len(maxQ)-1]
			}
			maxQ = append(maxQ, i)
			for len(minQ) > minHead && *data[minQ[len(minQ)-1]].Low >= *b.Low {
				minQ = minQ[:len(minQ)-1]
			}
			minQ = append(minQ, i)
		}
		start := i - o.Period + 1
		if i >= o.Period {
			gone := data[i-o.Period]
			if gone.High == nil || gone.Low == nil {
				bad--
			}
		}
		for len(maxQ) > maxHead && maxQ[maxHead] < start {
			maxHead++
		}
		for len(minQ) > minHead && minQ[minHead] < start {
			minHead++
		}
		if i < o.Period-1 || bad > 0 || b.Close == nil {
			continue
		}
		high, low := *data[maxQ[maxHead]].High, *data[minQ[minHead]].Low
		if high == low {
			continue
		}
		rsv := (*b.Close - low) / (high - low) * 100
		kp, dp := float64(o.KPeriod), float64(o.DPeriod)
		k = ((kp-1)/kp)*k + (1/kp)*rsv
		d = ((dp-1)/dp)*d + (1/dp)*k
		out[i] = KDJResult{rounded(k, o.Decimals), rounded(d, o.Decimals), rounded(3*k-2*d, o.Decimals)}
	}
	return out, nil
}
func CalcATR(data []OHLCV, options ...ATROptions) ([]ATRResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	o.Period = def(o.Period, 14)
	if err := validate(o.Period, o.Decimals); err != nil {
		return nil, err
	}
	data = cleanBars(data)
	trs := make([]*float64, len(data))
	out := make([]ATRResult, len(data))
	for i, b := range data {
		if b.High == nil || b.Low == nil || b.Close == nil {
			continue
		}
		tr := *b.High - *b.Low
		if i > 0 && data[i-1].Close != nil {
			prev := *data[i-1].Close
			tr = math.Max(tr, math.Max(math.Abs(*b.High-prev), math.Abs(*b.Low-prev)))
		}
		trs[i] = Float(tr)
	}
	var atr *float64
	p := float64(o.Period)
	for i, tr := range trs {
		if tr != nil {
			out[i].TR = rounded(*tr, o.Decimals)
		}
		if i < o.Period-1 {
			continue
		}
		if atr == nil {
			sum, count := 0.0, 0
			for j := i - o.Period + 1; j <= i; j++ {
				if trs[j] != nil {
					sum += *trs[j]
					count++
				}
			}
			if count == o.Period {
				atr = Float(sum / p)
			}
		} else if tr != nil {
			atr = Float((*atr*(p-1) + *tr) / p)
		}
		if atr != nil {
			out[i].ATR = rounded(*atr, o.Decimals)
		}
	}
	return out, nil
}
func CalcKC(data []OHLCV, options ...KCOptions) ([]KCResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	o.EMAPeriod = def(o.EMAPeriod, 20)
	o.ATRPeriod = def(o.ATRPeriod, 10)
	if err := validatePeriods([]int{o.EMAPeriod, o.ATRPeriod}, nil); err != nil {
		return nil, err
	}
	k := 2.0
	if o.Multiplier != nil {
		if !finite(o.Multiplier) || *o.Multiplier < 0 {
			return nil, invalid("KC multiplier must be finite and nonnegative")
		}
		k = *o.Multiplier
	}
	ema := moving(closes(data), o.EMAPeriod, "ema", nil)
	atr, err := CalcATR(data, ATROptions{Period: o.ATRPeriod})
	if err != nil {
		return nil, err
	}
	out := make([]KCResult, len(data))
	for i, m := range ema {
		a := atr[i].ATR
		if m == nil || a == nil {
			continue
		}
		upper, lower := *m+k**a, *m-k**a
		out[i] = KCResult{Mid: m, Upper: Float(upper), Lower: Float(lower)}
		if *m > 0 {
			out[i].Width = Float((upper - lower) / *m * 100)
		}
	}
	return out, nil
}

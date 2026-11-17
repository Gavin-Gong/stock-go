package indicators

import "math"

func CalcDMI(data []OHLCV, options ...DMIOptions) ([]DMIResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	o.Period = def(o.Period, 14)
	o.ADXPeriod = def(o.ADXPeriod, o.Period)
	if err := validatePeriods([]int{o.Period, o.ADXPeriod}, nil); err != nil {
		return nil, err
	}
	data = cleanBars(data)
	out := make([]DMIResult, len(data))
	if len(data) < 2 {
		return out, nil
	}
	plus, minus, tr := make([]float64, len(data)), make([]float64, len(data)), make([]float64, len(data))
	for i := 1; i < len(data); i++ {
		cur, prev := data[i], data[i-1]
		if cur.High == nil || cur.Low == nil || cur.Close == nil || prev.High == nil || prev.Low == nil || prev.Close == nil {
			continue
		}
		tr[i] = math.Max(*cur.High-*cur.Low, math.Max(math.Abs(*cur.High-*prev.Close), math.Abs(*cur.Low-*prev.Close)))
		up, down := *cur.High-*prev.High, *prev.Low-*cur.Low
		if up > down && up > 0 {
			plus[i] = up
		}
		if down > up && down > 0 {
			minus[i] = down
		}
	}
	smoothTR, smoothPlus, smoothMinus := 0.0, 0.0, 0.0
	dx := make([]float64, len(data))
	p := float64(o.Period)
	for i := 1; i < len(data); i++ {
		if i <= o.Period {
			smoothTR += tr[i]
			smoothPlus += plus[i]
			smoothMinus += minus[i]
			if i < o.Period {
				continue
			}
		} else {
			smoothTR = smoothTR - smoothTR/p + tr[i]
			smoothPlus = smoothPlus - smoothPlus/p + plus[i]
			smoothMinus = smoothMinus - smoothMinus/p + minus[i]
		}
		pdi, mdi := 0.0, 0.0
		if smoothTR > 0 {
			pdi = smoothPlus / smoothTR * 100
			mdi = smoothMinus / smoothTR * 100
		}
		out[i].PDI = Float(pdi)
		out[i].MDI = Float(mdi)
		if pdi+mdi > 0 {
			dx[i] = math.Abs(pdi-mdi) / (pdi + mdi) * 100
		}
	}
	// Avoid integer overflow for periods longer than the entire available series.
	if o.Period >= len(data) || o.ADXPeriod > len(data)-o.Period {
		return out, nil
	}
	seed := o.Period + o.ADXPeriod - 1
	sum, prevADX := 0.0, 0.0
	ap := float64(o.ADXPeriod)
	for i := o.Period; i < len(data); i++ {
		if i <= seed {
			sum += dx[i]
			if i < seed {
				continue
			}
			prevADX = sum / ap
		} else {
			prevADX = (prevADX*(ap-1) + dx[i]) / ap
		}
		out[i].ADX = Float(prevADX)
	}
	for i := seed; i < len(data); i++ {
		if i-seed < o.ADXPeriod {
			continue
		}
		cur, prev := out[i].ADX, out[i-o.ADXPeriod].ADX
		if cur != nil && prev != nil {
			out[i].ADXR = Float((*cur + *prev) / 2)
		}
	}
	return out, nil
}
func CalcSAR(data []OHLCV, options ...SAROptions) ([]SARResult, error) {
	o, err := one(options)
	if err != nil {
		return nil, err
	}
	start, inc, maxAF := 0.02, 0.02, 0.2
	for _, p := range []*float64{o.AFStart, o.AFIncrement, o.AFMax} {
		if p != nil && (!finite(p) || *p < 0) {
			return nil, invalid("SAR acceleration factors must be finite and nonnegative")
		}
	}
	if o.AFStart != nil {
		start = *o.AFStart
	}
	if o.AFIncrement != nil {
		inc = *o.AFIncrement
	}
	if o.AFMax != nil {
		maxAF = *o.AFMax
	}
	if start > maxAF {
		return nil, invalid("SAR afStart must not exceed afMax")
	}
	data = cleanBars(data)
	out := make([]SARResult, len(data))
	if len(data) < 2 {
		return out, nil
	}
	seed := 0
	for seed < len(data) && (data[seed].High == nil || data[seed].Low == nil) {
		seed++
	}
	if seed >= len(data)-1 {
		return out, nil
	}
	trend, af, ep, sar := 1, start, *data[seed].High, *data[seed].Low
	if a, b := data[seed].Close, data[seed+1].Close; a != nil && b != nil && *b < *a {
		trend = -1
		ep = *data[seed].Low
		sar = *data[seed].High
	}
	for i := seed + 1; i < len(data); i++ {
		cur, prev := data[i], data[i-1]
		if cur.High == nil || cur.Low == nil || prev.High == nil || prev.Low == nil {
			continue
		}
		next := sar + af*(ep-sar)
		before := data[max(seed, i-2)]
		if trend == 1 {
			low := prev.Low
			if before.Low != nil {
				low = before.Low
			}
			next = math.Min(next, math.Min(*prev.Low, *low))
			if *cur.Low < next {
				trend = -1
				next = ep
				ep = *cur.Low
				af = start
			} else if *cur.High > ep {
				ep = *cur.High
				af = math.Min(af+inc, maxAF)
			}
		} else {
			high := prev.High
			if before.High != nil {
				high = before.High
			}
			next = math.Max(next, math.Max(*prev.High, *high))
			if *cur.High > next {
				trend = 1
				next = ep
				ep = *cur.High
				af = start
			} else if *cur.Low < ep {
				ep = *cur.Low
				af = math.Min(af+inc, maxAF)
			}
		}
		sar = next
		out[i] = SARResult{Float(sar), Int(trend), Float(ep), Float(af)}
	}
	return out, nil
}

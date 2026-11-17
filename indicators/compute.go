package indicators

import "math"

type Options struct {
	MA   *MAOptions   `json:"ma,omitempty"`
	MACD *MACDOptions `json:"macd,omitempty"`
	BOLL *BOLLOptions `json:"boll,omitempty"`
	KDJ  *KDJOptions  `json:"kdj,omitempty"`
	RSI  *RSIOptions  `json:"rsi,omitempty"`
	WR   *WROptions   `json:"wr,omitempty"`
	BIAS *BIASOptions `json:"bias,omitempty"`
	CCI  *CCIOptions  `json:"cci,omitempty"`
	ATR  *ATROptions  `json:"atr,omitempty"`
	OBV  *OBVOptions  `json:"obv,omitempty"`
	ROC  *ROCOptions  `json:"roc,omitempty"`
	DMI  *DMIOptions  `json:"dmi,omitempty"`
	SAR  *SAROptions  `json:"sar,omitempty"`
	KC   *KCOptions   `json:"kc,omitempty"`
}

// Result contains only the indicators explicitly requested by Options.
type Result struct {
	MA   Values      `json:"ma,omitempty"`
	MACD *MACDResult `json:"macd,omitempty"`
	BOLL *BOLLResult `json:"boll,omitempty"`
	KDJ  *KDJResult  `json:"kdj,omitempty"`
	RSI  Values      `json:"rsi,omitempty"`
	WR   Values      `json:"wr,omitempty"`
	BIAS Values      `json:"bias,omitempty"`
	CCI  *CCIResult  `json:"cci,omitempty"`
	ATR  *ATRResult  `json:"atr,omitempty"`
	OBV  *OBVResult  `json:"obv,omitempty"`
	ROC  *ROCResult  `json:"roc,omitempty"`
	DMI  *DMIResult  `json:"dmi,omitempty"`
	SAR  *SARResult  `json:"sar,omitempty"`
	KC   *KCResult   `json:"kc,omitempty"`
}

func Compute(data []OHLCV, o Options) ([]Result, error) {
	out := make([]Result, len(data))
	c := closes(data)
	_ = c
	if o.MA != nil {
		values, err := CalcMA(c, *o.MA)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].MA = values[i]
		}
	}
	if o.MACD != nil {
		values, err := CalcMACD(c, *o.MACD)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].MACD = &values[i]
		}
	}
	if o.BOLL != nil {
		values, err := CalcBOLL(c, *o.BOLL)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].BOLL = &values[i]
		}
	}
	if o.KDJ != nil {
		values, err := CalcKDJ(data, *o.KDJ)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].KDJ = &values[i]
		}
	}
	if o.RSI != nil {
		values, err := CalcRSI(c, *o.RSI)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].RSI = values[i]
		}
	}
	if o.WR != nil {
		values, err := CalcWR(data, *o.WR)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].WR = values[i]
		}
	}
	if o.BIAS != nil {
		values, err := CalcBIAS(c, *o.BIAS)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].BIAS = values[i]
		}
	}
	if o.CCI != nil {
		values, err := CalcCCI(data, *o.CCI)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].CCI = &values[i]
		}
	}
	if o.ATR != nil {
		values, err := CalcATR(data, *o.ATR)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].ATR = &values[i]
		}
	}
	if o.OBV != nil {
		values, err := CalcOBV(data, *o.OBV)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].OBV = &values[i]
		}
	}
	if o.ROC != nil {
		values, err := CalcROC(data, *o.ROC)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].ROC = &values[i]
		}
	}
	if o.DMI != nil {
		values, err := CalcDMI(data, *o.DMI)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].DMI = &values[i]
		}
	}
	if o.SAR != nil {
		values, err := CalcSAR(data, *o.SAR)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].SAR = &values[i]
		}
	}
	if o.KC != nil {
		values, err := CalcKC(data, *o.KC)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].KC = &values[i]
		}
	}
	return out, nil
}

type EnrichedBar struct {
	OHLCV
	Result
}

func AddIndicators(data []OHLCV, o Options) ([]EnrichedBar, error) {
	r, err := Compute(data, o)
	if err != nil {
		return nil, err
	}
	out := make([]EnrichedBar, len(data))
	for i, b := range data {
		out[i] = EnrichedBar{b, r[i]}
	}
	return out, nil
}

// Lookback describes the pre-window bars needed to seed the requested indicators.
type Lookback struct {
	MaxLookback, RequiredBars, MaxRecursiveLookback int
	HasEMABased, HasCumulative                      bool
}

func EstimateLookback(o Options) (Lookback, error) {
	if _, err := Compute(nil, o); err != nil {
		return Lookback{}, err
	}
	var out Lookback
	add := func(bars int, recursive, ema bool) {
		out.MaxLookback = max(out.MaxLookback, bars)
		if recursive || ema {
			out.MaxRecursiveLookback = max(out.MaxRecursiveLookback, bars)
		}
		out.HasEMABased = out.HasEMABased || ema
	}
	periodsMax := func(p []int, fallback []int, emptyFallback int) int {
		if p == nil {
			p = fallback
		}
		if len(p) == 0 {
			return emptyFallback
		}
		v := 0
		for _, n := range p {
			v = max(v, n)
		}
		return v
	}
	if o.MA != nil {
		add(periodsMax(o.MA.Periods, []int{5, 10, 20, 30, 60, 120, 250}, 20), false, o.MA.Type == "ema")
	}
	if o.MACD != nil {
		add(def(o.MACD.Long, 26)*3+def(o.MACD.Signal, 9), true, true)
	}
	if o.BOLL != nil {
		add(def(o.BOLL.Period, 20), false, false)
	}
	if o.KDJ != nil {
		add(def(o.KDJ.Period, 9), true, false)
	}
	if o.RSI != nil {
		add(periodsMax(o.RSI.Periods, []int{6, 12, 24}, 14)+1, true, false)
	}
	if o.WR != nil {
		add(periodsMax(o.WR.Periods, []int{6, 10}, 10), false, false)
	}
	if o.BIAS != nil {
		add(periodsMax(o.BIAS.Periods, []int{6, 12, 24}, 12), false, false)
	}
	if o.CCI != nil {
		add(def(o.CCI.Period, 14), false, false)
	}
	if o.ATR != nil {
		add(def(o.ATR.Period, 14), true, false)
	}
	if o.OBV != nil {
		add(max(2, o.OBV.MAPeriod), false, false)
		out.HasCumulative = true
	}
	if o.ROC != nil {
		add(def(o.ROC.Period, 12)+o.ROC.SignalPeriod, false, false)
	}
	if o.DMI != nil {
		p := def(o.DMI.Period, 14)
		add(p*2+def(o.DMI.ADXPeriod, p), true, false)
	}
	if o.SAR != nil {
		add(5, true, false)
	}
	if o.KC != nil {
		add(max(def(o.KC.EMAPeriod, 20)*3, def(o.KC.ATRPeriod, 10)), true, true)
	}
	buffer := 1.2
	if out.HasEMABased {
		buffer = 1.5
	}
	out.RequiredBars = int(math.Ceil(float64(out.MaxLookback) * buffer))
	return out, nil
}

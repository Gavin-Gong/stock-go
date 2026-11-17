package indicators

import (
	"encoding/json"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"math"
	"testing"
)

func TestInvalidOptions(t *testing.T) {
	for _, run := range []func() error{
		func() error { _, e := CalcSMA(nil, 0); return e }, func() error { _, e := CalcEMA(nil, 2, 1, 2); return e }, func() error { _, e := CalcWMA(nil, 2, 16); return e },
		func() error { _, e := CalcMA(nil, MAOptions{Periods: []int{-1}}); return e }, func() error { _, e := CalcMA(nil, MAOptions{Type: "bad"}); return e },
		func() error { _, e := CalcMACD(nil, MACDOptions{Short: -1}); return e }, func() error { _, e := CalcBOLL(nil, BOLLOptions{StdDev: Float(-1)}); return e }, func() error { _, e := CalcKDJ(nil, KDJOptions{KPeriod: -1}); return e },
		func() error { _, e := CalcRSI(nil, RSIOptions{Decimals: Int(16)}); return e }, func() error { _, e := CalcWR(nil, WROptions{Periods: []int{0}}); return e }, func() error { _, e := CalcBIAS(nil, BIASOptions{Periods: []int{-1}}); return e },
		func() error { _, e := CalcCCI(nil, CCIOptions{Period: -1}); return e }, func() error { _, e := CalcATR(nil, ATROptions{Period: -1}); return e }, func() error { _, e := CalcOBV(nil, OBVOptions{MAPeriod: -1}); return e }, func() error { _, e := CalcROC(nil, ROCOptions{SignalPeriod: -1}); return e },
		func() error { _, e := CalcDMI(nil, DMIOptions{ADXPeriod: -1}); return e }, func() error { _, e := CalcSAR(nil, SAROptions{AFStart: Float(0.5)}); return e }, func() error { _, e := CalcSAR(nil, SAROptions{AFIncrement: Float(math.NaN())}); return e }, func() error { _, e := CalcKC(nil, KCOptions{Multiplier: Float(-1)}); return e },
		func() error { _, e := CalcMA(nil, MAOptions{}, MAOptions{}); return e }, func() error { _, e := Compute(nil, Options{ATR: &ATROptions{Period: -1}}); return e }, func() error { _, e := EstimateLookback(Options{ATR: &ATROptions{Period: -1}}); return e },
	} {
		if err := run(); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
		}
	}
}
func TestMissingDataRecoversAndPrecisionZero(t *testing.T) {
	data := []*float64{Float(10), nil, Float(12), Float(14), nil, Float(16)}
	values, err := CalcEMA(data, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []*float64{nil, nil, nil, Float(13), Float(13), Float(15)}
	for i := range want {
		if (values[i] == nil) != (want[i] == nil) || (want[i] != nil && *values[i] != *want[i]) {
			t.Fatalf("index %d: got %v want %v", i, values[i], want[i])
		}
	}
	invalidData := []*float64{Float(math.NaN()), Float(math.Inf(1)), Float(1), Float(2), Float(3)}
	sma, err := CalcSMA(invalidData, 2)
	if err != nil || sma[0] != nil || sma[1] != nil || sma[3] == nil {
		t.Fatalf("%v %v", sma, err)
	}
	if _, err := json.Marshal(sma); err != nil {
		t.Fatal(err)
	}
	if v := rounded(-1.5, Int(0)); v == nil || *v != -1 {
		t.Fatal(v)
	}
}
func FuzzCompute(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 0, 9})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 300 {
			input = input[:300]
		}
		bars := make([]OHLCV, len(input))
		for i, b := range input {
			if b == 0 {
				continue
			}
			c := float64(b)
			bars[i] = OHLCV{Open: Float(c), Close: Float(c), High: Float(c + 1), Low: Float(c - 1), Volume: Float(float64(i + 1))}
		}
		out, err := Compute(bars, Options{MA: &MAOptions{Periods: []int{2, 3}}, MACD: &MACDOptions{Short: 2, Long: 3, Signal: 2}, BOLL: &BOLLOptions{Period: 3}, KDJ: &KDJOptions{Period: 3}, RSI: &RSIOptions{Periods: []int{2}}, WR: &WROptions{Periods: []int{2}}, BIAS: &BIASOptions{Periods: []int{2}}, CCI: &CCIOptions{Period: 3}, ATR: &ATROptions{Period: 3}, OBV: &OBVOptions{MAPeriod: 3}, ROC: &ROCOptions{Period: 2, SignalPeriod: 2}, DMI: &DMIOptions{Period: 2, ADXPeriod: 2}, SAR: &SAROptions{}, KC: &KCOptions{EMAPeriod: 3, ATRPeriod: 2}})
		if err != nil || len(out) != len(input) {
			t.Fatalf("len=%d %v", len(out), err)
		}
		if _, err := json.Marshal(out); err != nil {
			t.Fatal(err)
		}
	})
}

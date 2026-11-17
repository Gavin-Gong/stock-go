package indicators

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
)

func TestUpstreamParity(t *testing.T) {
	body, err := os.ReadFile("../testdata/indicator-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Datasets map[string][]OHLCV
		Fixtures []struct {
			Name, Dataset     string
			Options, Expected json.RawMessage
		}
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatal(err)
	}
	for index, f := range data.Fixtures {
		t.Run(fmt.Sprintf("%s/%s/%d", f.Name, f.Dataset, index), func(t *testing.T) {
			bars := data.Datasets[f.Dataset]
			c := closes(bars)
			var got any
			var err error
			switch f.Name {
			case "SMA", "EMA", "WMA":
				var o struct {
					Period   int
					Decimals *int
				}
				json.Unmarshal(f.Options, &o)
				d := []int{}
				if o.Decimals != nil {
					d = append(d, *o.Decimals)
				}
				p := def(o.Period, 5)
				switch f.Name {
				case "SMA":
					got, err = CalcSMA(c, p, d...)
				case "EMA":
					got, err = CalcEMA(c, p, d...)
				case "WMA":
					got, err = CalcWMA(c, p, d...)
				}
			case "MA":
				var o MAOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcMA(c, o)
			case "MACD":
				var o MACDOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcMACD(c, o)
			case "BOLL":
				var o BOLLOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcBOLL(c, o)
			case "KDJ":
				var o KDJOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcKDJ(bars, o)
			case "RSI":
				var o RSIOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcRSI(c, o)
			case "WR":
				var o WROptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcWR(bars, o)
			case "BIAS":
				var o BIASOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcBIAS(c, o)
			case "CCI":
				var o CCIOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcCCI(bars, o)
			case "ATR":
				var o ATROptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcATR(bars, o)
			case "OBV":
				var o OBVOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcOBV(bars, o)
			case "ROC":
				var o ROCOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcROC(bars, o)
			case "DMI":
				var o DMIOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcDMI(bars, o)
			case "SAR":
				var o SAROptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcSAR(bars, o)
			case "KC":
				var o KCOptions
				json.Unmarshal(f.Options, &o)
				got, err = CalcKC(bars, o)
			default:
				t.Fatal("unknown function", f.Name)
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var actual, expected any
			json.Unmarshal(encoded, &actual)
			json.Unmarshal(f.Expected, &expected)
			compare(t, actual, expected, "result")
		})
	}
}
func compare(t *testing.T, a, b any, path string) {
	t.Helper()
	switch b := b.(type) {
	case nil:
		if a != nil {
			t.Errorf("%s: Go=%v TS=null", path, a)
		}
	case float64:
		v, ok := a.(float64)
		if !ok || math.Abs(v-b) > 1e-9*math.Max(1, math.Abs(b)) {
			t.Errorf("%s: Go=%v TS=%.15g", path, a, b)
		}
	case []any:
		v, ok := a.([]any)
		if !ok || len(v) != len(b) {
			t.Fatalf("%s: array length mismatch", path)
		}
		for i := range b {
			compare(t, v[i], b[i], fmt.Sprintf("%s[%d]", path, i))
		}
	case map[string]any:
		v, ok := a.(map[string]any)
		if !ok || len(v) != len(b) {
			t.Fatalf("%s: object shape mismatch Go=%v TS=%v", path, a, b)
		}
		for k, item := range b {
			value, exists := v[k]
			if !exists {
				t.Errorf("%s: missing key %s", path, k)
				continue
			}
			compare(t, value, item, path+"."+k)
		}
	default:
		if a != b {
			t.Errorf("%s: Go=%v TS=%v", path, a, b)
		}
	}
}

func TestComputeAndLookbackParity(t *testing.T) {
	body, err := os.ReadFile("../testdata/indicator-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Datasets  map[string][]OHLCV
		Pipelines []struct {
			Dataset    string
			Options    Options
			Expected   json.RawMessage
			Cumulative bool
			Lookback   struct {
				MaxLookback, RequiredBars, MaxRecursiveLookback int
				HasEmaBasedIndicator                            bool
			}
		}
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatal(err)
	}
	for i, f := range data.Pipelines {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			input := data.Datasets[f.Dataset]
			before, _ := json.Marshal(input)
			enriched, err := AddIndicators(input, f.Options)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("input mutated")
			}
			gotJSON, err := json.Marshal(enriched)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			json.Unmarshal(gotJSON, &got)
			json.Unmarshal(f.Expected, &want)
			compare(t, got, want, "pipeline")
			estimate, err := EstimateLookback(f.Options)
			if err != nil {
				t.Fatal(err)
			}
			if estimate.MaxLookback != f.Lookback.MaxLookback || estimate.RequiredBars != f.Lookback.RequiredBars || estimate.MaxRecursiveLookback != f.Lookback.MaxRecursiveLookback || estimate.HasEMABased != f.Lookback.HasEmaBasedIndicator || estimate.HasCumulative != f.Cumulative {
				t.Fatalf("Go=%+v TS=%+v", estimate, f.Lookback)
			}
		})
	}
}

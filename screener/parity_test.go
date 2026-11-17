package screener

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"testing"
)

type testBar struct {
	Close *float64
	Date  string
}

func TestUpstreamParity(t *testing.T) {
	body, err := os.ReadFile("../testdata/strategy-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Backtests []struct {
			Name                         string
			Bars                         []*testBar
			Actions                      []StrategySignal
			InitialCapital, PositionSize *float64
			Fee, Expected                json.RawMessage
		}
		Screens []struct {
			Items []struct {
				ID string `json:"id"`
				V  any    `json:"v"`
			}
			Direction Direction
			Expected  json.RawMessage
		}
	}
	if err := json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures.Backtests {
		t.Run(f.Name, func(t *testing.T) {
			fee := Fee{}
			if len(f.Fee) > 0 {
				var n float64
				if json.Unmarshal(f.Fee, &n) == nil {
					fee = FeeRate(n)
				} else if err := json.Unmarshal(f.Fee, &fee); err != nil {
					t.Fatal(err)
				}
			}
			report, err := Backtest(BacktestOptions[*testBar]{Klines: f.Bars, Fee: fee, InitialCapital: f.InitialCapital, PositionSize: f.PositionSize, GetClose: func(b *testBar) *float64 { return b.Close }, GetDate: func(b *testBar) any { return b.Date }, Strategy: func(b *testBar, i int, all []*testBar) StrategySignal {
				if b == nil || len(all) != len(f.Bars) {
					t.Fatal("strategy contract violated")
				}
				return f.Actions[i]
			}})
			if err != nil {
				t.Fatal(err)
			}
			compareJSON(t, report, f.Expected)
		})
	}
	for _, f := range fixtures.Screens {
		t.Run(string(f.Direction), func(t *testing.T) {
			builder := Screen(f.Items).SortByValue(func(v struct {
				ID string `json:"id"`
				V  any    `json:"v"`
			}) any {
				return v.V
			}, f.Direction)
			items, err := builder.ToArray()
			if err != nil {
				t.Fatal(err)
			}
			compareJSON(t, items, f.Expected)
		})
	}
}
func compareJSON(t *testing.T, got any, expected json.RawMessage) {
	t.Helper()
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	json.Unmarshal(encoded, &a)
	json.Unmarshal(expected, &b)
	compare(t, a, b, "result")
}
func compare(t *testing.T, a, b any, path string) {
	t.Helper()
	switch b := b.(type) {
	case float64:
		v, ok := a.(float64)
		if !ok || math.Abs(v-b) > 1e-9*math.Max(1, math.Abs(b)) {
			t.Errorf("%s: Go=%v TS=%v", path, a, b)
		}
	case []any:
		v, ok := a.([]any)
		if !ok || len(v) != len(b) {
			t.Fatalf("%s: array mismatch Go=%v TS=%v", path, a, b)
		}
		for i := range b {
			compare(t, v[i], b[i], fmt.Sprintf("%s[%d]", path, i))
		}
	case map[string]any:
		v, ok := a.(map[string]any)
		if !ok || len(v) != len(b) {
			t.Fatalf("%s: object mismatch Go=%v TS=%v", path, a, b)
		}
		for k, x := range b {
			g, exists := v[k]
			if !exists {
				t.Fatalf("missing key %s", k)
			}
			compare(t, g, x, path+"."+k)
		}
	default:
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: Go=%v TS=%v", path, a, b)
		}
	}
}
func TestScreenCopyAndErrors(t *testing.T) {
	input := []int{3, 1, 2, 2}
	b := Screen(input).Where(func(v int) bool { return v >= 2 }).SortBy(func(v int) *float64 { n := float64(v); return &n })
	input[0] = 999
	top, err := b.Top(2)
	if err != nil || !reflect.DeepEqual(top, []int{3, 2}) {
		t.Fatalf("%v %v", top, err)
	}
	top[0] = 0
	again, _ := b.ToArray()
	if again[0] != 3 {
		t.Fatal("result aliases builder state")
	}
	if _, err := b.Top(-1); err == nil {
		t.Fatal("negative top")
	}
	for _, builder := range []*Builder[int]{Screen(input).Where(nil), Screen(input).SortBy(nil), Screen(input).SortByValue(nil), Screen(input).SortBy(func(int) *float64 { return nil }, "bad"), Screen(input).SortBy(func(int) *float64 { return nil }, Ascending, Descending)} {
		if builder.Err() == nil {
			t.Fatal("missing builder error")
		}
		if _, err := builder.Where(func(int) bool { return true }).SortBy(nil).Top(1); err == nil {
			t.Fatal("missing sticky error")
		}
		if _, err := builder.ToArray(); err == nil {
			t.Fatal("missing toArray error")
		}
	}
}
func TestBacktestValidation(t *testing.T) {
	base := BacktestOptions[testBar]{Strategy: func(testBar, int, []testBar) StrategySignal { return Hold }, GetClose: func(b testBar) *float64 { return b.Close }}
	for _, change := range []func(*BacktestOptions[testBar]){func(o *BacktestOptions[testBar]) { o.InitialCapital = Float(0) }, func(o *BacktestOptions[testBar]) { o.InitialCapital = Float(math.NaN()) }, func(o *BacktestOptions[testBar]) { o.PositionSize = Float(0) }, func(o *BacktestOptions[testBar]) { o.PositionSize = Float(1.1) }, func(o *BacktestOptions[testBar]) { o.Fee = FeeRate(-0.1) }, func(o *BacktestOptions[testBar]) { o.Fee = Fee{Sell: 1} }, func(o *BacktestOptions[testBar]) { o.Strategy = nil }, func(o *BacktestOptions[testBar]) { o.GetClose = nil }} {
		o := base
		change(&o)
		if _, err := Backtest(o); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
	o := base
	o.Klines = []testBar{{Close: Float(1)}}
	o.Strategy = func(testBar, int, []testBar) StrategySignal { return "bad" }
	if _, err := Backtest(o); err == nil {
		t.Fatal("invalid strategy signal")
	}
}
func TestSortNumberAndNonfinite(t *testing.T) {
	for _, v := range []any{nil, " ", "--", "1.5%", true, math.NaN(), math.Inf(1)} {
		if n := SortNumber(v); n != nil {
			t.Errorf("%v became %v", v, *n)
		}
	}
	for _, v := range []any{int(10), int64(10), uint(10), float32(10), "10", " 10 ", "0xa"} {
		if n := SortNumber(v); n == nil || *n != 10 {
			t.Errorf("%v became %v", v, n)
		}
	}
}

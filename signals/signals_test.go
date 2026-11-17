package signals

import (
	"encoding/json"
	"github.com/Gavin-Gong/stock-go/indicators"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"os"
	"reflect"
	"testing"
)

func TestUpstreamParity(t *testing.T) {
	body, err := os.ReadFile("../testdata/strategy-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Signals []struct {
			Bars     []Bar
			Options  Options
			Expected []Signal
		}
	}
	if err := json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	for i, f := range fixtures.Signals {
		got, err := CalcSignals(f.Bars, f.Options)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, f.Expected) {
			t.Fatalf("fixture %d: Go=%v TS=%v", i, got, f.Expected)
		}
	}
}
func TestValidationAndMissingPeriods(t *testing.T) {
	bars := []Bar{{Result: indicators.Result{MA: indicators.Values{"ma5": nil, "ma20": nil}, RSI: indicators.Values{"rsi6": nil}}}}
	for _, o := range []Options{{MA: &MACross{5, 5}}, {MA: &MACross{-1, 5}}, {MA: &MACross{3, 8}}, {RSI: &RSIOptions{Period: 12}}, {RSI: &RSIOptions{Period: -1}}} {
		if _, err := CalcSignals(bars, o); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatalf("%+v %v", o, err)
		}
	}
	if _, err := CalcSignals(bars, Options{MA: &MACross{5, 20}, RSI: &RSIOptions{}}); err != nil {
		t.Fatal(err)
	}
	if result, err := CalcSignals(nil, Options{MA: &MACross{5, 20}}); err != nil || result == nil || len(result) != 0 {
		t.Fatalf("%v %v", result, err)
	}
}
func TestThresholdOverrides(t *testing.T) {
	timestamp := int64(123)
	bars := []Bar{{}, {Timestamp: &timestamp, Close: indicators.Float(1), Result: indicators.Result{KDJ: &indicators.KDJResult{K: indicators.Float(40)}, RSI: indicators.Values{"rsi6": indicators.Float(40)}}}}
	options := Options{KDJ: &Thresholds{Overbought: indicators.Float(30), Oversold: indicators.Float(10)}, RSI: &RSIOptions{Thresholds: Thresholds{Overbought: indicators.Float(30)}}}
	result, err := CalcSignals(bars, options)
	if err != nil || len(result) != 2 || result[0].Type != "kdj_overbought" || result[1].Type != "rsi_overbought" {
		t.Fatalf("%v %v", result, err)
	}
}

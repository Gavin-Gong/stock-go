package indicators

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
)

func TestChipUpstreamParity(t *testing.T) {
	body, err := os.ReadFile("../testdata/chip-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Datasets map[string][]ChipBar
		Fixtures []struct {
			Dataset  string
			Options  ChipOptions
			Expected json.RawMessage
		}
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatal(err)
	}
	for i, f := range data.Fixtures {
		t.Run(fmt.Sprintf("%s/%d", f.Dataset, i), func(t *testing.T) {
			out, err := CalcChipDistribution(data.Datasets[f.Dataset], f.Options)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			json.Unmarshal(encoded, &got)
			json.Unmarshal(f.Expected, &want)
			compare(t, got, want, "chips")
		})
	}
}
func TestChipValidationPriceAndHistogram(t *testing.T) {
	for _, o := range []ChipOptions{{Range: Int(-1)}, {Decimals: Int(16)}, {IncludeHistogram: "invalid"}} {
		if _, err := CalcChipDistribution(nil, o); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
	if _, err := CalcChipDistribution(nil, ChipOptions{}, ChipOptions{}); err == nil {
		t.Fatal("multiple options")
	}
	for _, c := range []struct{ input, want float64 }{{1.125, 1.13}, {-1.125, -1.13}, {2.675, 2.67}, {1e25, 1e25}} {
		if got := jsPrice(c.input); got != c.want {
			t.Fatalf("%v: %v want %v", c.input, got, c.want)
		}
	}
	if got := prec12(100000000000.5); got != 100000000001 {
		t.Fatal(got)
	}
	if got := prec12(-100000000000.5); got != -100000000001 {
		t.Fatal(got)
	}
	bars := []ChipBar{{Date: "D", Open: Float(10), High: Float(11), Low: Float(9), Close: Float(10), TurnoverRate: Float(5)}}
	result, err := CalcChipDistribution(bars, ChipOptions{IncludeHistogram: HistogramAll})
	if err != nil || len(result) != 1 || result[0].Histogram == nil {
		t.Fatalf("%v %v", result, err)
	}
	h := result[0].Histogram
	sum := 0.0
	for _, r := range h.Ratios {
		if r < 0 || r > 1 {
			t.Fatal(r)
		}
		sum += r
	}
	if len(h.Prices) != 150 || math.Abs(sum-1) > 0.0001 {
		t.Fatalf("histogram length=%d sum=%v", len(h.Prices), sum)
	}
}
func FuzzChipDistribution(f *testing.F) {
	f.Add([]byte{1, 2, 0, 3, 255})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 50 {
			raw = raw[:50]
		}
		bars := make([]ChipBar, len(raw))
		for i, n := range raw {
			if n == 0 {
				continue
			}
			p := float64(n) / 10
			bars[i] = ChipBar{Date: fmt.Sprint(i), Open: Float(p), High: Float(p + 0.5), Low: Float(p - 0.5), Close: Float(p), TurnoverRate: Float(float64(n))}
		}
		got, err := CalcChipDistribution(bars, ChipOptions{Range: Int(5), Tail: Int(3), IncludeHistogram: HistogramLast})
		if err != nil || len(got) != min(3, len(bars)) {
			t.Fatalf("%v %v", got, err)
		}
		if _, err := json.Marshal(got); err != nil {
			t.Fatal(err)
		}
	})
}

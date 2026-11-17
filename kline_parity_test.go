package stockgo

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestKlineUpstreamParity(t *testing.T) {
	body, err := os.ReadFile("testdata/kline-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Kind, Line                              string
		History, Anchor, Minute, Sina, Expected json.RawMessage
		Options                                 struct{ Period, StartDate, EndDate string }
	}
	if err := json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Kind+f.Line, func(t *testing.T) {
			var actual any
			if f.Kind == "csv" {
				actual = csvBar(f.Line)
			} else {
				sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
					var raw json.RawMessage
					switch f.Kind {
					case "tencentHistory":
						raw = f.Anchor
						if strings.HasSuffix(r.URL.Query().Get("param"), ",hfq") {
							raw = f.History
						}
					case "tencentMinute":
						raw = f.Minute
					case "sinaHistory":
						raw = f.Sina
					}
					w.Write(raw)
				}, false, 0)
				o, _ := historyOptions([]KlineOptions{{Period: f.Options.Period, StartDate: f.Options.StartDate, EndDate: f.Options.EndDate}})
				switch f.Kind {
				case "tencentHistory":
					actual, err = sdk.Kline.tencentHistory(context.Background(), "600519", o)
				case "sinaHistory":
					actual, err = sdk.Kline.sinaBars(context.Background(), "600519", o, "")
				case "tencentMinute":
					mo, _ := minuteOptions([]MinuteOptions{{Period: f.Options.Period, StartDate: f.Options.StartDate, EndDate: f.Options.EndDate}})
					actual, err = sdk.Kline.tencentMinute(context.Background(), "600519", mo)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			encoded, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(f.Expected, &want); err != nil {
				t.Fatal(err)
			}
			compareSourceFields(t, got, want)
		})
	}
}

// Go adds source/adjust/code metadata and unifies history/minute types. Compare all source fields.
func compareSourceFields(t *testing.T, got, want any) {
	t.Helper()
	switch want := want.(type) {
	case []any:
		actual, ok := got.([]any)
		if !ok || len(actual) != len(want) {
			t.Fatalf("row mismatch: got %v want %v", got, want)
		}
		for i := range want {
			compareSourceFields(t, actual[i], want[i])
		}
	case map[string]any:
		actual, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("expected object, got %v", got)
		}
		for k, v := range want {
			g, exists := actual[k]
			if !exists || !reflect.DeepEqual(g, v) {
				t.Errorf("field %s: Go=%v (exists=%v), TS=%v", k, g, exists, v)
			}
		}
	default:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Go=%v TS=%v", got, want)
		}
	}
}

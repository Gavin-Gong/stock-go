package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Gavin-Gong/stock-go/cache"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"math"
	"os"
	"reflect"
	"sort"
	"testing"
)

func TestPublicHelpersUpstreamParity(t *testing.T) {
	body, e := os.ReadFile("testdata/helpers-parity.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixtures []struct {
		Kind     string
		Input    json.RawMessage
		Expected json.RawMessage
	}
	if e = json.Unmarshal(body, &fixtures); e != nil {
		t.Fatal(e)
	}
	for index, f := range fixtures {
		t.Run(fmt.Sprintf("%03d_%s", index, f.Kind), func(t *testing.T) {
			var input any
			if e := json.Unmarshal(f.Input, &input); e != nil {
				t.Fatal(e)
			}
			var got any
			var err error
			switch f.Kind {
			case "safeNumber":
				got = SafeNumber(input.(string))
			case "safeNumberOrNull":
				got = SafeNumberOrNull(input.(string))
			case "toNumber":
				got = ToNumber(input.(string))
			case "toNumberSafe":
				got = ToNumberSafe(input)
			case "toFiniteNumberOrNull":
				got = ToFiniteNumberOrNull(input)
			case "parseResponse":
				got = ParseResponse(input.(string))
			case "decodeGBK":
				var bytes []int
				json.Unmarshal(f.Input, &bytes)
				data := make([]byte, len(bytes))
				for i, n := range bytes {
					data[i] = byte(n)
				}
				got, err = DecodeGBK(data)
			case "parseJSVars":
				got = ParseJSVars(input.(string), []string{"x", "y", "z"})
			case "extractJSONFromJSONP":
				v, e := ExtractJSONFromJSONP(input.(string))
				if e != nil {
					got = map[string]any{"error": sdkerrors.CodeOf(e)}
				} else {
					got = map[string]any{"value": v}
				}
			case "externalLinks":
				var result SearchResult
				json.Unmarshal(f.Input, &result)
				got = GenerateSearchExternalLinks(result)
			case "buildTimeMeta":
				var in struct{ Local, TZ string }
				json.Unmarshal(f.Input, &in)
				got = BuildTimeMeta(in.Local, in.TZ)
			case "formatInTZ":
				var in struct {
					Epoch *int64
					TZ    string
				}
				json.Unmarshal(f.Input, &in)
				got = FormatInTZ(in.Epoch, in.TZ)
			case "buildTimeMetaFromDateAndTime":
				var in struct{ Date, Clock, TZ string }
				json.Unmarshal(f.Input, &in)
				got = BuildTimeMetaFromDateAndTime(in.Date, in.Clock, in.TZ)
			case "cacheKey":
				got = cache.CreateKey(input.([]any)...)
			case "chunkArray":
				var in struct {
					Items []int
					Size  int
				}
				json.Unmarshal(f.Input, &in)
				got, err = ChunkArray(in.Items, in.Size)
			case "asyncPool":
				var in struct {
					Values      []int
					Concurrency int
					Ordered     bool
				}
				json.Unmarshal(f.Input, &in)
				tasks := make([]func(context.Context) (int, error), len(in.Values))
				for i, n := range in.Values {
					tasks[i] = func(context.Context) (int, error) { return n, nil }
				}
				var values []int
				values, err = AsyncPool(context.Background(), tasks, in.Concurrency, in.Ordered)
				if !in.Ordered {
					sort.Ints(values)
				}
				got = values
			default:
				t.Fatal("unknown fixture", f.Kind)
			}
			if err != nil {
				t.Fatal(err)
			}
			if n, ok := got.(*float64); ok && n != nil {
				got = *n
			}
			if n, ok := got.(float64); ok && math.IsInf(n, 0) {
				if n > 0 {
					got = "Infinity"
				} else {
					got = "-Infinity"
				}
			}
			actual, e := json.Marshal(got)
			if e != nil {
				t.Fatal(e)
			}
			var a, b any
			json.Unmarshal(actual, &a)
			json.Unmarshal(f.Expected, &b)
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("input %s\ngot %s\nwant %s", f.Input, actual, f.Expected)
			}
		})
	}
	if !t.Failed() {
		t.Logf("%d fixed upstream helper fixtures passed", len(fixtures))
	}
}

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"sync"
	"testing"
)

func TestCLIUpstreamParity(t *testing.T) {
	body, e := os.ReadFile("../../testdata/cli-parity.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixtures []struct {
		Kind, Body, Format string
		Pretty             bool
		Tokens, Args       []string
		Options            map[string]any
		Calls, Expected    json.RawMessage
	}
	if e = json.Unmarshal(body, &fixtures); e != nil {
		t.Fatal(e)
	}
	for i, f := range fixtures {
		t.Run(fmt.Sprintf("%s/%d", f.Kind, i), func(t *testing.T) {
			switch f.Kind {
			case "dispatch":
				c, pos, ok := find(f.Tokens)
				if !ok {
					t.Fatal(f.Tokens)
				}
				var mu sync.Mutex
				calls := []map[string]any{}
				invoke := func(ctx context.Context, path string, args []any) (any, error) {
					mu.Lock()
					calls = append(calls, map[string]any{"path": path, "args": args})
					mu.Unlock()
					return []any{}, nil
				}
				_, e := dispatch(context.Background(), invoke, c, pos, f.Options, &bytes.Buffer{})
				if e != nil {
					t.Fatal(f.Tokens, e)
				}
				sort.Slice(calls, func(i, j int) bool { return calls[i]["path"].(string) < calls[j]["path"].(string) })
				encoded, _ := json.Marshal(calls)
				var got, want any
				json.Unmarshal(encoded, &got)
				json.Unmarshal(f.Calls, &want)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("tokens %v\ngot  %s\nwant %s", f.Tokens, encoded, f.Calls)
				}
			case "parse":
				got := Parse(f.Args, booleanFlags())
				encoded, _ := json.Marshal(got)
				var actual, expected any
				json.Unmarshal(encoded, &actual)
				json.Unmarshal(f.Expected, &expected)
				if !reflect.DeepEqual(actual, expected) {
					t.Fatal(string(encoded), string(f.Expected))
				}
			case "format":
				got, e := FormatJSON([]byte(f.Body), f.Format, f.Pretty)
				if e != nil {
					t.Fatal(e)
				}
				var want string
				json.Unmarshal(f.Expected, &want)
				if got != want {
					t.Fatalf("got %q\nwant%q", got, want)
				}
			}
		})
	}
}

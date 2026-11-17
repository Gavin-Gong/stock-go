package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func testServer(t *testing.T, options Options) *Server {
	t.Helper()
	if options.LookupEnv == nil {
		options.LookupEnv = func(string) string { return "" }
	}
	s, e := New(options)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func semantic(value any) any {
	body, _ := encode(value)
	var out any
	json.Unmarshal(body, &out)
	normalize(out)
	return out
}
func normalize(v any) {
	switch x := v.(type) {
	case map[string]any:
		if x["type"] == "text" {
			if text, ok := x["text"].(string); ok {
				var parsed any
				if json.Unmarshal([]byte(text), &parsed) == nil {
					x["text"] = parsed
				}
			}
		}
		for _, value := range x {
			normalize(value)
		}
	case []any:
		for _, value := range x {
			normalize(value)
		}
	}
}
func TestMCPUpstreamParity(t *testing.T) {
	body, e := os.ReadFile("../testdata/mcp-parity.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixtures []struct {
		Kind, Name string
		Value      any
		Args       map[string]any
		Message    json.RawMessage
		Options    struct {
			Code, Message, Provider, URL string
			Status                       int
		}
		Calls, Expected json.RawMessage
	}
	if e = json.Unmarshal(body, &fixtures); e != nil {
		t.Fatal(e)
	}
	for i, f := range fixtures {
		t.Run(fmt.Sprintf("%s/%s/%d", f.Kind, f.Name, i), func(t *testing.T) {
			s := testServer(t, Options{ToolSet: "full", PromptSet: "full"})
			var actual any
			switch f.Kind {
			case "tools":
				actual = s.Dispatch(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)).Result.(map[string]any)["tools"]
			case "prompts":
				actual = s.Dispatch(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`)).Result.(map[string]any)["prompts"]
			case "call":
				calls := []map[string]any{}
				s.invoke = func(ctx context.Context, path string, args []any) (any, error) {
					calls = append(calls, map[string]any{"path": path, "args": args})
					return []any{}, nil
				}
				raw, _ := encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": f.Name, "arguments": f.Args}})
				response := s.Dispatch(context.Background(), raw)
				if response == nil || response.Error != nil {
					t.Fatal(response)
				}
				var wantCalls any
				json.Unmarshal(f.Calls, &wantCalls)
				if !reflect.DeepEqual(semantic(calls), wantCalls) {
					t.Fatalf("got calls %v\nwant %s", semantic(calls), f.Calls)
				}
				actual = response.Result
			case "prompt", "rpc":
				actual = s.Dispatch(context.Background(), f.Message)
			case "version":
				actual = NegotiateProtocolVersion(f.Value)
			case "result":
				actual = ToToolResult(f.Value)
			case "error":
				actual = ToolErrorResult(&sdkerrors.Error{Code: sdkerrors.Code(f.Options.Code), Message: f.Options.Message, Provider: f.Options.Provider, URL: f.Options.URL, Status: f.Options.Status})
			}
			var expected any
			json.Unmarshal(f.Expected, &expected)
			normalize(expected)
			if !reflect.DeepEqual(semantic(actual), expected) {
				got, _ := encode(actual)
				t.Fatalf("got %s\nwant %s", got, f.Expected)
			}
		})
	}
}

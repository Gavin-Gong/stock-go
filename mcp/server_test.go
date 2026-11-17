package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	stockgo "github.com/Gavin-Gong/stock-go"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestCatalogFilteringEnvironmentAndContracts(t *testing.T) {
	for _, test := range []struct {
		o              Options
		tools, prompts int
	}{{Options{}, 26, 4}, {Options{ToolSet: "FULL", PromptSet: "full"}, 93, 7}, {Options{Tools: []string{"search", "missing"}, Prompts: []string{"analyze_stock"}}, 1, 1}, {Options{LookupEnv: func(k string) string {
		if strings.HasSuffix(k, "TOOLS") {
			return "FULL"
		}
		return ""
	}}, 93, 4}, {Options{Tools: []string{}, LookupEnv: func(string) string { return "full" }}, 26, 7}, {Options{ToolSet: "search, get_market_status"}, 2, 4}} {
		s := testServer(t, test.o)
		if len(s.tools) != test.tools || len(s.prompts) != test.prompts {
			t.Fatal(len(s.tools), len(s.prompts))
		}
		if s.Warnings() == nil {
			t.Fatal("nil warnings")
		}
		if len(s.Warnings()) > 0 {
			warnings := s.Warnings()
			warnings[0] = "mutated"
			if s.Warnings()[0] == "mutated" {
				t.Fatal("warnings not copied")
			}
		}
	}
	all := testServer(t, Options{ToolSet: "full", PromptSet: "full"})
	core := testServer(t, Options{})
	if len(core.warnings) != 0 {
		t.Fatal(core.warnings)
	}
	for _, p := range all.prompts {
		for _, name := range p.UsesTools {
			if _, ok := all.toolMap[name]; !ok {
				t.Fatal(p.Name, name)
			}
		}
	}
	for _, o := range []Options{{MaxLineBytes: -1}, {MaxConcurrent: -1}, {MaxPending: -1}, {MaxLineBytes: 65 << 20}, {MaxConcurrent: 1025}, {MaxPending: 65537}, {SDKOptions: &stockgo.Options{Timeout: -1}}} {
		if _, e := New(o); e == nil {
			t.Fatal(o)
		}
	}
	if _, e := New(Options{}, Options{}); e == nil {
		t.Fatal("multiple options")
	}
	for key, want := range map[string]string{"STOCK_GO_MCP_TOOLS": "go", "STOCK_SDK_MCP_TOOLS": "legacy"} {
		v := envValue(func(k string) string {
			if k == key {
				return want
			}
			return ""
		}, "TOOLS")
		if v != want {
			t.Fatal(key, v)
		}
	}
	if set, names := filter("", nil, ", ,"); set != "core" || names != nil {
		t.Fatal(set, names)
	}
}
func TestProtocolBoundariesNotificationsAndIDs(t *testing.T) {
	s := testServer(t, Options{})
	cases := []struct {
		raw  string
		code int
	}{{`invalid`, ParseError}, {`[]`, InvalidRequest}, {`null`, InvalidRequest}, {`42`, InvalidRequest}, {`{}`, InvalidRequest}, {`{"jsonrpc":"1.0","method":"ping"}`, InvalidRequest}, {`{"jsonrpc":"2.0","id":true,"method":"ping"}`, InvalidRequest}, {`{"jsonrpc":"2.0","id":{},"method":"ping"}`, InvalidRequest}, {`{"jsonrpc":"2.0","id":1,"method":"unknown"}`, MethodNotFound}, {`{"jsonrpc":"2.0","id":1,"method":"ping","params":[]}`, InvalidParams}, {`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":5}}`, InvalidParams}, {`{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"missing"}}`, InvalidParams}}
	for _, c := range cases {
		r := s.Dispatch(context.Background(), []byte(c.raw))
		if r == nil || r.Error == nil || r.Error.Code != c.code {
			t.Fatal(c.raw, r)
		}
	}
	for _, method := range []string{"ping", "initialize", "tools/list", "tools/call", "unknown", "notifications/initialized", "notifications/cancelled"} {
		raw, _ := encode(map[string]any{"jsonrpc": "2.0", "method": method})
		if r := s.Dispatch(context.Background(), raw); r != nil {
			t.Fatal("notification generated response", method, r)
		}
	}
	for _, id := range []string{`0`, `"string"`, `null`, `9007199254740993`, `1e400`} {
		raw := []byte(`{"jsonrpc":"2.0","id":` + id + `,"method":"ping"}`)
		r := s.Dispatch(context.Background(), raw)
		if r == nil || string(r.ID) != id || r.Error != nil {
			t.Fatal(r)
		}
	}
	for _, v := range []any{nil, "unsupported", 7, map[string]any{}} {
		if NegotiateProtocolVersion(v) != LatestProtocolVersion {
			t.Fatal(v)
		}
	}
	r := s.Dispatch(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`))
	result := r.Result.(map[string]any)
	if result["protocolVersion"] != "2025-06-18" || result["serverInfo"].(map[string]any)["name"] != "stock-go" {
		t.Fatal(r)
	}
}
func TestSchemaValidationAndNoInvocation(t *testing.T) {
	calls := 0
	s := testServer(t, Options{ToolSet: "full", Invoke: func(context.Context, string, []any) (any, error) { calls++; return nil, nil }})
	cases := []struct {
		name string
		args any
	}{{"search", map[string]any{}}, {"search", map[string]any{"keyword": 1}}, {"search", map[string]any{"keyword": "word", "unknown": true}}, {"get_a_share_quotes", map[string]any{"codes": "600519"}}, {"get_a_share_quotes", map[string]any{"codes": []any{1}}}, {"get_kline_with_indicators", map[string]any{"symbol": "600519", "market": "JP"}}, {"get_kline_with_indicators", map[string]any{"symbol": "600519", "indicators": map[string]any{"bad": true}}}, {"get_kline_with_indicators", map[string]any{"symbol": "600519", "indicators": map[string]any{"ma": "bad"}}}, {"get_stock_changes", map[string]any{"page": 1.5}}, {"get_stock_changes", map[string]any{"page": true}}, {"get_unusual_fluctuation", map[string]any{"triggered": "true"}}}
	for _, c := range cases {
		raw, _ := encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": c.name, "arguments": c.args}})
		r := s.Dispatch(context.Background(), raw)
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		result := r.Result.(ToolResult)
		if !result.IsError || result.Meta["code"] != "INVALID_ARGUMENT" {
			t.Fatal(c.name, result)
		}
	}
	if calls != 0 {
		t.Fatal("invalid args executed", calls)
	}
	if e := validate(map[string]any{"type": "number"}, "x", "number"); e == nil {
		t.Fatal("invalid number")
	}
	if e := validate(map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}, []any{1.5}, "array"); e == nil {
		t.Fatal("invalid nested item")
	}
}
func TestToolErrorsSerializationAndPromptSubstitution(t *testing.T) {
	s := testServer(t, Options{Invoke: func(context.Context, string, []any) (any, error) {
		return nil, &sdkerrors.Error{Code: sdkerrors.Timeout, Message: "timeout", Provider: "tencent"}
	}})
	r := s.Dispatch(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"keyword":"word"}}}`))
	result := r.Result.(ToolResult)
	if !result.IsError || result.Meta["provider"] != "tencent" || r.Error != nil {
		t.Fatal(r)
	}
	s.invoke = func(context.Context, string, []any) (any, error) { panic("test") }
	r = s.Dispatch(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"keyword":"word"}}}`))
	if r.Error == nil || r.Error.Code != InternalError {
		t.Fatal(r)
	}
	for _, data := range []any{math.NaN(), func() {}} {
		if !ToToolResult(data).IsError {
			t.Fatal("invalid result not marked as error")
		}
	}
	if !ToolErrorResult(nil).IsError {
		t.Fatal("nil error")
	}
	if utf16Length([]byte("中😀")) != 3 {
		t.Fatal("UTF-16 count")
	}
	if r := ToToolResult([201]int{}); !strings.Contains(r.Content[0].Text, `"truncated":true`) {
		t.Fatal(r)
	}
	large := make([]string, 201)
	for i := range large {
		large[i] = strings.Repeat("x", 1000)
	}
	if r := ToToolResult(large); r.Meta["truncated"] != true || !strings.Contains(r.Content[0].Text, "oversized") {
		t.Fatal(r)
	}
	p := s.promptMap["analyze_stock"]
	text, e := p.render(map[string]any{"symbol": "__STOCK_GO_ARG_period__", "period": "weekly"})
	if e != nil || !strings.Contains(text, "analysis of __STOCK_GO_ARG_period__.") {
		t.Fatal(text, e)
	}
	for _, v := range []any{nil, ""} {
		if _, e = p.render(map[string]any{"symbol": v}); e == nil {
			t.Fatal(v)
		}
	}
	if v := jsString([]any{1.0, nil, true, map[string]any{}}); v != "1,,true,[object Object]" {
		t.Fatal(v)
	}
}

type channelWriter struct{ messages chan []byte }

func (w channelWriter) Write(b []byte) (int, error) {
	w.messages <- append([]byte{}, b...)
	return len(b), nil
}
func TestServeCancellationDuplicateAndEOFDrain(t *testing.T) {
	started := make(chan struct{})
	s := testServer(t, Options{MaxConcurrent: 1, MaxPending: 2, Invoke: func(ctx context.Context, path string, args []any) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, &sdkerrors.Error{Code: sdkerrors.Aborted, Message: "canceled"}
	}})
	reader, writer := io.Pipe()
	output := channelWriter{make(chan []byte, 8)}
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background(), reader, output, io.Discard) }()
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":"query","method":"tools/call","params":{"name":"search","arguments":{"keyword":"word"}}}`)
	<-started
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":"query","method":"ping"}`)
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"query"}}`)
	writer.Close()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if len(output.messages) != 3 {
		t.Fatal(len(output.messages))
	}
	codes := map[string]int{}
	for len(output.messages) > 0 {
		var r map[string]any
		json.Unmarshal(<-output.messages, &r)
		if errorValue, ok := r["error"].(map[string]any); ok {
			codes["duplicate"] = int(errorValue["code"].(float64))
		} else if result, ok := r["result"].(map[string]any); ok && result["isError"] == true {
			codes["canceled"]++
		} else {
			codes["ping"]++
		}
	}
	if codes["duplicate"] != InvalidRequest || codes["canceled"] != 1 || codes["ping"] != 1 {
		t.Fatal(codes)
	}
}
func TestServeBoundedConcurrencyAndOverload(t *testing.T) {
	var active, peak atomic.Int32
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	s := testServer(t, Options{MaxConcurrent: 3, MaxPending: 12, Invoke: func(context.Context, string, []any) (any, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return []any{}, nil
	}})
	var input strings.Builder
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&input, "{\"jsonrpc\":\"2.0\",\"id\":%d,\"method\":\"tools/call\",\"params\":{\"name\":\"search\",\"arguments\":{\"keyword\":\"word\"}}}\n", i)
	}
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background(), strings.NewReader(input.String()), &output, io.Discard) }()
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if peak.Load() != 3 || len(strings.Split(strings.TrimSpace(output.String()), "\n")) != 12 {
		t.Fatal(peak.Load(), output.String())
	}
}
func TestReadLinesLimitsRecoveryAndPartialEOF(t *testing.T) {
	for _, input := range []string{strings.Repeat("x", 120000) + "\n{\"ok\":true}\n", strings.Repeat("x", 120000)} {
		lines := []string{}
		large := 0
		e := readLines(strings.NewReader(input), 100, func(line []byte) error { lines = append(lines, string(line)); return nil }, func() error { large++; return nil })
		if e != nil || large != 1 {
			t.Fatal(e, large)
		}
		if strings.Contains(input, "ok") && !reflect.DeepEqual(lines, []string{`{"ok":true}`}) {
			t.Fatal(lines)
		}
	}
	lines := []string{}
	e := readLines(strings.NewReader("\n \r\nping\r\nlast"), 100, func(line []byte) error { lines = append(lines, string(line)); return nil }, func() error { return nil })
	if e != nil || !reflect.DeepEqual(lines, []string{"ping", "last"}) {
		t.Fatal(lines, e)
	}
	s := testServer(t, Options{MaxLineBytes: 100})
	var output, log bytes.Buffer
	e = s.Serve(context.Background(), strings.NewReader(strings.Repeat("x", 101)+"\n"+`{"jsonrpc":"2.0","id":1,"method":"ping"}`), &output, &log)
	if e != nil || !strings.Contains(output.String(), "Message too large") || !strings.Contains(output.String(), `"id":1`) {
		t.Fatal(output.String(), e)
	}
	if strings.Contains(output.String(), "ready") || !strings.Contains(log.String(), "ready") {
		t.Fatal(output.String(), log.String())
	}
}

type errorWriter struct {
	err   error
	short bool
}

func (w errorWriter) Write(b []byte) (int, error) {
	if w.short {
		return 0, nil
	}
	return 0, w.err
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("input failed") }
func TestTransportErrorsAndContextCleanup(t *testing.T) {
	s := testServer(t, Options{})
	if e := s.Serve(context.Background(), nil, io.Discard, nil); e == nil {
		t.Fatal("nil streams")
	}
	for _, w := range []errorWriter{{err: syscall.EPIPE}, {err: io.ErrClosedPipe}, {short: true}} {
		e := s.Serve(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`), w, nil)
		if w.err == syscall.EPIPE {
			if e != nil {
				t.Fatal(e)
			}
		} else if e == nil {
			t.Fatal("writer failure lost")
		}
	}
	if e := s.Serve(context.Background(), errorReader{}, io.Discard, nil); e == nil {
		t.Fatal("input error lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	defer writer.Close()
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, reader, io.Discard, nil) }()
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("reader was not closed on cancellation")
	}
	for _, id := range []string{`1`, `1.0`, `1e0`} {
		if idKey(json.RawMessage(id)) != "number:1" {
			t.Fatal(id)
		}
	}
	for _, id := range []string{`1e999999999`, `1e-999999999`, strings.Repeat("1", 200)} {
		if idKey(json.RawMessage(id)) == "" {
			t.Fatal(id)
		}
	}
}
func FuzzDispatch(f *testing.F) {
	for _, raw := range []string{`{"jsonrpc":"2.0","id":1,"method":"ping"}`, `[]`, `null`, `{"jsonrpc":"2.0","id":{},"method":"tools/call"}`} {
		f.Add(raw)
	}
	s, _ := New(Options{LookupEnv: func(string) string { return "" }, Invoke: func(context.Context, string, []any) (any, error) { return nil, nil }})
	f.Fuzz(func(t *testing.T, line string) {
		r := s.Dispatch(context.Background(), []byte(line))
		if r != nil {
			if body, e := encode(r); e != nil || !json.Valid(body) {
				t.Fatal(string(body), e)
			}
		}
	})
}

func TestServeOverloadAndNotificationFastPath(t *testing.T) {
	started := make(chan struct{})
	s := testServer(t, Options{MaxConcurrent: 1, MaxPending: 1, Invoke: func(ctx context.Context, path string, args []any) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, &sdkerrors.Error{Code: sdkerrors.Aborted, Message: "canceled"}
	}})
	reader, writer := io.Pipe()
	output := channelWriter{make(chan []byte, 8)}
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background(), reader, output, nil) }()
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"keyword":"word"}}}`)
	<-started
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":3,"method":"ping"}`)
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","method":"ping"}`)
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1.0}}`)
	writer.Close()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if len(output.messages) != 3 {
		t.Fatal("notification generated response", len(output.messages))
	}
	busy := false
	for len(output.messages) > 0 {
		var r Response
		json.Unmarshal(<-output.messages, &r)
		if string(r.ID) == "3" {
			busy = r.Error != nil && r.Error.Code == InternalError && strings.Contains(r.Error.Message, "busy")
		}
	}
	if !busy {
		t.Fatal("queue overload wasn't reported")
	}
}
func TestRuntimeEntryAndDefaultOptionArgumentMapping(t *testing.T) {
	var out bytes.Buffer
	if e := Start(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`), &out, nil, Options{LookupEnv: func(string) string { return "" }}); e != nil || !json.Valid(bytes.TrimSpace(out.Bytes())) {
		t.Fatal(out.String(), e)
	}
	calls := []string{}
	s := testServer(t, Options{ToolSet: "full", Invoke: func(ctx context.Context, path string, args []any) (any, error) {
		calls = append(calls, path)
		if path == "northbound.history" {
			if len(args) != 2 || args[0] != nil {
				t.Error(args)
			}
		}
		return []any{}, nil
	}})
	for _, name := range []string{"get_northbound_history", "get_stock_changes"} {
		raw, _ := encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name}})
		r := s.Dispatch(context.Background(), raw)
		if r.Error != nil || r.Result.(ToolResult).IsError {
			t.Fatal(r)
		}
	}
	if len(calls) != 2 {
		t.Fatal(calls)
	}
	for _, raw := range []string{"1000", "invalid", "-1", "1s", "Inf", "1e100"} {
		sdk := stockgo.Options{}
		if raw == "1000" {
			sdk.Timeout = time.Second
		}
		_, e := New(Options{SDKOptions: &sdk, LookupEnv: func(string) string { return raw }})
		if e != nil {
			t.Fatal(raw, e)
		}
	}
}

func TestCatalogResponseDoesNotMutateValidation(t *testing.T) {
	s := testServer(t, Options{})
	r := s.Dispatch(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	tools := r.Result.(map[string]any)["tools"].([]Tool)
	name := tools[0].Name
	tools[0].Name = "mutated"
	tools[0].InputSchema["additionalProperties"] = true
	if s.toolMap[name].InputSchema["additionalProperties"] != false || s.tools[0].Name == "mutated" {
		t.Fatal("catalog response modified live validation")
	}
}

func TestCancellationUnblocksOutputPipe(t *testing.T) {
	s := testServer(t, Options{})
	outReader, outWriter := io.Pipe()
	defer outReader.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.Serve(ctx, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), outWriter, nil)
	}()
	time.Sleep(5 * time.Millisecond)
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("output writer wasn't closed on cancellation")
	}
}

func TestSDKConfigurationEnvironmentPrecedence(t *testing.T) {
	for _, value := range []string{"1000", "invalid", "-1", "1s", "Inf", "1e100"} {
		s := testServer(t, Options{LookupEnv: func(key string) string {
			if strings.HasSuffix(key, "TIMEOUT") {
				return value
			}
			return ""
		}})
		want := 15 * time.Second
		if value == "1000" {
			want = time.Second
		}
		if s.sdk.Requests.Timeout() != want {
			t.Fatal(value, s.sdk.Requests.Timeout(), want)
		}
	}
	s := testServer(t, Options{SDKOptions: &stockgo.Options{Timeout: 2 * time.Second}, LookupEnv: func(string) string { return "1000" }})
	if s.sdk.Requests.Timeout() != 2*time.Second {
		t.Fatal(s.sdk.Requests.Timeout())
	}
	sdk, e := stockgo.New()
	if e != nil {
		t.Fatal(e)
	}
	s = testServer(t, Options{SDK: sdk, SDKOptions: &stockgo.Options{Timeout: -1}})
	if s.sdk != sdk {
		t.Fatal("existing SDK not reused")
	}
}

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	stockgo "github.com/Gavin-Gong/stock-go"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func runTest(args []string, fn Invoke) (int, string, string) {
	var out, err bytes.Buffer
	code := Run(context.Background(), args, Config{Stdout: &out, Stderr: &err, Invoke: fn, Version: "test-version"})
	return code, out.String(), err.String()
}
func TestHelpVersionAndNoNetwork(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-V"}, {"kline", "cn", "--help"}, {"options", "etf", "dailyKline", "--help"}, {"mcp", "--help"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			code, out, err := runTest(args, func(context.Context, string, []any) (any, error) { t.Fatal("help attempted a query"); return nil, nil })
			if code != 0 || out == "" || err != "" {
				t.Fatal(code, out, err)
			}
		})
	}
	code, out, err := runTest([]string{"--version"}, nil)
	if code != 0 || out != "test-version\n" || err != "" {
		t.Fatal(code, out, err)
	}
}
func TestValidationStopsBeforeInvocation(t *testing.T) {
	for _, args := range [][]string{{"missing"}, {"quote"}, {"quote", "600519", "--market", "bad"}, {"quote", "sh600519", "--market", "hk"}, {"quote", "930903"}, {"kline", "GDAXI"}, {"kline", "600519", "--period", "bad"}, {"kline", "600519", "--start"}, {"kline", "600519", "--wat", "yes"}, {"codes", "us", "--board", "sh"}, {"codes", "bad"}, {"minute", "600519", "--ndays", "3"}, {"search", "word", "extra"}, {"calendar", "marketStatus", "JP"}, {"dragonTiger", "detail", "--start", "20240101"}, {"futures", "globalSpot", "--pageSize", "NaN"}, {"call", "client.get"}, {"call", "constructor"}, {"call", "search", "--args", "invalid"}, {"call", "search", "--args"}, {"quote", "600519", "--limit", "invalid"}, {"quote", "600519", "--format", "bad"}, {"quote", "600519", "--timeout"}, {"quote", "600519", "--timeout", "0"}, {"quote", "600519", "--timeout", "NaN"}, {"quote", "600519", "--timeout", "1e100"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			calls := 0
			code, out, err := runTest(args, func(context.Context, string, []any) (any, error) { calls++; return nil, nil })
			if code != 2 || out != "" || err == "" || calls != 0 {
				t.Fatal(code, out, err, calls)
			}
		})
	}
}
func TestOutputLimitPrettyQuietAndCSV(t *testing.T) {
	fn := func(context.Context, string, []any) (any, error) {
		return []map[string]any{{"code": "600519", "name": "茅台"}, {"code": "000001", "name": "平安银行"}}, nil
	}
	for _, args := range [][]string{{"search", "word", "--limit", "1"}, {"search", "word", "--pretty"}, {"search", "word", "-f", "csv"}, {"search", "word", "--format=table"}, {"search", "word", "--limit", "-5"}} {
		code, out, err := runTest(args, fn)
		if code != 0 || out == "" || err != "" {
			t.Fatal(code, out, err)
		}
		if contains(args, "--pretty") && !strings.Contains(out, "\n  {") {
			t.Fatal(out)
		}
		if contains(args, "1") {
			var rows []any
			json.Unmarshal([]byte(out), &rows)
			if len(rows) != 1 {
				t.Fatal(out)
			}
		}
	}
	for _, value := range []any{nil, []string{}, (*string)(nil)} {
		fn := func(context.Context, string, []any) (any, error) { return value, nil }
		code, _, err := runTest([]string{"search", "word"}, fn)
		if code != 0 || !strings.Contains(err, "无数据") {
			t.Fatal(code, err)
		}
		code, _, err = runTest([]string{"search", "word", "-q"}, fn)
		if code != 0 || err != "" {
			t.Fatal(code, err)
		}
	}
	code, _, err := runTest([]string{"search", "word"}, func(context.Context, string, []any) (any, error) { return math.NaN(), nil })
	if code != 1 || err == "" {
		t.Fatal(code, err)
	}
}
func TestMixedMarketPartialFailureAndOrdering(t *testing.T) {
	var mu sync.Mutex
	calls := map[string][]any{}
	fn := func(ctx context.Context, path string, args []any) (any, error) {
		mu.Lock()
		calls[path] = args
		mu.Unlock()
		if path == "quotes.hk" {
			return nil, &sdkerrors.Error{Code: sdkerrors.Network, Message: "HK failed"}
		}
		return []string{path}, nil
	}
	code, out, err := runTest([]string{"quote", "600519", "00700", "AAPL"}, fn)
	if code != 0 || !strings.Contains(err, "部分市场") || !strings.Contains(out, `["quotes.cnSimple","quotes.us"]`) {
		t.Fatal(code, out, err)
	}
	if !reflect.DeepEqual(calls["quotes.cnSimple"], []any{[]string{"sh600519"}}) {
		t.Fatal(calls)
	}
	code, out, err = runTest([]string{"quote", "00700"}, fn)
	if code != 3 || out != "" || !strings.Contains(err, "NETWORK_ERROR") {
		t.Fatal(code, out, err)
	}
	code, _, err = runTest([]string{"quote", "hkHSI"}, func(ctx context.Context, path string, args []any) (any, error) {
		if path != "quotes.hk" {
			t.Error(path)
		}
		return []any{}, nil
	})
	if code != 0 || !strings.Contains(err, "无数据") {
		t.Fatal(code, err)
	}
	code, _, err = runTest([]string{"quote", "@@"}, func(ctx context.Context, path string, args []any) (any, error) {
		if path != "quotes.cnSimple" {
			t.Error(path)
		}
		return []string{}, nil
	})
	if code != 0 {
		t.Fatal(code, err)
	}
}
func TestExitCodesAndErrorRendering(t *testing.T) {
	for code, want := range map[sdkerrors.Code]int{sdkerrors.InvalidArgument: 2, sdkerrors.InvalidSymbol: 2, sdkerrors.NotFound: 2, sdkerrors.Network: 3, sdkerrors.Timeout: 3, sdkerrors.Aborted: 3, sdkerrors.UpstreamError: 4, sdkerrors.UpstreamEmpty: 4, sdkerrors.Parse: 4, sdkerrors.HTTP: 4, sdkerrors.RateLimited: 5, sdkerrors.CircuitOpen: 5, "OTHER": 1} {
		fn := func(context.Context, string, []any) (any, error) {
			return nil, &sdkerrors.Error{Code: code, Message: "failure"}
		}
		got, out, err := runTest([]string{"search", "word"}, fn)
		if got != want || out != "" || !strings.Contains(err, `"message":"failure"`) {
			t.Fatal(got, want, out, err)
		}
		got, _, err = runTest([]string{"search", "word", "-f", "table"}, fn)
		if got != want || !strings.Contains(err, "stock-go:") {
			t.Fatal(got, err)
		}
	}
	if ExitCode(errors.New("fail")) != 1 {
		t.Fatal("generic exit code")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
func TestWriterFailuresAndMCPDelegation(t *testing.T) {
	fn := func(context.Context, string, []any) (any, error) { return []int{1}, nil }
	for _, writeError := range []error{syscall.EPIPE, io.ErrClosedPipe} {
		code := Run(context.Background(), []string{"search", "word"}, Config{Invoke: fn, Stdout: failingWriter{writeError}})
		want := 1
		if writeError == syscall.EPIPE {
			want = 0
		}
		if code != want {
			t.Fatal(code, want)
		}
	}
	for _, args := range [][]string{nil, {"--version"}, {"search", "--help"}, {"mcp", "--help"}} {
		if code := Run(context.Background(), args, Config{Stdout: failingWriter{syscall.EPIPE}}); code != 0 {
			t.Fatal(code)
		}
	}
	var out bytes.Buffer
	called := false
	code := Run(context.Background(), []string{"mcp"}, Config{Stdout: &out, MCP: func(ctx context.Context, in io.Reader, out, err io.Writer) error { called = true; return nil }})
	if code != 0 || !called {
		t.Fatal(code, called)
	}
	code = Run(context.Background(), []string{"mcp"}, Config{})
	if code != 1 {
		t.Fatal(code)
	}
}
func TestRunnerFactoryTimeoutAndRealSDK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=s_sh600519") {
			t.Error(r.URL)
		}
		w.Write([]byte(`v_s_sh600519="51~name~600519~10~9~9~1~100";`))
	}))
	defer srv.Close()
	var out, err bytes.Buffer
	var timeout time.Duration
	code := Run(context.Background(), []string{"quotes", "cnSimple", "600519", "--timeout", "2s"}, Config{Stdout: &out, Stderr: &err, NewSDK: func(o stockgo.Options) (*stockgo.SDK, error) {
		timeout = o.Timeout
		o.TencentURL = srv.URL
		return stockgo.New(o)
	}})
	if code != 0 || timeout != 2*time.Second || !strings.Contains(out.String(), "600519") {
		t.Fatal(code, timeout, out.String(), err.String())
	}
	for _, raw := range []any{nil, "1000", "1ms", "0.0000001", "-1", "bad", true, "Inf"} {
		d, e := timeoutValue(raw)
		if raw == nil {
			if d != 0 || e != nil {
				t.Fatal(d, e)
			}
		} else if raw == "1000" || raw == "1ms" {
			if d <= 0 || e != nil {
				t.Fatal(raw, d, e)
			}
		} else if e == nil {
			t.Fatal(raw, d)
		}
	}
	code = Run(context.Background(), []string{"search", "word"}, Config{NewSDK: func(stockgo.Options) (*stockgo.SDK, error) { return nil, errors.New("factory") }})
	if code != 1 {
		t.Fatal(code)
	}
}
func FuzzParse(f *testing.F) {
	f.Add("quote\x00600519\x00--full")
	f.Add("--\x00--x")
	f.Fuzz(func(t *testing.T, text string) {
		parts := strings.Split(text, "\x00")
		parsed := Parse(parts, booleanFlags())
		if parsed.Positional == nil || parsed.Options == nil {
			t.Fatal("nil parser result")
		}
	})
}

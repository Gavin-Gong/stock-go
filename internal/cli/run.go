// Package cli implements the stock-go command line without process-global state.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"syscall"
	"time"

	stockgo "github.com/Gavin-Gong/stock-go"
	"github.com/Gavin-Gong/stock-go/internal/operations"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type Config struct {
	Stdout, Stderr io.Writer
	Stdin          io.Reader
	NewSDK         func(stockgo.Options) (*stockgo.SDK, error)
	Invoke         Invoke
	Version        string
	MCP            func(context.Context, io.Reader, io.Writer, io.Writer) error
}

func ExitCode(e error) int {
	var u *usageError
	if errors.As(e, &u) {
		return 2
	}
	switch sdkerrors.CodeOf(e) {
	case sdkerrors.InvalidArgument, sdkerrors.InvalidSymbol, sdkerrors.NotFound:
		return 2
	case sdkerrors.Network, sdkerrors.Timeout, sdkerrors.Aborted:
		return 3
	case sdkerrors.UpstreamError, sdkerrors.UpstreamEmpty, sdkerrors.Parse, sdkerrors.HTTP:
		return 4
	case sdkerrors.RateLimited, sdkerrors.CircuitOpen:
		return 5
	}
	return 1
}
func renderError(e error, format string) string {
	code := string(sdkerrors.CodeOf(e))
	var u *usageError
	if errors.As(e, &u) {
		code = "USAGE"
	}
	if code == "" {
		code = "UNKNOWN"
	}
	message := e.Error()
	var sdkError *sdkerrors.Error
	if errors.As(e, &sdkError) {
		message = sdkError.Message
	}
	if format == "json" {
		body, _ := encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
		return string(body)
	}
	return "stock-go: " + code + ": " + message
}
func timeoutValue(v any) (time.Duration, error) {
	if v == nil {
		return 0, nil
	}
	if v == true {
		return 0, usage("--timeout 缺少值")
	}
	text := fmt.Sprint(last(v))
	if n, e := strconv.ParseFloat(text, 64); e == nil {
		if n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) || n >= float64(math.MaxInt64)/float64(time.Millisecond) {
			return 0, usage("--timeout 需要正数毫秒")
		}
		d := time.Duration(n * float64(time.Millisecond))
		if d <= 0 {
			return 0, usage("--timeout 太小")
		}
		return d, nil
	}
	d, e := time.ParseDuration(text)
	if e != nil || d <= 0 {
		return 0, usage("--timeout 需要正数毫秒或 Go duration")
	}
	return d, nil
}
func Run(ctx context.Context, args []string, cfg Config) int {
	if cfg.Stdout == nil {
		cfg.Stdout = io.Discard
	}
	if cfg.Stderr == nil {
		cfg.Stderr = io.Discard
	}
	format := "json"
	fail := func(e error) int {
		if errors.Is(e, syscall.EPIPE) {
			return 0
		}
		_ = writeLine(cfg.Stderr, renderError(e, format))
		return ExitCode(e)
	}
	parsed := Parse(args, booleanFlags())
	raw := parsed.Options
	if v := raw["format"]; v != nil {
		format = fmt.Sprint(last(v))
	} else if v := raw["f"]; v != nil {
		format = fmt.Sprint(last(v))
	}
	if !contains([]string{"json", "csv", "table"}, format) {
		format = "json"
		return fail(usage("--format 可选 json / table / csv"))
	}
	if boolValue(last(raw["version"])) || boolValue(last(raw["V"])) {
		v := cfg.Version
		if v == "" {
			v = "dev"
		}
		if e := writeLine(cfg.Stdout, v); e != nil {
			return fail(e)
		}
		return 0
	}
	if len(parsed.Positional) == 0 {
		if e := writeLine(cfg.Stdout, help(nil)); e != nil {
			return fail(e)
		}
		return 0
	}
	if parsed.Positional[0] == "mcp" {
		if boolValue(last(raw["help"])) || boolValue(last(raw["h"])) {
			if e := writeLine(cfg.Stdout, "stock-go mcp — MCP stdio server\n用法: stock-go mcp\n环境变量: STOCK_GO_MCP_TOOLS=core|full|工具名单；STOCK_GO_MCP_PROMPTS=core|full|模板名单；STOCK_GO_MCP_TIMEOUT=毫秒"); e != nil {
				return fail(e)
			}
			return 0
		}
		if cfg.MCP == nil {
			return fail(errors.New("MCP server is not available in this build"))
		}
		if e := cfg.MCP(ctx, cfg.Stdin, cfg.Stdout, cfg.Stderr); e != nil {
			return fail(e)
		}
		return 0
	}
	command, rest, ok := find(parsed.Positional)
	if !ok {
		return fail(usage("未知命令: " + fmt.Sprint(parsed.Positional)))
	}
	if boolValue(last(raw["help"])) || boolValue(last(raw["h"])) {
		if e := writeLine(cfg.Stdout, help(&command)); e != nil {
			return fail(e)
		}
		return 0
	}
	timeout, e := timeoutValue(raw["timeout"])
	if e != nil {
		return fail(e)
	}
	limitN := 0.0
	if value := raw["limit"]; value != nil && value != true {
		n, e := strconv.ParseFloat(fmt.Sprint(last(value)), 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return fail(usage("--limit 需要有限数值"))
		}
		limitN = n
	}
	invoke := cfg.Invoke
	if invoke == nil {
		factory := cfg.NewSDK
		if factory == nil {
			factory = func(o stockgo.Options) (*stockgo.SDK, error) { return stockgo.New(o) }
		}
		sdk, e := factory(stockgo.Options{Timeout: timeout})
		if e != nil {
			return fail(e)
		}
		registry, e := operations.New(sdk)
		if e != nil {
			return fail(e)
		}
		invoke = registry.Invoke
	}
	result, e := dispatch(ctx, invoke, command, rest, raw, cfg.Stderr)
	if e != nil {
		return fail(e)
	}
	if limitN >= 1 {
		body, e := encode(result)
		if e != nil {
			return fail(e)
		}
		var items []json.RawMessage
		if json.Unmarshal(body, &items) == nil && items != nil && limitN < float64(len(items)) {
			result = items[:int(limitN)]
		}
	}

	empty := result == nil
	if !empty {
		rv := reflect.ValueOf(result)
		empty = (rv.Kind() == reflect.Slice && rv.Len() == 0) || (rv.Kind() == reflect.Pointer && rv.IsNil())
	}
	if empty && !boolValue(last(raw["quiet"])) && !boolValue(last(raw["q"])) {
		if e := writeLine(cfg.Stderr, "stock-go: 无数据"); e != nil {
			return fail(e)
		}
	}
	output, e := Format(result, format, boolValue(last(raw["pretty"])))
	if e != nil {
		return fail(e)
	}
	if e = writeLine(cfg.Stdout, output); e != nil {
		return fail(e)
	}
	return 0
}

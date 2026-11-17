package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/Gavin-Gong/stock-go/internal/operations"
	"github.com/Gavin-Gong/stock-go/symbols"
)

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }
func usage(message string) error    { return &usageError{message} }
func last(v any) any {
	if values, ok := v.([]string); ok {
		return values[len(values)-1]
	}
	return v
}
func boolValue(v any) bool { return v == true || v == "true" || v == "1" || v == "yes" }
func numberArray(v any) []int {
	text := fmt.Sprint(v)
	if values, ok := v.([]string); ok {
		text = strings.Join(values, ",")
	}
	if values, ok := v.([]any); ok {
		parts := []string{}
		for _, item := range values {
			parts = append(parts, fmt.Sprint(item))
		}
		text = strings.Join(parts, ",")
	}
	out := []int{}
	for _, part := range strings.Split(text, ",") {
		n, e := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if e == nil && n > 0 && n == math.Trunc(n) && n < float64(math.MaxInt) {
			out = append(out, int(n))
		}
	}
	return out
}

var globals = map[string]bool{"format": true, "f": true, "pretty": true, "quiet": true, "q": true, "help": true, "h": true, "version": true, "V": true, "timeout": true, "limit": true}

func normalized(c Command, raw map[string]any) (map[string]any, map[string]any, error) {
	declared := map[string]Option{}
	for _, o := range c.Options {
		declared[o.Flag] = o
	}
	options := map[string]any{}
	flags := map[string]any{}
	for key, raw := range raw {
		if globals[key] {
			continue
		}
		o, ok := declared[key]
		if !ok {
			return nil, nil, usage("未知选项 --" + key)
		}
		value := raw
		if o.Type != "number[]" {
			value = last(raw)
		}
		if value == true && o.Type != "boolean" && o.Type != "number[]" {
			return nil, nil, usage("--" + key + " 缺少值")
		}
		if o.Upper {
			if s, ok := value.(string); ok {
				value = strings.ToUpper(s)
			}
		}
		switch o.Type {
		case "boolean":
			value = boolValue(value)
		case "number":
			n, e := strconv.ParseFloat(fmt.Sprint(value), 64)
			if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return nil, nil, usage("--" + key + " 需要有限数值")
			}
			value = n
		case "number[]":
			if value != true {
				value = numberArray(value)
			}
		case "enum":
			if !contains(o.Enum, fmt.Sprint(value)) {
				return nil, nil, usage("--" + key + " 非法值，可选: " + strings.Join(o.Enum, " / "))
			}
		}
		flags[key] = value
		field := o.Field
		if field == "" {
			field = key
		}
		if s, ok := value.(string); ok && o.Map != nil {
			if mapped, exists := o.Map[s]; exists {
				value = mapped
			}
		}
		options[field] = value
	}
	for _, o := range c.Options {
		if o.Required {
			if _, ok := flags[o.Flag]; !ok {
				return nil, nil, usage("缺少必填选项 --" + o.Flag)
			}
		}
	}
	return options, flags, nil
}
func contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
func positional(c Command, tokens []string) ([]any, error) {
	out := []any{}
	index := 0
	for _, p := range c.Positional {
		if p.Variadic {
			if p.Required && index >= len(tokens) {
				return nil, usage("缺少参数 <" + p.Name + ">")
			}
			for _, v := range tokens[index:] {
				out = append(out, v)
			}
			return out, nil
		}
		if index >= len(tokens) {
			if p.Required {
				return nil, usage("缺少参数 <" + p.Name + ">")
			}
			out = append(out, nil)
			continue
		}
		v := tokens[index]
		if p.Upper {
			v = strings.ToUpper(v)
		}
		if len(p.Enum) > 0 && !contains(p.Enum, v) {
			return nil, usage("<" + p.Name + "> 非法值，可选: " + strings.Join(p.Enum, " / "))
		}
		out = append(out, v)
		index++
	}
	if index < len(tokens) {
		return nil, usage("多余的位置参数: " + strings.Join(tokens[index:], " "))
	}
	return out, nil
}

type Invoke func(context.Context, string, []any) (any, error)

func defaultArguments(c Command, pos []string, opts map[string]any) ([]any, error) {
	switch c.ArgShape {
	case "none":
		if len(pos) > 0 {
			return nil, usage("此命令不接受位置参数")
		}
		return []any{}, nil
	case "codes[]", "codes+options":
		if len(pos) == 0 {
			return nil, usage("需要至少一个代码参数")
		}
		args := []any{pos}
		if c.ArgShape == "codes+options" {
			args = append(args, opts)
		}
		return args, nil
	default:
		args, e := positional(c, pos)
		if e != nil {
			return nil, e
		}
		if c.ArgShape == "symbol+options" || c.ArgShape == "options" {
			args = append(args, opts)
		}
		return args, nil
	}
}
func dispatch(ctx context.Context, invoke Invoke, c Command, pos []string, raw map[string]any, stderr io.Writer) (any, error) {
	opts, flags, e := normalized(c, raw)
	if e != nil {
		return nil, e
	}
	path := strings.Join(c.Path, ".")
	if len(c.Alias) > 0 {
		alias := c.Alias[0]
		switch alias {
		case "call":
			p, e := positional(c, pos)
			if e != nil {
				return nil, e
			}
			path = fmt.Sprint(p[0])
			known := false
			for _, s := range operations.Specs() {
				if strings.Join(s.Path, ".") == path {
					known = true
					break
				}
			}
			if !known {
				return nil, usage("未知方法: " + path)
			}
			args := []any{}
			if raw, ok := flags["args"].(string); ok {
				var value any
				if e := json.Unmarshal([]byte(raw), &value); e != nil {
					return nil, usage("--args 不是合法 JSON")
				}
				if values, ok := value.([]any); ok {
					args = values
				} else {
					args = []any{value}
				}
			}
			return invoke(ctx, path, args)
		case "quote":
			if len(pos) == 0 {
				return nil, usage("缺少股票代码")
			}
			return quote(ctx, invoke, pos, flags, stderr)
		case "kline", "minute":
			p, e := positional(c, pos)
			if e != nil {
				return nil, e
			}
			symbol := p[0].(string)
			market := fmt.Sprint(flags["market"])
			if flags["market"] == nil {
				market, e = marketTag(symbol)
				if e != nil {
					return nil, e
				}
			}
			delete(opts, "market")
			method := map[string]string{"a": "cn", "hk": "hk", "us": "us"}[market]
			if alias == "minute" {
				method += "Minute"
				if _, ok := opts["ndays"]; ok && market == "a" {
					return nil, usage("--ndays 仅支持港/美股分时")
				}
			}
			return invoke(ctx, "kline."+method, []any{symbol, opts})
		case "codes":
			p, e := positional(c, pos)
			if e != nil {
				return nil, e
			}
			market := p[0].(string)
			if flags["board"] != nil && market != "a" {
				return nil, usage("--board 仅支持 A 股")
			}
			method := map[string]string{"a": "cn", "hk": "hk", "us": "us", "fund": "fund"}[market]
			args := []any{}
			if market == "a" || market == "us" {
				args = append(args, opts)
			}
			return invoke(ctx, "codes."+method, args)
		case "ztpool":
			p, e := positional(c, pos)
			if e != nil {
				return nil, e
			}
			return invoke(ctx, path, []any{p[0], flags["date"]})
		}
	}
	if path == "kline.withIndicators" {
		if _, e := positional(c, pos); e != nil {
			return nil, e
		}
		nested := map[string]any{}
		for _, key := range []string{"ma", "rsi", "wr", "bias"} {
			v, ok := flags[key]
			if !ok {
				continue
			}
			if v == true {
				nested[key] = map[string]any{}
			} else if periods, ok := v.([]int); ok && len(periods) > 0 {
				nested[key] = map[string]any{"periods": periods}
			}
			delete(opts, key)
		}
		for _, key := range []string{"macd", "kdj", "boll", "cci", "atr", "obv", "roc", "dmi", "sar", "kc"} {
			if enabled, ok := flags[key].(bool); ok && enabled {
				nested[key] = true
			}
			delete(opts, key)
		}
		opts["indicators"] = nested
	}
	if path == "northbound.minute" || path == "northbound.history" {
		p, e := positional(c, pos)
		if e != nil {
			return nil, e
		}
		direction := flags["direction"]
		if direction == nil && len(p) > 0 {
			direction = p[0]
		}
		delete(opts, "direction")
		args := []any{direction}
		if path == "northbound.history" {
			args = append(args, opts)
		}
		return invoke(ctx, path, args)
	}
	args, e := defaultArguments(c, pos, opts)
	if e != nil {
		return nil, e
	}
	return invoke(ctx, path, args)
}
func marketTag(symbol string) (string, error) {
	ns, e := symbols.Normalize(symbol)
	if e != nil {
		return "a", nil
	}
	switch ns.Market {
	case symbols.HK:
		return "hk", nil
	case symbols.US:
		return "us", nil
	case symbols.Global:
		return "", usage("GLOBAL 市场需显式指定支持的市场及原始 secid")
	}
	return "a", nil
}
func appendItems(out []any, v any) []any {
	if v == nil {
		return out
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return append(out, v)
	}
	for i := 0; i < rv.Len(); i++ {
		out = append(out, rv.Index(i).Interface())
	}
	return out
}
func quote(ctx context.Context, invoke Invoke, codes []string, flags map[string]any, stderr io.Writer) (any, error) {
	groups := map[string][]string{}
	forced, _ := flags["market"].(string)
	for _, code := range codes {
		if forced == "fund" {
			groups[forced] = append(groups[forced], code)
			continue
		}
		tag := forced
		var ns symbols.Symbol
		var e error
		if forced != "" {
			market := map[string]symbols.Market{"a": symbols.CN, "hk": symbols.HK, "us": symbols.US}[forced]
			ns, e = symbols.Normalize(code, symbols.Hint{Market: market})
		} else {
			ns, e = symbols.Normalize(code)
		}
		if e != nil {
			if forced != "" {
				if _, baseError := symbols.Normalize(code); baseError == nil {
					return nil, e
				}
			}
			if tag == "" {
				tag = "a"
			}
			groups[tag] = append(groups[tag], code)
			continue
		}
		if tag == "" {
			tag = "a"
			if ns.Market == symbols.HK {
				tag = "hk"
			} else if ns.Market == symbols.US {
				tag = "us"
			}
		}
		key, e := symbols.ToTencent(ns)
		if e != nil && ns.AssetType == symbols.Index {
			return nil, usage("特殊指数 " + ns.Code + " 无腾讯行情接口，请使用 K 线入口")
		}
		normalized := ns.Code
		if tag == "a" && e == nil {
			normalized = key
		}
		groups[tag] = append(groups[tag], normalized)
	}
	markets := []string{"a", "hk", "us", "fund"}
	results := make([]any, len(markets))
	failures := make([]error, len(markets))
	var wg sync.WaitGroup
	for i, market := range markets {
		if len(groups[market]) == 0 {
			continue
		}
		wg.Add(1)
		go func(i int, market string) {
			defer wg.Done()
			method := map[string]string{"a": "cnSimple", "hk": "hk", "us": "us", "fund": "fund"}[market]
			if market == "a" && flags["full"] == true {
				method = "cn"
			}
			results[i], failures[i] = invoke(ctx, "quotes."+method, []any{groups[market]})
		}(i, market)
	}
	wg.Wait()
	out := []any{}
	var first error
	messages := []string{}
	for i, result := range results {
		if failures[i] != nil {
			if first == nil {
				first = failures[i]
			}
			messages = append(messages, failures[i].Error())
			continue
		}
		out = appendItems(out, result)
	}
	if len(out) == 0 && first != nil {
		return nil, first
	}
	if len(messages) > 0 {
		if e := writeLine(stderr, "stock-go: 部分市场行情查询失败: "+strings.Join(messages, "; ")); e != nil {
			return nil, e
		}
	}
	return out, nil
}

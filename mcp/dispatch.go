package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func validate(schema map[string]any, value any, path string) error {
	kind, _ := schema["type"].(string)
	ok := true
	switch kind {
	case "object":
		_, ok = value.(map[string]any)
	case "array":
		_, ok = value.([]any)
	case "string":
		_, ok = value.(string)
	case "boolean":
		_, ok = value.(bool)
	case "integer":
		n, is := value.(float64)
		ok = is && !math.IsInf(n, 0) && math.Trunc(n) == n
	case "number":
		_, ok = value.(float64)
	}
	if !ok {
		return fmt.Errorf("参数 %q 类型应为 %s", path, kind)
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, v := range enum {
			if fmt.Sprint(v) == fmt.Sprint(value) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("参数 %q 取值非法", path)
		}
	}
	if obj, ok := value.(map[string]any); ok {
		properties, _ := schema["properties"].(map[string]any)
		if schema["additionalProperties"] == false {
			for k := range obj {
				if _, exists := properties[k]; !exists {
					return fmt.Errorf("未知参数 %q", k)
				}
			}
		}
		if required, ok := schema["required"].([]any); ok {
			for _, key := range required {
				if obj[key.(string)] == nil {
					return fmt.Errorf("缺少必填参数 %q", key)
				}
			}
		}
		for k, v := range obj {
			if property, ok := properties[k].(map[string]any); ok {
				if e := validate(property, v, path+"."+k); e != nil {
					return e
				}
			}
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		if arr, ok := value.([]any); ok {
			for i, v := range arr {
				if e := validate(items, v, fmt.Sprintf("%s[%d]", path, i)); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func (s *Server) arguments(tool Tool, a map[string]any) []any {
	spec := s.specs[tool.Path]
	if spec.MCPCustom {
		opts := map[string]any{}
		for key, value := range a {
			if key != "symbol" {
				opts[key] = value
			}
		}
		return []any{a["symbol"], opts}
	}
	args := []any{}
	if spec.ArgShape == "codes[]" || spec.ArgShape == "codes+options" {
		args = append(args, a["codes"])
	}
	for _, p := range spec.Positional {
		args = append(args, a[p.Name])
	}
	opts := map[string]any{}
	for _, p := range spec.Params {
		if p.MCP != nil && !*p.MCP {
			continue
		}
		field := p.Field
		if field == "" {
			field = p.Flag
		}
		key := p.JSONKey
		if key == "" {
			key = field
		}
		if value, exists := a[key]; exists {
			opts[field] = value
		}
	}
	if spec.ArgShape == "options" || spec.ArgShape == "codes+options" || spec.ArgShape == "symbol+options" {
		args = append(args, opts)
	}
	return args
}
func object(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if e := json.Unmarshal(raw, &out); e != nil || out == nil {
		return nil, fmt.Errorf("params must be an object")
	}
	return out, nil
}
func rpcError(id json.RawMessage, code int, message string) *Response {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &Response{JSONRPC: "2.0", ID: id, Error: &RPCError{code, message}}
}
func parseID(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&value) != nil {
		return false
	}
	switch value.(type) {
	case string, json.Number, nil:
		return true
	}
	return false
}
func (s *Server) Dispatch(ctx context.Context, line []byte) (response *Response) {
	var msg map[string]json.RawMessage
	if !json.Valid(line) {
		return rpcError(nil, ParseError, "Parse error")
	}
	if e := json.Unmarshal(line, &msg); e != nil || msg == nil {
		return rpcError(nil, InvalidRequest, "Invalid Request")
	}
	id, hasID := msg["id"]
	if hasID && !parseID(id) {
		return rpcError(nil, InvalidRequest, "Invalid request id")
	}
	defer func() {
		if v := recover(); v != nil {
			if hasID {
				response = rpcError(id, InternalError, "Internal error")
			} else {
				response = nil
			}
		}
	}()
	var method, version string
	_ = json.Unmarshal(msg["method"], &method)
	_ = json.Unmarshal(msg["jsonrpc"], &version)
	if version != "2.0" || strings.TrimSpace(method) == "" {
		return rpcError(id, InvalidRequest, "Invalid Request")
	}
	// Every notification is response-free, including notifications carrying known request methods.
	if !hasID {
		return nil
	}
	if method == "notifications/initialized" || method == "notifications/cancelled" {
		return nil
	}
	ok := func(result any) *Response { return &Response{JSONRPC: "2.0", ID: id, Result: result} }
	params, e := object(msg["params"])
	if e != nil {
		return rpcError(id, InvalidParams, e.Error())
	}
	switch method {
	case "initialize":
		return ok(map[string]any{"protocolVersion": NegotiateProtocolVersion(params["protocolVersion"]), "capabilities": map[string]any{"tools": map[string]any{}, "prompts": map[string]any{}}, "serverInfo": map[string]any{"name": "stock-go", "version": s.version}})
	case "ping":
		return ok(map[string]any{})
	case "tools/list":
		return ok(map[string]any{"tools": snapshot(s.tools)})
	case "prompts/list":
		return ok(map[string]any{"prompts": snapshot(s.prompts)})
	case "tools/call", "prompts/get":
		name, is := params["name"].(string)
		if !is {
			return rpcError(id, InvalidParams, "params.name must be a string")
		}
		args := map[string]any{}
		if raw, exists := params["arguments"]; exists {
			var is bool
			args, is = raw.(map[string]any)
			if !is {
				return rpcError(id, InvalidParams, "params.arguments must be an object")
			}
		}
		if method == "prompts/get" {
			prompt, exists := s.promptMap[name]
			if !exists {
				return rpcError(id, InvalidParams, "Unknown prompt: "+name)
			}
			text, e := prompt.render(args)
			if e != nil {
				return rpcError(id, InvalidParams, e.Error())
			}
			description := prompt.Title
			if len(prompt.Arguments) > 0 {
				if first := args[prompt.Arguments[0].Name]; truthy(first) {
					description += " · " + jsString(first)
				}
			}
			return ok(map[string]any{"description": description, "messages": []any{map[string]any{"role": "user", "content": Content{Type: "text", Text: text}}}})
		}
		tool, exists := s.toolMap[name]
		if !exists {
			return rpcError(id, InvalidParams, "Unknown tool: "+name)
		}
		if e = validate(tool.InputSchema, args, "arguments"); e == nil && tool.Path == "kline.withIndicators" {
			if indicators, is := args["indicators"].(map[string]any); is {
				known := map[string]bool{}
				for _, k := range []string{"ma", "macd", "boll", "kdj", "rsi", "wr", "bias", "cci", "atr", "obv", "roc", "dmi", "sar", "kc"} {
					known[k] = true
				}
				for k, v := range indicators {
					if !known[k] {
						e = fmt.Errorf("unknown indicator: %s", k)
						break
					}
					switch v.(type) {
					case bool, map[string]any:
					default:
						e = fmt.Errorf("indicator %s must be boolean or object", k)
					}
				}
			}
		}
		if e != nil {
			return ok(ToolErrorResult(&sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: e.Error()}))
		}
		result, e := s.invoke(ctx, tool.Path, s.arguments(tool, args))
		if e != nil {
			return ok(ToolErrorResult(e))
		}
		return ok(ToToolResult(result))
	default:
		return rpcError(id, MethodNotFound, "Unknown method: "+method)
	}
}

func snapshot[T any](value T) T {
	body, _ := encode(value)
	var copy T
	_ = json.Unmarshal(body, &copy)
	return copy
}

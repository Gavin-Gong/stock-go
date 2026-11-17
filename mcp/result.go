package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type ToolResult struct {
	Content []Content      `json:"content"`
	IsError bool           `json:"isError,omitempty"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

func encode(v any) ([]byte, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if e := encoder.Encode(v); e != nil {
		return nil, e
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
func utf16Length(text []byte) int {
	n := 0
	for _, r := range string(text) {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}
func ToToolResult(data any) ToolResult {
	if data != nil {
		v := reflect.ValueOf(data)
		if (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) && v.Len() > MaxArrayItems {
			data = struct {
				Total     int    `json:"total"`
				Truncated bool   `json:"truncated"`
				Note      string `json:"note"`
				Sample    any    `json:"sample"`
			}{v.Len(), true, "结果过大，仅返回前 200 条；请用日期 / 分页 / 市场等参数缩小范围", arraySample(v)}
		}
	}
	text, e := encode(data)
	if e != nil {
		return ToolErrorResult(e)
	}
	length := utf16Length(text)
	var meta map[string]any
	if length > MaxTextChars {
		meta = map[string]any{"truncated": true, "approxChars": length}
		text, _ = encode(struct {
			Truncated bool   `json:"truncated"`
			Reason    string `json:"reason"`
			ApproxKB  int    `json:"approxKB"`
			Note      string `json:"note"`
		}{true, "oversized", (length + 512) / 1024, "结果体积过大，已省略数据体；请用更小的日期 / 分页 / 范围参数，或直接用 SDK 获取完整数据"})
	}
	return ToolResult{Content: []Content{{Type: "text", Text: string(text)}}, Meta: meta}
}
func ToolErrorResult(err error) ToolResult {
	code := "UNKNOWN"
	if err == nil {
		err = errors.New("unknown error")
	}
	message := err.Error()
	meta := map[string]any{"code": code}
	var e *sdkerrors.Error
	if errors.As(err, &e) {
		code = string(e.Code)
		message = e.Message
		meta["code"] = code
		if e.Provider != "" {
			meta["provider"] = e.Provider
		}
		if e.URL != "" {
			meta["url"] = e.URL
		}
		if e.Status != 0 {
			meta["status"] = e.Status
		}
	}
	return ToolResult{Content: []Content{{Type: "text", Text: "Error[" + code + "]: " + message}}, IsError: true, Meta: meta}
}

func arraySample(v reflect.Value) []any {
	sample := make([]any, MaxArrayItems)
	for i := range sample {
		sample[i] = v.Index(i).Interface()
	}
	return sample
}

package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/Gavin-Gong/stock-go/internal/gbk"
	"github.com/Gavin-Gong/stock-go/internal/jsonp"
	"github.com/Gavin-Gong/stock-go/internal/jsvars"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func DecodeGBK(data []byte) (string, error) {
	decoded, e := gbk.Decode(data)
	if e != nil {
		return "", &sdkerrors.Error{Code: sdkerrors.Parse, Message: "invalid GBK text", Cause: e}
	}
	return decoded, nil
}

// ParseResponse exposes the upstream permissive tilde/assignment splitter.
// It does not execute JavaScript. SDK quote methods additionally validate assignment keys.
func ParseResponse(text string) []RawQuote {
	out := []RawQuote{}
	for _, line := range strings.Split(text, ";") {
		line = strings.TrimSpace(line)
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		key := strings.TrimPrefix(strings.TrimSpace(line[:i]), "v_")
		raw := strings.TrimSpace(line[i+1:])
		if len(raw) > 0 && raw[0] == '"' && raw[len(raw)-1] == '"' {
			if len(raw) == 1 {
				raw = ""
			} else {
				raw = raw[1 : len(raw)-1]
			}
		}
		out = append(out, RawQuote{key, strings.Split(raw, "~")})
	}
	return out
}

// SafeNumberOrNull uses JavaScript parseFloat semantics, including Infinity.
// SDK service records use finite values so they remain JSON-serializable.
func jsSpace(r rune) bool { return r == 0xfeff || r != 0x85 && unicode.IsSpace(r) }
func SafeNumberOrNull(value string) *float64 {
	value = strings.TrimLeftFunc(value, jsSpace)
	for _, v := range []string{"Infinity", "+Infinity", "-Infinity"} {
		if strings.HasPrefix(value, v) {
			n := math.Inf(1)
			if v[0] == '-' {
				n = math.Inf(-1)
			}
			return &n
		}
	}
	prefix := numberPattern.FindString(value)
	if prefix == "" {
		return nil
	}
	n, e := strconv.ParseFloat(prefix, 64)
	if e != nil && !math.IsInf(n, 0) {
		return nil
	}
	return &n
}
func SafeNumber(value string) float64 {
	if n := SafeNumberOrNull(value); n != nil {
		return *n
	}
	return 0
}
func ToNumber(value string) *float64 { return SafeNumberOrNull(value) }

// jsString mirrors String(value) for JSON-like values accepted by ToNumberSafe.
func jsString(value any) string {
	if value == nil {
		return ""
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		return jsString(v.Elem().Interface())
	}
	if v.Kind() == reflect.Array || v.Kind() == reflect.Slice {
		parts := make([]string, v.Len())
		for i := range parts {
			parts[i] = jsString(v.Index(i).Interface())
		}
		return strings.Join(parts, ",")
	}
	if v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64 {
		n := v.Float()
		if math.IsInf(n, 1) {
			return "Infinity"
		}
		if math.IsInf(n, -1) {
			return "-Infinity"
		}
	}
	if v.Kind() == reflect.Map || v.Kind() == reflect.Struct {
		return "[object Object]"
	}
	return fmt.Sprint(value)
}
func ToNumberSafe(value any) *float64 {
	if value == nil {
		return nil
	}
	return ToNumber(jsString(value))
}

// ToFiniteNumberOrNull requires a complete finite number, unlike the parseFloat helpers.
func ToFiniteNumberOrNull(value any) *float64 {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimFunc(v, jsSpace)
		if len(trimmed) > 2 && trimmed[0] == '0' && strings.ContainsRune("xXoObB", rune(trimmed[1])) {
			base := 16
			switch trimmed[1] {
			case 'o', 'O':
				base = 8
			case 'b', 'B':
				base = 2
			}
			integer, ok := new(big.Int).SetString(trimmed[2:], base)
			if !ok || integer.Sign() < 0 || strings.ContainsAny(trimmed[2:], "_+-") {
				return nil
			}
			n, _ := new(big.Float).SetInt(integer).Float64()
			if math.IsInf(n, 0) {
				return nil
			}
			return &n
		}
		raw, _ := json.Marshal(trimmed)
		return strictNumber(raw)
	case json.Number:
		return strictNumber(json.RawMessage(v))
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Float32, reflect.Float64, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strictNumber(json.RawMessage(fmt.Sprint(value)))
	}
	return nil
}
func ExtractJSONFromJSONP(text string) (json.RawMessage, error) {
	raw, e := jsonp.Extract([]byte(text))
	if e != nil {
		return nil, &sdkerrors.Error{Code: sdkerrors.Parse, Message: "invalid JSONP response", Cause: e}
	}
	return raw, nil
}
func ParseJSVars(text string, names []string) map[string]json.RawMessage {
	return jsvars.Extract(text, names)
}

// FetchJSVarsOptions uses a supplied client's governance. Timeout additionally caps
// this call; zero leaves the client's timeout (15 seconds for a new client).
type FetchJSVarsOptions struct {
	Client  *RequestClient
	Timeout time.Duration
	Headers http.Header
}
type JSONPOptions struct {
	Client        *RequestClient
	Timeout       time.Duration
	Headers       http.Header
	CallbackParam string
	// CallbackMode is "query" (default) or "path", replacing {callback}.
	CallbackMode string
}

func helperClient(client *RequestClient, timeout time.Duration) (*RequestClient, error) {
	if timeout < 0 {
		return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "timeout must be nonnegative"}
	}
	if client != nil {
		return client, nil
	}
	return NewRequestClient(RequestOptions{Timeout: timeout})
}
func fetchParsed(ctx context.Context, client *RequestClient, timeout time.Duration, endpoint string, headers http.Header, validate func([]byte) error) ([]byte, error) {
	parsed, e := url.Parse(endpoint)
	if e != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Fragment != "" {
		return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "endpoint must be an absolute HTTP(S) URL without a fragment", URL: endpoint, Cause: e}
	}
	client, e = helperClient(client, timeout)
	if e != nil {
		return nil, e
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return client.GetParsed(ctx, "", endpoint, headers, validate)
}
func FetchJSVars(ctx context.Context, endpoint string, names []string, options ...FetchJSVarsOptions) (map[string]json.RawMessage, error) {
	if len(options) > 1 {
		return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "at most one FetchJSVarsOptions is allowed"}
	}
	o := FetchJSVarsOptions{}
	if len(options) == 1 {
		o = options[0]
	}
	body, e := fetchParsed(ctx, o.Client, o.Timeout, endpoint, o.Headers, nil)
	if e != nil {
		return nil, e
	}
	return ParseJSVars(string(body), names), nil
}

var jsonpCounter atomic.Uint64

// JSONPRequest fetches and safely decodes a query/path callback without executing scripts.
func JSONPRequest(ctx context.Context, endpoint string, options ...JSONPOptions) (json.RawMessage, error) {
	if len(options) > 1 {
		return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "at most one JSONPOptions is allowed"}
	}
	o := JSONPOptions{}
	if len(options) == 1 {
		o = options[0]
	}
	if o.CallbackMode != "" && o.CallbackMode != "query" && o.CallbackMode != "path" {
		return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "callback mode must be query or path"}
	}
	cb := "__stock_go_jsonp_" + strconv.FormatUint(jsonpCounter.Add(1), 10)
	if o.CallbackMode == "path" {
		if !strings.Contains(endpoint, "{callback}") {
			return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "path JSONP URL must contain {callback}"}
		}
		endpoint = strings.Replace(endpoint, "{callback}", cb, 1)
	} else {
		u, e := url.Parse(endpoint)
		if e != nil {
			return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "invalid JSONP URL", Cause: e}
		}
		name := o.CallbackParam
		if name == "" {
			name = "callback"
		}
		q := u.Query()
		q.Set(name, cb)
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}
	var result json.RawMessage
	_, e := fetchParsed(ctx, o.Client, o.Timeout, endpoint, o.Headers, func(body []byte) error { var err error; result, err = jsonp.Extract(body); return err })
	if e != nil {
		return nil, e
	}
	return result, nil
}

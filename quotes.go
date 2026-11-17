package stockgo

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"github.com/Gavin-Gong/stock-go/symbols"
)

type QuoteService struct {
	fundFlow             *FundFlowService
	client               *transport.Client
	baseURL, timelineURL string
}

// RawQuote contains a Tencent response key and its unmodified tilde-separated fields.
type RawQuote struct {
	Key    string   `json:"key"`
	Fields []string `json:"fields"`
}

var assignmentPattern = regexp.MustCompile(`(?:^|;)\s*(?:var\s+)?v_([A-Za-z0-9_.-]+)\s*=\s*"([^"\r\n]*)"\s*`)
var numberPattern = regexp.MustCompile(`^[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?`)
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
var fundPattern = regexp.MustCompile(`^\d{6}$`)

func at(f []string, i int) string {
	if i < 0 || i >= len(f) {
		return ""
	}
	return f[i]
}
func nullable(s string) *float64 {
	s = numberPattern.FindString(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}
func num(s string) float64 {
	if p := nullable(s); p != nil {
		return *p
	}
	return 0
}
func quoteMeta(code string, market symbols.Market) (string, symbols.AssetType) {
	bare := strings.TrimPrefix(code, ".")
	s, err := symbols.Normalize(bare, symbols.Hint{Market: market})
	if err == nil && s.AssetType == symbols.Index {
		return s.Code, s.AssetType
	}
	return code, symbols.Stock
}
func endpointQuery(base string, params url.Values) string {
	u, _ := url.Parse(base)
	q := u.Query()
	for key, values := range params {
		q[key] = values
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func (s *QuoteService) raw(ctx context.Context, keys []string) ([]RawQuote, error) {
	out := make([]RawQuote, 0)
	if len(keys) == 0 {
		return out, nil
	}
	endpoint := endpointQuery(s.baseURL, url.Values{"q": {strings.Join(keys, ",")}})
	body, err := s.client.Get(ctx, "tencent", endpoint, http.Header{"Referer": {"https://gu.qq.com/"}})
	if err != nil {
		return nil, err
	}
	text, err := DecodeGBK(body)
	if err != nil {
		return nil, &sdkerrors.Error{Code: sdkerrors.Parse, Message: "invalid GBK quote response", Provider: "tencent", URL: endpoint, Cause: err}
	}
	for _, m := range assignmentPattern.FindAllStringSubmatch(text, -1) {
		out = append(out, RawQuote{m[1], strings.Split(m[2], "~")})
	}
	return out, nil
}
func filterRows(rows []RawQuote, keys []string, min int) []RawQuote {
	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[key] = true
	}
	out := make([]RawQuote, 0, len(rows))
	for _, row := range rows {
		if wanted[row.Key] && len(row.Fields) >= min && row.Fields[0] != "" {
			out = append(out, row)
		}
	}
	return out
}
func (s *QuoteService) rows(ctx context.Context, keys []string, min int) ([]RawQuote, error) {
	rows, err := s.raw(ctx, keys)
	if err != nil {
		return nil, err
	}
	return filterRows(rows, keys, min), nil
}
func mapRows[T any](rows []RawQuote, parse func([]string) T) []T {
	out := make([]T, 0, len(rows))
	for _, r := range rows {
		out = append(out, parse(r.Fields))
	}
	return out
}
func withPrefix(keys []string, prefix string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = prefix + k
	}
	return out
}

// CN returns full A-share quotes. Invalid/unsupported symbols and incomplete rows are skipped.
// Volume and amount retain Tencent's source units (lots and ten-thousand currency units).
func (s *QuoteService) CN(ctx context.Context, codes []string) ([]FullQuote, error) {
	rows, err := s.rows(ctx, symbols.TencentBatch(codes, symbols.CN).Keys, 38)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, parseFullQuote), nil
}
func (s *QuoteService) CNSimple(ctx context.Context, codes []string) ([]SimpleQuote, error) {
	keys := withPrefix(symbols.TencentBatch(codes, symbols.CN).Keys, "s_")
	rows, err := s.rows(ctx, keys, 8)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, parseSimpleQuote), nil
}

// HK prefers real-time r_hk keys and falls back to delayed hk keys for missing valid rows.
func (s *QuoteService) HK(ctx context.Context, codes []string) ([]HKQuote, error) {
	keys := symbols.TencentBatch(codes, symbols.HK).Keys
	request := make([]string, len(keys))
	for i, k := range keys {
		request[i] = k
		if digitsOnly(k[2:]) {
			request[i] = "r_" + k
		}
	}
	rows, err := s.rows(ctx, request, 38)
	if err != nil {
		return nil, err
	}
	got := map[string]bool{}
	for _, row := range rows {
		got[row.Key] = true
	}
	fallback := []string{}
	for i, k := range keys {
		if request[i] != k && !got[request[i]] {
			fallback = append(fallback, k)
		}
	}
	if len(fallback) > 0 {
		more, err := s.rows(ctx, fallback, 38)
		if err != nil {
			return nil, err
		}
		rows = append(rows, more...)
	}
	return mapRows(rows, parseHKQuote), nil
}
func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func (s *QuoteService) US(ctx context.Context, codes []string) ([]USQuote, error) {
	rows, err := s.rows(ctx, symbols.TencentBatch(codes, symbols.US).Keys, 38)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, parseUSQuote), nil
}
func (s *QuoteService) Fund(ctx context.Context, codes []string) ([]FundQuote, error) {
	keys := []string{}
	seen := map[string]bool{}
	for _, code := range codes {
		if !fundPattern.MatchString(code) {
			return nil, &sdkerrors.Error{Code: sdkerrors.InvalidSymbol, Message: "fund code must contain exactly six digits"}
		}
		if !seen[code] {
			keys = append(keys, "jj"+code)
			seen[code] = true
		}
	}
	rows, err := s.rows(ctx, keys, 9)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, parseFundQuote), nil
}
func (s *QuoteService) LargeOrder(ctx context.Context, codes []string) ([]PanelLargeOrder, error) {
	rows, err := s.rows(ctx, withPrefix(symbols.TencentBatch(codes, symbols.CN).Keys, "s_pk"), 4)
	if err != nil {
		return nil, err
	}
	return mapRows(rows, parsePanelLargeOrder), nil
}

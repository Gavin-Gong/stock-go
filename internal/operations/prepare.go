package operations

import (
	"context"
	"strconv"
	"strings"
	"time"

	stockgo "github.com/Gavin-Gong/stock-go"
	"github.com/Gavin-Gong/stock-go/symbols"
)

func object(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{}, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, usage("options must be a JSON object")
	}
	copy := map[string]any{}
	for k, v := range m {
		copy[k] = v
	}
	return copy, nil
}
func at(args []any, i int) any {
	if i < len(args) {
		return args[i]
	}
	return nil
}
func text(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
func prepare(path string, args []any) ([]any, error) {
	out := append([]any{}, args...)
	// Convert source option unions to the public Go SDK's explicit representations.
	for i, value := range out {
		if value == nil {
			continue
		}
		if _, ok := value.(map[string]any); !ok {
			continue
		}
		m, e := object(value)
		if e != nil {
			return nil, e
		}
		out[i] = m
		if v, ok := m["adjust"]; ok && v == "" {
			m["adjust"] = "none"
		}
		if m["market"] == "A" || m["market"] == "a" {
			if strings.HasPrefix(path, "kline.") {
				m["market"] = "CN"
			}
		}
		if path == "fund.dividendList" {
			for _, key := range []string{"page", "year"} {
				if v, ok := m[key].(string); ok {
					if key == "page" && v == "all" {
						delete(m, key)
						m["allPages"] = true
					} else {
						n, e := strconv.Atoi(v)
						if e != nil {
							return nil, usage(key + " must be a positive integer or page=all")
						}
						m[key] = n
					}
				}
			}
		}
		if strings.HasPrefix(path, "chips.") {
			if v, ok := m["includeHistogram"].(bool); ok {
				if v {
					m["includeHistogram"] = "all"
				} else {
					delete(m, "includeHistogram")
				}
			}
		}
		if path == "kline.withIndicators" {
			if v, exists := m["indicators"]; exists {
				items, e := object(v)
				if e != nil {
					return nil, e
				}
				for k, v := range items {
					if enabled, ok := v.(bool); ok {
						if enabled {
							items[k] = map[string]any{}
						} else {
							delete(items, k)
						}
					}
				}
				m["indicators"] = items
			}
		}
	}
	switch path {
	case "marketEvent.ztPool":
		if e := arity(out, 0, 2); e != nil {
			return nil, e
		}
		return []any{map[string]any{"type": at(out, 0), "date": at(out, 1)}}, nil
	case "marketEvent.stockChanges":
		if e := arity(out, 0, 2); e != nil {
			return nil, e
		}
		m, e := object(at(out, 1))
		if e != nil {
			return nil, e
		}
		if v := at(out, 0); v != nil {
			if _, ok := v.(string); ok {
				m["types"] = []any{v}
			} else {
				m["types"] = v
			}
		}
		return []any{m}, nil
	case "northbound.history":
		if e := arity(out, 0, 2); e != nil {
			return nil, e
		}
		m, e := object(at(out, 1))
		if e != nil {
			return nil, e
		}
		if at(out, 0) != nil {
			m["direction"] = out[0]
		}
		return []any{m}, nil
	case "marketEvent.individualChanges", "marketEvent.individualChangesHistory", "board.industry.minuteKline", "board.concept.minuteKline":
		if e := arity(out, 1, 2); e != nil {
			return nil, e
		}
		if len(out) == 2 && out[1] != nil {
			m, e := object(out[1])
			if e != nil {
				return nil, e
			}
			field := "period"
			if path == "marketEvent.individualChanges" {
				field = "date"
			} else if path == "marketEvent.individualChangesHistory" {
				field = "days"
			}
			if v, ok := m[field]; ok {
				return []any{out[0], v}, nil
			}
		}
		return out[:1], nil
	}
	return out, nil
}
func marketStatus(s *stockgo.SDK) Handler {
	return func(ctx context.Context, args []any) (any, error) {
		if e := arity(args, 0, 2); e != nil {
			return nil, e
		}
		market := symbols.CN
		if len(args) > 0 && args[0] != nil {
			v, e := decode[string](args[0])
			if e != nil {
				return nil, e
			}
			market = symbols.Market(strings.ToUpper(v))
			if market == "A" {
				market = symbols.CN
			}
		}
		if len(args) < 2 || args[1] == nil {
			return s.Calendar.MarketStatus(market)
		}
		v, e := decode[string](args[1])
		if e != nil {
			return nil, e
		}
		now, e := time.Parse(time.RFC3339Nano, v)
		if e != nil {
			return nil, usage("time must be RFC3339")
		}
		return s.Calendar.MarketStatus(market, now)
	}
}

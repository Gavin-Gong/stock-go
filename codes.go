package stockgo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/internal/memo"
	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

// CodeEndpoints overrides the static code-list sources for testing or proxies.
type CodeEndpoints struct{ CN, HK, US, Fund string }
type CodeService struct {
	calendarURL string
	client      *transport.Client
	endpoints   CodeEndpoints
	cache       *memo.Store[[]string]
}

// CodeOptions filters exchanges: CN uses sh/sz/bj/kc/cy; US uses NASDAQ/NYSE/AMEX.
type CodeOptions struct {
	Simple bool
	Market string
}

func argument(message string) error {
	return &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: message}
}
func wrapContext(err error) error {
	if errors.Is(err, context.Canceled) {
		return &sdkerrors.Error{Code: sdkerrors.Aborted, Message: err.Error(), Cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &sdkerrors.Error{Code: sdkerrors.Timeout, Message: err.Error(), Cause: err}
	}
	return err
}
func (s *CodeService) list(ctx context.Context, endpoint string, fund bool) ([]string, error) {
	list, err := s.cache.Load(ctx, endpoint, 6*time.Hour, func(ctx context.Context) ([]string, error) {
		body, err := s.client.Get(ctx, "linkdiary", endpoint, nil)
		if err != nil {
			return nil, err
		}
		var list []string
		if fund {
			parts := strings.Split(string(body), ",")
			for _, p := range parts[1:] {
				if p = strings.TrimSpace(p); p != "" {
					list = append(list, p)
				}
			}
		} else {
			var response struct {
				Success *bool    `json:"success"`
				List    []string `json:"list"`
			}
			if err := json.Unmarshal(body, &response); err != nil {
				return nil, &sdkerrors.Error{Code: sdkerrors.Parse, Message: "invalid code list JSON", Provider: "linkdiary", URL: endpoint, Cause: err}
			}
			if response.Success == nil || *response.Success {
				list = response.List
			}
		}
		if len(list) == 0 {
			return nil, &sdkerrors.Error{Code: sdkerrors.UpstreamEmpty, Message: "empty code list", Provider: "linkdiary", URL: endpoint}
		}
		return list, nil
	})
	if err != nil {
		return nil, wrapContext(err)
	}
	return append([]string{}, list...), nil
}
func stripCN(code string) string {
	for _, p := range []string{"sh", "sz", "bj"} {
		if strings.HasPrefix(code, p) {
			return code[2:]
		}
	}
	return code
}
func matchCN(code, market string) bool {
	code = stripCN(code)
	switch market {
	case "":
		return true
	case "sh":
		return strings.HasPrefix(code, "6")
	case "sz":
		return strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3")
	case "bj":
		return strings.HasPrefix(code, "4") || strings.HasPrefix(code, "8") || strings.HasPrefix(code, "92")
	case "kc":
		return strings.HasPrefix(code, "688")
	case "cy":
		return strings.HasPrefix(code, "30")
	}
	return false
}
func codeOption(options []CodeOptions) (CodeOptions, error) {
	if len(options) > 1 {
		return CodeOptions{}, argument("at most one CodeOptions is allowed")
	}
	if len(options) == 1 {
		return options[0], nil
	}
	return CodeOptions{}, nil
}
func (s *CodeService) CN(ctx context.Context, options ...CodeOptions) ([]string, error) {
	o, err := codeOption(options)
	if err != nil {
		return nil, err
	}
	switch o.Market {
	case "", "sh", "sz", "bj", "kc", "cy":
	default:
		return nil, argument("invalid CN code market")
	}
	list, err := s.list(ctx, s.endpoints.CN, false)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, code := range list {
		if matchCN(code, o.Market) {
			if o.Simple {
				code = stripCN(code)
			}
			out = append(out, code)
		}
	}
	return out, nil
}
func (s *CodeService) US(ctx context.Context, options ...CodeOptions) ([]string, error) {
	o, err := codeOption(options)
	if err != nil {
		return nil, err
	}
	prefix := map[string]string{"NASDAQ": "105.", "NYSE": "106.", "AMEX": "107."}[o.Market]
	if o.Market != "" && prefix == "" {
		return nil, argument("invalid US code market")
	}
	list, err := s.list(ctx, s.endpoints.US, false)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, code := range list {
		if !strings.HasPrefix(code, prefix) {
			continue
		}
		if o.Simple {
			_, plain, ok := strings.Cut(code, ".")
			if ok {
				code = plain
			}
		}
		out = append(out, code)
	}
	return out, nil
}
func (s *CodeService) HK(ctx context.Context) ([]string, error) {
	return s.list(ctx, s.endpoints.HK, false)
}
func (s *CodeService) Fund(ctx context.Context) ([]string, error) {
	return s.list(ctx, s.endpoints.Fund, true)
}

// ClearCaches invalidates this instance's public caches, code lists, calendar, US symbols and board names.
func (s *SDK) ClearCaches() { s.client.ClearCaches() }

func (s *SDK) clearServiceCaches() {
	s.Codes.cache.Clear()
	s.Kline.usCache.Clear()
	s.Board.Industry.names.Clear()
	s.Board.Concept.names.Clear()
}

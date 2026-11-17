package stockgo

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

// PageOptions selects one page when Page or PageSize is nonzero. Otherwise all pages are fetched.
// Automatic pagination uses bounded waves; exceeding MaxPages returns an explicit error.
type PageOptions struct{ Page, PageSize, MaxPages, Concurrency int }
type emRow map[string]json.RawMessage

func rowText(row emRow, key string) string {
	raw := row[key]
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if value, ok := rawString(raw); ok {
		return value
	}
	return string(raw)
}
func rowNumber(row emRow, key string) *float64 { return rawNumber(row[key]) }
func optionalText(row emRow, key string) *string {
	raw := row[key]
	text := rowText(row, key)
	if text == "" || string(raw) == "0" || string(raw) == "false" {
		return nil
	}
	return &text
}
func pageOptions(options []PageOptions) (PageOptions, error) {
	if len(options) > 1 {
		return PageOptions{}, argument("at most one PageOptions is allowed")
	}
	var o PageOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Page < 0 || o.PageSize < 0 || o.PageSize > 100 || o.MaxPages < 0 || o.Concurrency < 0 {
		return o, argument("invalid pagination options (pageSize must be <=100)")
	}
	if o.MaxPages == 0 {
		o.MaxPages = 1000
	}
	if o.Concurrency == 0 {
		o.Concurrency = 3
	}
	return o, nil
}

type clistPage struct {
	Rows    []emRow
	Total   int
	Present bool
}

func fetchClist(ctx context.Context, client *transport.Client, endpoint string, params url.Values, page, size int) (clistPage, error) {
	params = cloneValues(params)
	params.Set("pn", numberString(page))
	params.Set("pz", numberString(size))
	endpoint = endpointQuery(endpoint, params)
	var response struct {
		RC   int `json:"rc"`
		Data *struct {
			Total int     `json:"total"`
			Diff  []emRow `json:"diff"`
		} `json:"data"`
	}
	if err := getJSON(ctx, client, "eastmoney", endpoint, &response); err != nil {
		return clistPage{}, err
	}
	if response.RC != 0 {
		return clistPage{}, klineError(sdkerrors.UpstreamError, "eastmoney", endpoint, "clist upstream error", nil)
	}
	if response.Data == nil || response.Data.Diff == nil {
		return clistPage{}, nil
	}
	rows := make([]emRow, 0, len(response.Data.Diff))
	for _, row := range response.Data.Diff {
		if row != nil {
			rows = append(rows, row)
		}
	}
	return clistPage{rows, response.Data.Total, true}, nil
}
func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, values := range v {
		out[k] = append([]string{}, values...)
	}
	return out
}
func clistRows(ctx context.Context, client *transport.Client, endpoint string, params url.Values, o PageOptions, shortTerminal bool) ([]emRow, error) {
	size := o.PageSize
	if size == 0 {
		size = 100
	}
	if o.Page != 0 || o.PageSize != 0 {
		page := max(1, o.Page)
		data, err := fetchClist(ctx, client, endpoint, params, page, size)
		if err != nil {
			return nil, err
		}
		return append([]emRow{}, data.Rows...), nil
	}
	first, err := fetchClist(ctx, client, endpoint, params, 1, size)
	if err != nil {
		return nil, err
	}
	out := append([]emRow{}, first.Rows...)
	if !first.Present || len(first.Rows) == 0 || len(out) >= first.Total || (shortTerminal && len(first.Rows) < size) {
		return out, nil
	}
	last := first.Total / size
	if first.Total%size != 0 {
		last++
	}
	limit := min(last, o.MaxPages)
	for start := 2; start <= limit; {
		width := min(o.Concurrency, limit-start+1)
		ids := make([]string, width)
		for i := range ids {
			ids[i] = numberString(start + i)
		}
		pages, err := batchFetch(ctx, ids, BatchOptions{BatchSize: 1, Concurrency: width}, func(ctx context.Context, ids []string) ([]clistPage, error) {
			page, _ := strconv.Atoi(ids[0])
			data, err := fetchClist(ctx, client, endpoint, params, page, size)
			if err != nil {
				return nil, err
			}
			return []clistPage{data}, nil
		})
		if err != nil {
			return nil, err
		}
		for _, data := range pages {
			if !data.Present || len(data.Rows) == 0 {
				return out, nil
			}
			out = append(out, data.Rows...)
			if shortTerminal && len(data.Rows) < size {
				return out, nil
			}
		}
		start += width
	}
	if !shortTerminal {
		for page := last + 1; len(out) < first.Total && page <= o.MaxPages; page++ {
			data, err := fetchClist(ctx, client, endpoint, params, page, size)
			if err != nil {
				return nil, err
			}
			if !data.Present || len(data.Rows) == 0 {
				return out, nil
			}
			out = append(out, data.Rows...)
		}
	}
	if len(out) < first.Total && last >= o.MaxPages {
		return nil, klineError(sdkerrors.UpstreamError, "eastmoney", endpoint, "automatic pagination exceeded maxPages", nil)
	}
	return out, nil
}

func optionalRowText(row emRow, key string) string {
	if value := optionalText(row, key); value != nil {
		return *value
	}
	return ""
}

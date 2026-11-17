package stockgo

import (
	"context"
	"net/url"
	"strings"

	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

// DataRow retains upstream field names for advanced datacenter reports.
type DataRow = emRow
type DatacenterQuery struct {
	ReportName, Columns, Filter, SortColumns, SortTypes, QuoteColumns, QuoteType string
	PageSize, StartPage, MaxPages, Concurrency                                   int
	FetchAllPages                                                                *bool
	ExtraParams                                                                  map[string]string
}
type DatacenterResult struct {
	Data  []DataRow `json:"data"`
	Total int       `json:"total"`
	Pages int       `json:"pages"`
}
type DatacenterService struct {
	client   *transport.Client
	endpoint string
}

func Bool(v bool) *bool { return &v }
func (s *DatacenterService) Query(ctx context.Context, q DatacenterQuery) (*DatacenterResult, error) {
	if strings.TrimSpace(q.ReportName) == "" {
		return nil, argument("reportName must not be empty")
	}
	if q.PageSize < 0 || q.PageSize > 5000 || q.StartPage < 0 || q.MaxPages < 0 || q.Concurrency < 0 {
		return nil, argument("invalid datacenter paging options")
	}
	if q.PageSize == 0 {
		q.PageSize = 500
	}
	if q.StartPage == 0 {
		q.StartPage = 1
	}
	if q.MaxPages == 0 {
		q.MaxPages = 1000
	}
	if q.Concurrency == 0 {
		q.Concurrency = 3
	}
	if q.Columns == "" {
		q.Columns = "ALL"
	}
	type page struct {
		Rows         []emRow
		Pages, Count *int
		Present      bool
	}
	fetch := func(ctx context.Context, number int) (page, error) {
		params := url.Values{"reportName": {q.ReportName}, "columns": {q.Columns}, "pageSize": {numberString(q.PageSize)}, "pageNumber": {numberString(number)}, "source": {"WEB"}, "client": {"WEB"}}
		for k, v := range map[string]string{"filter": q.Filter, "sortColumns": q.SortColumns, "sortTypes": q.SortTypes, "quoteColumns": q.QuoteColumns, "quoteType": q.QuoteType} {
			if v != "" {
				params.Set(k, v)
			}
		}
		for k, v := range q.ExtraParams {
			params.Set(k, v)
		}
		endpoint := endpointQuery(s.endpoint, params)
		var response struct {
			Success *bool  `json:"success"`
			Message string `json:"message"`
			Result  *struct {
				Pages, Count *int
				Data         []emRow
			} `json:"result"`
		}
		if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
			return page{}, err
		}
		if response.Success != nil && !*response.Success {
			return page{}, klineError(sdkerrors.UpstreamError, "eastmoney", endpoint, response.Message, nil)
		}
		if response.Result == nil || response.Result.Data == nil {
			return page{}, nil
		}
		rows := make([]emRow, 0, len(response.Result.Data))
		for _, row := range response.Result.Data {
			if row != nil {
				rows = append(rows, row)
			}
		}
		return page{rows, response.Result.Pages, response.Result.Count, true}, nil
	}
	first, err := fetch(ctx, q.StartPage)
	if err != nil {
		return nil, err
	}
	out := &DatacenterResult{Data: []DataRow{}, Pages: 1}
	if !first.Present {
		return out, nil
	}
	out.Data = append(out.Data, first.Rows...)
	out.Total = len(first.Rows)
	if first.Count != nil {
		out.Total = max(0, *first.Count)
	}
	if first.Pages != nil {
		out.Pages = max(0, *first.Pages)
	}
	if (q.FetchAllPages != nil && !*q.FetchAllPages) || out.Pages <= q.StartPage {
		return out, nil
	}
	limit := q.StartPage + min(q.MaxPages-1, out.Pages-q.StartPage)
	for start := q.StartPage + 1; start <= limit; {
		width := min(q.Concurrency, limit-start+1)
		numbers := make([]string, width)
		for i := range numbers {
			numbers[i] = numberString(start + i)
		}
		pages, err := batchFetch(ctx, numbers, BatchOptions{BatchSize: 1, Concurrency: width}, func(ctx context.Context, ids []string) ([]page, error) {
			number := 0
			for _, c := range ids[0] {
				number = number*10 + int(c-'0')
			}
			p, err := fetch(ctx, number)
			if err != nil {
				return nil, err
			}
			return []page{p}, nil
		})
		if err != nil {
			return nil, err
		}
		for _, p := range pages {
			if !p.Present {
				return out, nil
			}
			out.Data = append(out.Data, p.Rows...)
		}
		start += width
	}
	if limit < out.Pages {
		return nil, klineError(sdkerrors.UpstreamError, "eastmoney", s.endpoint, "datacenter pagination exceeded maxPages", nil)
	}
	return out, nil
}
func (s *DatacenterService) rows(ctx context.Context, q DatacenterQuery) ([]emRow, error) {
	out, err := s.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	return out.Data, nil
}

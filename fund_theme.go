package stockgo

import (
	"context"
	"encoding/json"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"net/url"

	"github.com/Gavin-Gong/stock-go/internal/transport"
)

type FundThemeService struct {
	client *transport.Client
	base   string
}
type ThemeListOptions struct {
	Sort, Order, Category string
	Page, PageSize        int
}
type ThemeFundsOptions struct {
	SortColumn, Sort, FundType string
	Page, PageSize             int
}

func themeParams() url.Values {
	return url.Values{"plat": {"Android"}, "appType": {"ttjj"}, "product": {"EFund"}, "Version": {"1"}, "deviceid": {"stock-go"}}
}
func validateThemePage(page, size int) error {
	if page < 1 || size < 1 || size > 500 {
		return argument("theme page must be positive; pageSize must be 1..500")
	}
	return nil
}
func (s *FundThemeService) GetThemeList(ctx context.Context, options ...ThemeListOptions) (*ThemeFundListResult, error) {
	if len(options) > 1 {
		return nil, argument("at most one ThemeListOptions is allowed")
	}
	var o ThemeListOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Sort == "" {
		o.Sort = "ZDF"
	}
	if o.Order == "" {
		o.Order = "desc"
	}
	if o.Category == "" {
		o.Category = "2"
	}
	if o.Page == 0 {
		o.Page = 1
	}
	if o.PageSize == 0 {
		o.PageSize = 20
	}
	if err := validateThemePage(o.Page, o.PageSize); err != nil {
		return nil, err
	}
	if o.Order != "asc" && o.Order != "desc" {
		return nil, argument("theme order must be asc or desc")
	}
	switch o.Category {
	case "0", "1", "2":
	default:
		return nil, argument("theme category must be 0, 1 or 2")
	}
	switch o.Sort {
	case "ZDF", "SYL_W", "SYL_M", "SYL_3M", "SYL_6M", "SYL_Y", "SYL_3Y", "SYL_5Y":
	default:
		return nil, argument("invalid theme sort")
	}
	params := themeParams()
	params.Set("RankItems", o.Sort)
	params.Set("RankVectors", o.Order)
	params.Set("category", o.Category)
	params.Set("pageSize", numberString(o.PageSize))
	params.Set("page", numberString(o.Page))
	endpoint := endpointQuery(appendURLPath(s.base, "FundMNSubjectList"), params)
	var response struct {
		Data                 []emRow
		Datas                []emRow
		Success              *bool
		ErrCode              int
		ErrMsg, ErrorMessage string
		TotalCount           int
		Pageinfo             *struct{ TotalPages, PageSize, Page *int }
	}
	if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
		return nil, err
	}
	if response.Success != nil && !*response.Success || response.ErrCode != 0 {
		return nil, klineError(sdkerrors.UpstreamError, "eastmoney", endpoint, "theme request failed: "+response.ErrMsg+response.ErrorMessage, nil)
	}
	out := &ThemeFundListResult{Items: []ThemeFund{}, PageSize: o.PageSize, CurrentPage: o.Page}
	rows := response.Data
	if rows == nil {
		rows = response.Datas
	}
	if len(rows) > 0 && len(rows[0]["INDEXCODE"]) > 0 {
		out.CatalogOnly = true
		catalog := []emRow{}
		for _, r := range rows {
			kind := rowText(r, "TYPE")
			if o.Category == "0" && kind != "1" {
				continue
			}
			if o.Category == "1" && kind != "2" {
				continue
			}
			catalog = append(catalog, r)
		}
		out.TotalPages = len(catalog) / o.PageSize
		if len(catalog)%o.PageSize != 0 {
			out.TotalPages++
		}
		if o.Page > out.TotalPages {
			return out, nil
		}
		start := (o.Page - 1) * o.PageSize
		rows = catalog[start:min(start+o.PageSize, len(catalog))]
	} else if response.Pageinfo == nil && response.TotalCount > 0 {
		out.TotalPages = response.TotalCount / o.PageSize
		if response.TotalCount%o.PageSize != 0 {
			out.TotalPages++
		}
	}

	if response.Pageinfo != nil {
		if response.Pageinfo.TotalPages != nil {
			out.TotalPages = *response.Pageinfo.TotalPages
		}
		if response.Pageinfo.PageSize != nil {
			out.PageSize = *response.Pageinfo.PageSize
		}
		if response.Pageinfo.Page != nil {
			out.CurrentPage = *response.Pageinfo.Page
		}
	}
	for _, r := range rows {
		kind := "行业"
		if rowText(r, "TYPE") == "2" {
			kind = "概念"
		}
		_, isString := rawString(r["isIndustry"])
		if n := strictNumber(r["isIndustry"]); !isString && n != nil && *n == 0 {
			kind = "概念"
		}
		out.Items = append(out.Items, ThemeFund{Code: rowTextDefault(r, "code", "INDEXCODE"), Name: rowTextDefault(r, "name", "INDEXNAME"), DailyChange: strictNumber(r["ZDF"]), WeeklyReturn: strictNumber(r["SYL_W"]), MonthlyReturn: strictNumber(r["SYL_M"]), QuarterlyReturn: strictNumber(r["SYL_3M"]), HalfYearReturn: strictNumber(r["SYL_6M"]), YearlyReturn: strictNumber(r["SYL_Y"]), ThreeYearReturn: strictNumber(r["SYL_3Y"]), FiveYearReturn: strictNumber(r["SYL_5Y"]), Type: kind})
	}
	return out, nil
}
func (s *FundThemeService) GetThemeFunds(ctx context.Context, code string, options ...ThemeFundsOptions) (*ThemeFundItemList, error) {
	if len(options) > 1 {
		return nil, argument("at most one ThemeFundsOptions is allowed")
	}
	if code == "" {
		return nil, argument("theme code must not be empty")
	}
	var o ThemeFundsOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.SortColumn == "" {
		o.SortColumn = "SYL_1N"
	}
	if o.Sort == "" {
		o.Sort = "desc"
	}
	if o.Page == 0 {
		o.Page = 1
	}
	if o.PageSize == 0 {
		o.PageSize = 20
	}
	if err := validateThemePage(o.Page, o.PageSize); err != nil {
		return nil, err
	}
	if o.Sort != "asc" && o.Sort != "desc" {
		return nil, argument("theme sort must be asc or desc")
	}
	switch o.SortColumn {
	case "SYL_Z", "SYL_Y", "SYL_3Y", "SYL_1N", "RZDF":
	default:
		return nil, argument("invalid theme fund sort column")
	}
	params := themeParams()
	for k, v := range map[string]string{"TOPICAL": code, "SortColumn": o.SortColumn, "Sort": o.Sort, "pageIndex": numberString(o.Page), "pageSize": numberString(o.PageSize), "FundType": o.FundType} {
		params.Set(k, v)
	}
	endpoint := endpointQuery(appendURLPath(s.base, "FundMNRank"), params)
	var response struct {
		Data                 []emRow
		Datas                []emRow
		Success              *bool
		ErrCode              int
		ErrMsg, ErrorMessage string
		TotalCount           int
		Fundinfo             *struct{ Total, Pagesize, Pageindex *int }
	}
	if err := getJSON(ctx, s.client, "eastmoney", endpoint, &response); err != nil {
		return nil, err
	}
	if response.Success != nil && !*response.Success || response.ErrCode != 0 {
		return nil, klineError(sdkerrors.UpstreamError, "eastmoney", endpoint, "theme fund request failed: "+response.ErrMsg+response.ErrorMessage, nil)
	}
	out := &ThemeFundItemList{Items: []ThemeFundItem{}, PageIndex: o.Page, PageSize: o.PageSize}
	rows := response.Data
	if rows == nil {
		rows = response.Datas
	}
	if response.Fundinfo == nil {
		out.Total = response.TotalCount
	}

	if response.Fundinfo != nil {
		if response.Fundinfo.Total != nil {
			out.Total = *response.Fundinfo.Total
		}
		if response.Fundinfo.Pageindex != nil {
			out.PageIndex = *response.Fundinfo.Pageindex
		}
		if response.Fundinfo.Pagesize != nil {
			out.PageSize = *response.Fundinfo.Pagesize
		}
	}
	for _, r := range rows {
		out.Items = append(out.Items, ThemeFundItem{Code: rowTextDefault(r, "fscode", "FCODE"), Name: rowTextDefault(r, "abname", "SHORTNAME"), FundType: rowTextDefault(r, "ftype", "FUNDTYPE"), DailyChange: strictNumber(rawDefault(r, "ZDF", "RZDF")), WeeklyReturn: strictNumber(r["SYL_Z"]), MonthlyReturn: strictNumber(r["SYL_Y"]), QuarterlyReturn: strictNumber(r["SYL_3Y"]), YearlyReturn: strictNumber(r["SYL_1N"]), NAV: strictNumber(rawDefault(r, "dwjz", "DWJZ")), ThemeCode: code})
	}
	return out, nil
}

func rawDefault(r emRow, primary, fallback string) json.RawMessage {
	if value, ok := r[primary]; ok && string(value) != "null" {
		return value
	}
	return r[fallback]
}

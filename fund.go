package stockgo

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Gavin-Gong/stock-go/internal/jsvars"
	"github.com/Gavin-Gong/stock-go/internal/markettime"
	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type FundEndpoints struct{ PingzhongBase, Dividend, ThemeBase string }
type FundService struct {
	client    *transport.Client
	endpoints FundEndpoints
	Theme     *FundThemeService
}

func appendURLPath(base, path string) string {
	u, _ := url.Parse(base)
	u.Path = strings.TrimRight(u.Path, "/") + "/" + path
	return u.String()
}
func fundCode(code string) error {
	if !fundPattern.MatchString(code) {
		return argument("fund code must contain six digits")
	}
	return nil
}
func (s *FundService) vars(ctx context.Context, endpoint string, names ...string) (emRow, error) {
	body, err := s.client.Get(ctx, "eastmoney", endpoint, nil)
	if err != nil {
		return nil, err
	}
	values := jsvars.Extract(string(body), names)
	if len(values) == 0 {
		return nil, klineError(sdkerrors.UpstreamEmpty, "eastmoney", endpoint, "no requested JSON declarations found", nil)
	}
	return emRow(values), nil
}
func (s *FundService) ping(ctx context.Context, code string, names ...string) (emRow, error) {
	if err := fundCode(code); err != nil {
		return nil, err
	}
	return s.vars(ctx, appendURLPath(s.endpoints.PingzhongBase, code+".js"), names...)
}
func strictNumber(raw json.RawMessage) *float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	text := string(raw)
	if v, ok := rawString(raw); ok {
		text = v
	}
	text = strings.TrimSpace(text)
	if text == "" || text == "-" || text == "--" {
		return nil
	}
	if len(text) > 2 && text[0] == '0' && strings.ContainsRune("xXoObB", rune(text[1])) {
		n, err := strconv.ParseUint(text, 0, 64)
		if err != nil {
			return nil
		}
		out := float64(n)
		return &out
	}
	if numberPattern.FindString(text) != text {
		return nil
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}
func numericTimestamp(raw json.RawMessage) *int64 {
	if _, stringValue := rawString(raw); stringValue {
		return nil
	}
	v := strictNumber(raw)
	if v == nil || *v >= float64(math.MaxInt64) || *v < float64(math.MinInt64) {
		return nil
	}
	ms := int64(*v)
	return &ms
}
func shanghaiDate(ms int64) string {
	loc, _ := time.LoadLocation(markettime.CN)
	return time.UnixMilli(ms).In(loc).Format("2006-01-02")
}
func arrayRows(raw json.RawMessage) []emRow { var out []emRow; json.Unmarshal(raw, &out); return out }
func tupleRows(raw json.RawMessage) [][]json.RawMessage {
	var out [][]json.RawMessage
	json.Unmarshal(raw, &out)
	return out
}
func (s *FundService) NavHistory(ctx context.Context, code string) (*FundNavHistory, error) {
	vars, err := s.ping(ctx, code, "fS_code", "fS_name", "Data_netWorthTrend", "Data_ACWorthTrend")
	if err != nil {
		return nil, err
	}
	out := &FundNavHistory{Code: code, Name: nullableText(vars, "fS_name"), Items: []FundNavPoint{}}
	if value := nullableText(vars, "fS_code"); value != nil {
		out.Code = *value
	}
	acc := map[int64]*float64{}
	for _, row := range tupleRows(vars["Data_ACWorthTrend"]) {
		if len(row) < 2 {
			continue
		}
		if stamp := numericTimestamp(row[0]); stamp != nil {
			acc[*stamp] = strictNumber(row[1])
		}
	}
	for _, r := range arrayRows(vars["Data_netWorthTrend"]) {
		stamp := numericTimestamp(r["x"])
		if stamp == nil {
			continue
		}
		out.Items = append(out.Items, FundNavPoint{Date: shanghaiDate(*stamp), Timestamp: stamp, NAV: strictNumber(r["y"]), AccNAV: acc[*stamp], DailyReturn: strictNumber(r["equityReturn"]), UnitMoney: rowText(r, "unitMoney")})
	}
	return out, nil
}
func (s *FundService) RankHistory(ctx context.Context, code string) (*FundRankHistory, error) {
	vars, err := s.ping(ctx, code, "fS_code", "fS_name", "Data_rateInSimilarType", "Data_rateInSimilarPersent")
	if err != nil {
		return nil, err
	}
	out := &FundRankHistory{Code: code, Name: nullableText(vars, "fS_name"), Items: []FundRankPoint{}}
	if value := nullableText(vars, "fS_code"); value != nil {
		out.Code = *value
	}
	percent := map[int64]*float64{}
	for _, row := range tupleRows(vars["Data_rateInSimilarPersent"]) {
		if len(row) < 2 {
			continue
		}
		if stamp := numericTimestamp(row[0]); stamp != nil {
			percent[*stamp] = strictNumber(row[1])
		}
	}
	for _, r := range arrayRows(vars["Data_rateInSimilarType"]) {
		stamp := numericTimestamp(r["x"])
		if stamp == nil {
			continue
		}
		out.Items = append(out.Items, FundRankPoint{Date: shanghaiDate(*stamp), Timestamp: stamp, Rank: strictNumber(r["y"]), Total: strictNumber(r["sc"]), Percentile: percent[*stamp]})
	}
	return out, nil
}

type FundDividendOptions struct {
	Year, Page, MaxPages       int
	AllPages                   bool
	Code, FundType, Rank, Sort string
}

func (s *FundService) DividendList(ctx context.Context, options ...FundDividendOptions) (*FundDividendListResult, error) {
	if len(options) > 1 {
		return nil, argument("at most one FundDividendOptions is allowed")
	}
	var o FundDividendOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.Code != "" {
		if err := fundCode(o.Code); err != nil {
			return nil, err
		}
	}
	if o.Year < 0 || o.Page < 0 || o.MaxPages < 0 {
		return nil, argument("invalid dividend options")
	}
	if o.Year == 0 {
		loc, _ := time.LoadLocation(markettime.CN)
		o.Year = time.Now().In(loc).Year()
	}
	if o.Page == 0 {
		o.Page = 1
	}
	if o.MaxPages == 0 {
		o.MaxPages = 1000
	}
	if o.Sort == "" {
		o.Sort = "desc"
	}
	if o.Sort != "asc" && o.Sort != "desc" {
		return nil, argument("sort must be asc or desc")
	}
	if o.Rank == "" {
		o.Rank = "FSRQ"
	}
	switch o.Rank {
	case "BZDM", "ABBNAME", "DJR", "FSRQ", "FHFCZ", "FFR":
	default:
		return nil, argument("invalid dividend rank")
	}
	fetch := func(page int) (*FundDividendListResult, error) {
		endpoint := endpointQuery(s.endpoints.Dividend, url.Values{"dt": {"8"}, "page": {numberString(page)}, "rank": {o.Rank}, "sort": {o.Sort}, "gs": {""}, "ftype": {o.FundType}, "year": {numberString(o.Year)}})
		vars, err := s.vars(ctx, endpoint, "pageinfo", "jjfh_data")
		if err != nil {
			return nil, err
		}
		info := []int{}
		json.Unmarshal(vars["pageinfo"], &info)
		out := &FundDividendListResult{Items: []FundDividend{}, CurrentPage: page}
		if len(info) > 0 {
			out.TotalPages = info[0]
		}
		if len(info) > 1 {
			out.PageSize = info[1]
		}
		if len(info) > 2 {
			out.CurrentPage = info[2]
		}
		for _, tuple := range tupleRows(vars["jjfh_data"]) {
			f := make([]string, len(tuple))
			for i, raw := range tuple {
				f[i], _ = rawString(raw)
			}
			kind := strings.TrimSpace(at(f, 6))
			var kindPtr *string
			if kind != "" {
				kindPtr = &kind
			}
			var amount json.RawMessage
			if len(tuple) > 4 {
				amount = tuple[4]
			}
			out.Items = append(out.Items, FundDividend{Code: at(f, 0), Name: at(f, 1), EquityRecordDate: dcDatePtr(strings.TrimSpace(at(f, 2))), ExDividendDate: dcDatePtr(strings.TrimSpace(at(f, 3))), DividendPerShare: strictNumber(amount), PayDate: dcDatePtr(strings.TrimSpace(at(f, 5))), DividendType: kindPtr})
		}
		return out, nil
	}
	page := o.Page
	if o.AllPages {
		page = 1
	}
	out, err := fetch(page)
	if err != nil {
		return nil, err
	}
	if o.AllPages {
		if out.TotalPages > o.MaxPages {
			return nil, klineError(sdkerrors.UpstreamError, "eastmoney", s.endpoints.Dividend, "fund dividend pagination exceeded maxPages", nil)
		}
		for p := 2; p <= out.TotalPages; p++ {
			next, err := fetch(p)
			if err != nil {
				return nil, err
			}
			out.Items = append(out.Items, next.Items...)
		}
		out.CurrentPage = -1
	}
	if o.Code != "" {
		items := []FundDividend{}
		for _, item := range out.Items {
			if item.Code == o.Code {
				items = append(items, item)
			}
		}
		out.Items = items
	}
	return out, nil
}

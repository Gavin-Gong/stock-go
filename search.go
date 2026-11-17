package stockgo

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Gavin-Gong/stock-go/internal/gbk"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type SearchResult struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Market   string `json:"market"`
	Type     string `json:"type"`
	Category string `json:"category"`
}

// NormalizeSearchType converts Tencent's raw asset classification to a stable category.
func NormalizeSearchType(raw string) string {
	u := strings.ToUpper(raw)
	for _, prefix := range []string{"QDII", "ETF", "LOF", "KJ", "JJ"} {
		if strings.HasPrefix(u, prefix) {
			return "fund"
		}
	}
	if strings.Contains(u, "FUND") {
		return "fund"
	}
	for _, kind := range []struct{ prefix, contains, category string }{{"GP", "STOCK", "stock"}, {"ZS", "INDEX", "index"}, {"ZQ", "BOND", "bond"}, {"QH", "FUTURE", "futures"}, {"QZ", "OPTION", "option"}} {
		if (kind.prefix == "ZS" && u == "ZS") || (kind.prefix != "ZS" && strings.HasPrefix(u, kind.prefix)) || strings.Contains(u, kind.contains) {
			return kind.category
		}
	}
	return "other"
}

var searchPattern = regexp.MustCompile(`\bv_hint\s*=\s*"((?:[^"\\]|\\.)*)"`)

// Search looks up a stock code, name or pinyin without executing upstream JavaScript.
func (s *SDK) Search(ctx context.Context, keyword string) ([]SearchResult, error) {
	out := []SearchResult{}
	if strings.TrimSpace(keyword) == "" {
		return out, nil
	}
	endpoint := endpointQuery(s.searchURL, url.Values{"v": {"2"}, "t": {"all"}, "q": {keyword}})
	body, err := s.client.Get(ctx, "tencent", endpoint, nil)
	if err != nil {
		return nil, err
	}
	// Smartbox normally emits ASCII with Unicode escapes; accept UTF-8 and GBK names too.
	if !utf8.Valid(body) {
		decoded, e := gbk.Decode(body)
		err = e
		body = []byte(decoded)
		if err != nil {
			return nil, &sdkerrors.Error{Code: sdkerrors.Parse, Provider: "tencent", URL: endpoint, Message: "invalid search encoding", Cause: err}
		}
	}
	m := searchPattern.FindSubmatch(body)
	if m == nil {
		return out, nil
	}
	var raw string
	if err := json.Unmarshal(append(append([]byte{'"'}, m[1]...), '"'), &raw); err != nil {
		return nil, &sdkerrors.Error{Code: sdkerrors.Parse, Provider: "tencent", URL: endpoint, Message: "invalid search string", Cause: err}
	}
	if raw == "N" {
		return out, nil
	}
	for _, record := range strings.Split(raw, "^") {
		if record == "" {
			continue
		}
		fields := strings.Split(record, "~")
		market, code, name, kind := at(fields, 0), at(fields, 1), at(fields, 2), at(fields, 4)
		out = append(out, SearchResult{market + code, name, market, kind, NormalizeSearchType(kind)})
	}
	return out, nil
}

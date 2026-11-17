package stockgo

import (
	"github.com/Gavin-Gong/stock-go/symbols"
	"net/url"
	"regexp"
	"strings"
)

type ExternalLink struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

var globalSecidPrefix = regexp.MustCompile(`^\d{3}\.`)

func searchLinkTarget(result SearchResult) (code, market string) {
	raw := strings.TrimSpace(result.Code)
	market = strings.ToLower(strings.TrimSpace(result.Market))
	if m := map[string]string{"0": "sz", "1": "sh", "100": "global", "105": "us", "116": "hk"}[result.Market]; m != "" {
		market = m
	}
	if market == "global" {
		return strings.ToUpper(globalSecidPrefix.ReplaceAllString(raw, "")), market
	}
	hint := symbols.Hint{}
	switch market {
	case "sh", "sz":
		hint.Market = symbols.CN
	case "hk":
		hint.Market = symbols.HK
	case "us":
		hint.Market = symbols.US
	default:
		market = ""
	}
	symbol, e := symbols.Normalize(raw, hint)
	if e != nil || symbol.AssetType != symbols.Stock {
		return raw, "unknown"
	}
	if market == "" {
		switch symbol.Market {
		case symbols.CN:
			if symbol.Exchange == "SSE" {
				market = "sh"
			} else if symbol.Exchange == "SZSE" {
				market = "sz"
			}
		case symbols.HK:
			market = "hk"
		case symbols.US:
			market = "us"
		case symbols.Global:
			market = "global"
		}
	}
	if market == "" {
		return raw, "unknown"
	}
	code = symbol.Code
	if symbol.Market == symbols.US {
		code = strings.ToUpper(code)
	}
	return code, market
}
func encodeURIComponent(value string) string {
	encoded := strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
	for _, pair := range [][2]string{{"%21", "!"}, {"%27", "'"}, {"%28", "("}, {"%29", ")"}, {"%2A", "*"}, {"%7E", "~"}} {
		encoded = strings.ReplaceAll(encoded, pair[0], pair[1])
	}
	return encoded
}
func GenerateSearchExternalLinks(result SearchResult) []ExternalLink {
	code, market := searchLinkTarget(result)
	east := "https://so.eastmoney.com/web/s?keyword=" + encodeURIComponent(code)
	xue := "https://xueqiu.com/k?q=" + encodeURIComponent(code)
	switch market {
	case "sh", "sz":
		page := market + code
		if market == "sh" && strings.HasPrefix(code, "000") || market == "sz" && strings.HasPrefix(code, "399") {
			page = "zs" + code
		}
		east = "https://quote.eastmoney.com/" + page + ".html"
		xue = "https://xueqiu.com/S/" + strings.ToUpper(market) + code
	case "hk", "us":
		east = "https://quote.eastmoney.com/" + market + "/" + code + ".html"
		xue = "https://xueqiu.com/S/" + code
	case "global":
		em, xq := code, code
		switch code {
		case "IXIC":
			em = "NDX"
			xq = ".IXIC"
		case "NDX", "NDX100":
			em = "NDX"
			xq = ".NDX"
		}
		east = "https://quote.eastmoney.com/gb/zs" + em + ".html"
		xue = "https://xueqiu.com/S/" + xq
	}
	return []ExternalLink{{"东方财富", east}, {"雪球", xue}}
}

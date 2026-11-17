// Package symbols normalizes user symbols and maps them to provider identifiers.
// It has no network dependencies.
package symbols

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type Market string

const (
	CN     Market = "CN"
	HK     Market = "HK"
	US     Market = "US"
	Global Market = "GLOBAL"
)

type AssetType string

const (
	Stock   AssetType = "stock"
	Index   AssetType = "index"
	Fund    AssetType = "fund"
	Bond    AssetType = "bond"
	Futures AssetType = "futures"
	Option  AssetType = "option"
	Board   AssetType = "board"
)

// Hint disambiguates a symbol. Conflicting explicit market/exchange/asset assertions fail.
type Hint struct {
	Market    Market
	Exchange  string
	AssetType AssetType
}
type Symbol struct {
	Market    Market    `json:"market"`
	AssetType AssetType `json:"assetType"`
	Exchange  string    `json:"exchange"`
	Code      string    `json:"code"`
	Variety   string    `json:"variety,omitempty"`
	Input     string    `json:"input"`
}
type venue struct {
	market   Market
	exchange string
}

var prefixes = map[string]venue{"sh": {CN, "SSE"}, "sz": {CN, "SZSE"}, "bj": {CN, "BSE"}, "hk": {HK, "HKEX"}, "us": {US, "US"}}
var secids = map[string]venue{"0": {CN, "SZSE"}, "1": {CN, "SSE"}, "116": {HK, "HKEX"}, "105": {US, "NASDAQ"}, "106": {US, "NYSE"}, "107": {US, "AMEX"}}
var futures = map[string]Market{"SHFE": CN, "DCE": CN, "CZCE": CN, "INE": CN, "CFFEX": CN, "GFEX": CN, "COMEX": Global, "NYMEX": Global, "CBOT": Global, "LME": Global}
var digits = regexp.MustCompile(`^\d+$`)
var ticker = regexp.MustCompile(`^[A-Za-z][A-Za-z.\-]*$`)
var canonical = regexp.MustCompile(`^[A-Z][A-Za-z0-9.\-]*$`)
var lowerLetters = regexp.MustCompile(`^[a-z]{3,}$`)
var letters = regexp.MustCompile(`[A-Za-z]`)
var variety = regexp.MustCompile(`^[A-Za-z]+`)
var csi = regexp.MustCompile(`^(93\d{4}|H\d{5})$`)

type special struct {
	venue
	prefix, emCode, tencent string
	collision               bool
}

var named = map[string]special{
	"HSHCI":  {venue{HK, "HSI"}, "124", "", "", false},
	"HSI":    {venue{HK, "HSI"}, "100", "", "hkHSI", true},
	"HSCEI":  {venue{HK, "HSI"}, "100", "", "hkHSCEI", true},
	"HSTECH": {venue{HK, "HSI"}, "", "", "hkHSTECH", true},
	"DJI":    {venue{US, "US"}, "100", "DJIA", "usDJI", true},
	"INX":    {venue{US, "US"}, "100", "SPX", "usINX", true},
	"IXIC":   {venue{US, "US"}, "100", "NDX", "usIXIC", true},
	"GDAXI":  {venue{Global, "DAX"}, "100", "", "", false},
}

func lookup(code string) (special, bool) {
	code = strings.ToUpper(code)
	if csi.MatchString(code) {
		return special{venue: venue{CN, "CSI"}, prefix: "2"}, true
	}
	s, ok := named[code]
	return s, ok
}
func InferAShareExchange(code string) string {
	if strings.HasPrefix(code, "92") || strings.HasPrefix(code, "4") || strings.HasPrefix(code, "8") {
		return "BSE"
	}
	if len(code) > 0 && strings.ContainsRune("659", rune(code[0])) {
		return "SSE"
	}
	return "SZSE"
}
func owner(exchange string) Market {
	for _, v := range prefixes {
		if v.exchange == exchange {
			return v.market
		}
	}
	for _, v := range secids {
		if v.exchange == exchange {
			return v.market
		}
	}
	if m, ok := futures[exchange]; ok {
		return m
	}
	switch exchange {
	case "CSI":
		return CN
	case "HSI":
		return HK
	case "DAX":
		return Global
	}
	return ""
}
func pad(code string) string {
	if len(code) < 5 {
		return strings.Repeat("0", 5-len(code)) + code
	}
	return code
}
func stockCode(code string, m Market) string {
	if m == HK && digits.MatchString(code) {
		return pad(code)
	}
	if m == US {
		return strings.ToUpper(code)
	}
	return code
}

// Normalize accepts a bare code, provider secid, exchange suffix, prefix, or futures contract.
// An optional hint supplies missing market, exchange or asset information.
func Normalize(input string, hints ...Hint) (Symbol, error) {
	var h Hint
	if len(hints) > 1 {
		return Symbol{}, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "at most one symbol hint is allowed"}
	}
	if len(hints) == 1 {
		h = hints[0]
	}
	raw := strings.TrimSpace(input)
	invalid := func(reason string) (Symbol, error) {
		return Symbol{}, &sdkerrors.Error{Code: sdkerrors.InvalidSymbol, Message: fmt.Sprintf("%q: %s", input, reason)}
	}
	finish := func(v venue, code string, asset AssetType, exCertain, assetCertain bool) (Symbol, error) {
		if h.Market != "" && h.Market != v.market {
			return invalid("conflicting market hint")
		}
		if h.Exchange != "" {
			if m := owner(h.Exchange); m != "" && m != v.market {
				return invalid("exchange belongs to another market")
			}
			if exCertain && h.Exchange != v.exchange {
				return invalid("conflicting exchange hint")
			}
			v.exchange = h.Exchange
		}
		if h.AssetType != "" && h.AssetType != asset {
			if assetCertain || h.AssetType == Board || h.AssetType == Futures {
				return invalid("conflicting asset type hint")
			}
			asset = h.AssetType
		}
		s := Symbol{Market: v.market, Exchange: v.exchange, Code: code, AssetType: asset, Input: input}
		if asset == Futures {
			s.Variety = strings.ToUpper(variety.FindString(code))
			if s.Variety == "" {
				s.Variety = strings.ToUpper(code)
			}
		}
		return s, nil
	}
	specialFinish := func(s special, code string) (Symbol, error) {
		return finish(s.venue, strings.ToUpper(code), Index, true, true)
	}
	strip := func(code string, v venue, lossy bool) (string, venue, error) {
		for _, p := range []string{"sh", "sz", "bj", "hk", "us"} {
			if strings.HasPrefix(strings.ToLower(code), p) && digits.MatchString(code[len(p):]) {
				pv := prefixes[p]
				if pv.market != v.market || (pv.exchange != v.exchange && !(lossy && pv.exchange == "BSE")) {
					return "", v, fmt.Errorf("conflicting redundant prefix")
				}
				return code[len(p):], pv, nil
			}
		}
		return code, v, nil
	}
	if raw == "" {
		return invalid("empty symbol")
	}
	if l, r, ok := strings.Cut(raw, "."); ok {
		if l == "" || r == "" {
			return invalid("empty dotted symbol component")
		}
		if s, ok := lookup(r); ok && s.prefix == l {
			return specialFinish(s, r)
		}
		if v, ok := secids[l]; ok {
			code, v, err := strip(r, v, l == "0")
			if err != nil {
				return invalid(err.Error())
			}
			certain := true
			if l == "0" && code == r && digits.MatchString(code) {
				ex := InferAShareExchange(code)
				if ex == "SZSE" || ex == "BSE" {
					v.exchange = ex
				}
				certain = false
			}
			return finish(v, stockCode(code, v.market), Stock, certain, false)
		}
		if v, ok := prefixes[strings.ToLower(r)]; ok {
			if s, ok := lookup(l); ok && !s.collision && (v.market == CN || v.market == HK) {
				return invalid("special index conflicts with suffix")
			}
			code, v, err := strip(l, v, false)
			if err != nil {
				return invalid(err.Error())
			}
			return finish(v, stockCode(code, v.market), Stock, v.exchange != "US", false)
		}
		if m, ok := futures[strings.ToUpper(l)]; ok {
			return finish(venue{m, strings.ToUpper(l)}, strings.ToUpper(r), Futures, true, true)
		}
		if l == "90" {
			return finish(venue{CN, "SSE"}, r, Board, true, true)
		}
	}
	for _, p := range []string{"sh", "sz", "bj", "hk", "us"} {
		if !strings.HasPrefix(strings.ToLower(raw), p) || len(raw) <= len(p) {
			continue
		}
		rest := raw[len(p):]
		v := prefixes[p]
		canon := strings.HasPrefix(raw, p) && (canonical.MatchString(rest) || lowerLetters.MatchString(rest))
		if !digits.MatchString(rest) && (v.market == CN || !canon) {
			continue
		}
		if s, ok := lookup(rest); ok {
			if s.collision && s.market == v.market && h.AssetType != Stock {
				return specialFinish(s, rest)
			}
			if !s.collision && (v.market == CN || v.market == HK) {
				return invalid("special index conflicts with prefix")
			}
		}
		code := rest
		if v.market == HK {
			code = pad(strings.ToUpper(rest))
		} else if v.market == US {
			code = strings.ToUpper(rest)
		}
		return finish(v, code, Stock, v.exchange != "US", false)
	}
	if s, ok := lookup(raw); ok && (!s.collision || (h.Market == s.market && h.AssetType != Stock)) {
		return specialFinish(s, raw)
	}
	if digits.MatchString(raw) {
		if h.Market == US {
			return finish(venue{US, "US"}, raw, Stock, false, false)
		}
		if h.Market == CN {
			return finish(venue{CN, InferAShareExchange(raw)}, raw, Stock, false, false)
		}
		if h.Market == HK || len(raw) == 4 || len(raw) == 5 {
			return finish(venue{HK, "HKEX"}, pad(raw), Stock, true, false)
		}
		return finish(venue{CN, InferAShareExchange(raw)}, raw, Stock, false, false)
	}
	if (h.AssetType == Futures || h.Market == Global) && letters.MatchString(raw) {
		ex := h.Exchange
		if ex == "" {
			if h.Market == Global {
				return invalid("GLOBAL futures require an exchange")
			}
			ex = "SHFE"
		}
		m := h.Market
		if m == "" {
			m = futures[ex]
			if m == "" {
				m = CN
			}
		}
		return finish(venue{m, ex}, strings.ToUpper(raw), Futures, false, true)
	}
	if ticker.MatchString(raw) {
		return finish(venue{US, "US"}, strings.ToUpper(raw), Stock, false, false)
	}
	return invalid("unrecognized symbol")
}

func mappingError(s Symbol, provider string) (string, error) {
	return "", &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: fmt.Sprintf("%s cannot map %s/%s/%s", provider, s.Exchange, s.AssetType, s.Code)}
}
func ToTencent(s Symbol) (string, error) {
	if s.AssetType == Board || s.AssetType == Futures || s.AssetType == Option {
		return mappingError(s, "tencent")
	}
	if s.AssetType == Index {
		if idx, ok := lookup(s.Code); ok && idx.exchange == s.Exchange && idx.tencent != "" {
			return idx.tencent, nil
		}
	}
	switch s.Market {
	case CN:
		p := map[string]string{"SSE": "sh", "SZSE": "sz", "BSE": "bj"}[s.Exchange]
		if p != "" {
			return p + s.Code, nil
		}
	case HK:
		if digits.MatchString(s.Code) {
			return "hk" + pad(s.Code), nil
		}
	case US:
		return "us" + s.Code, nil
	}
	return mappingError(s, "tencent")
}
func ToEastmoney(s Symbol) (string, error) {
	if s.AssetType == Board {
		return "90." + s.Code, nil
	}
	if s.AssetType == Futures || s.AssetType == Option {
		return mappingError(s, "eastmoney")
	}
	if s.AssetType == Index {
		if idx, ok := lookup(s.Code); ok && idx.exchange == s.Exchange {
			if idx.prefix == "" {
				return mappingError(s, "eastmoney")
			}
			code := idx.emCode
			if code == "" {
				code = s.Code
			}
			return idx.prefix + "." + code, nil
		}
	}
	if s.Market == HK {
		return "116." + pad(s.Code), nil
	}
	p := map[string]string{"SSE": "1", "SZSE": "0", "BSE": "0", "NASDAQ": "105", "NYSE": "106", "AMEX": "107", "US": "105"}[s.Exchange]
	if p == "" {
		return mappingError(s, "eastmoney")
	}
	return p + "." + s.Code, nil
}

// BatchResult preserves input order and reports codes unsupported by Tencent.
type BatchResult struct {
	Keys    []string
	Invalid []InvalidInput
}
type InvalidInput struct {
	Code   string
	Reason string
}

func TencentBatch(codes []string, market Market) BatchResult {
	out := BatchResult{Keys: []string{}, Invalid: []InvalidInput{}}
	seen := map[string]bool{}
	for _, code := range codes {
		s, err := Normalize(code, Hint{Market: market})
		var key string
		if err == nil {
			key, err = ToTencent(s)
		}
		if err != nil {
			out.Invalid = append(out.Invalid, InvalidInput{code, err.Error()})
			continue
		}
		if !seen[key] {
			seen[key] = true
			out.Keys = append(out.Keys, key)
		}
	}
	return out
}

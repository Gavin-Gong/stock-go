// Package stockgo provides the stock-go SDK for market data and analysis.
package stockgo

import (
	"net/http"
	"net/url"
	"time"

	"github.com/Gavin-Gong/stock-go/internal/memo"
	"github.com/Gavin-Gong/stock-go/internal/transport"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

// HTTPDoer supports caller-supplied clients and transports.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Options configures an SDK instance. Zero values select safe defaults.
type Options struct {
	HTTPClient   HTTPDoer
	Timeout      time.Duration
	MaxRetries   int
	BaseDelay    time.Duration
	MaxBodyBytes int64
	// Request configures rate limits, circuits, provider policies and hooks.
	Request *RequestOptions
	// TencentURL overrides https://qt.gtimg.cn (for a proxy or testing).
	TencentURL string
	// SearchURL and TimelineURL override the respective Tencent endpoints.
	SearchURL                          string
	TimelineURL                        string
	CodeURLs                           CodeEndpoints
	KlineURLs                          KlineEndpoints
	DisableKlineFallback               bool
	TradingCalendarURL                 string
	IndustryURLs, ConceptURLs          BoardEndpoints
	FundFlowURLs                       FundFlowEndpoints
	DatacenterURL, NorthboundMinuteURL string
	FundURLs                           FundEndpoints
	TopicBaseURL                       string
	FuturesURLs                        FuturesEndpoints
	OptionURLs                         OptionEndpoints
}

// SDK owns instance-scoped service clients. It is safe for concurrent use if HTTPClient is.
type SDK struct {
	Quotes      *QuoteService
	Codes       *CodeService
	Batch       *BatchService
	Kline       *KlineService
	Chips       *ChipService
	Board       *BoardService
	FundFlow    *FundFlowService
	Datacenter  *DatacenterService
	Calendar    *CalendarService
	Reference   *ReferenceService
	Northbound  *NorthboundService
	Fund        *FundService
	DragonTiger *DragonTigerService
	BlockTrade  *BlockTradeService
	Margin      *MarginService
	MarketEvent *MarketEventService
	Futures     *FuturesService
	Options     *OptionService
	Requests    *RequestClient
	client      *transport.Client
	searchURL   string
}

func New(options ...Options) (*SDK, error) {
	if len(options) > 1 {
		return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "at most one Options is allowed"}
	}
	var cfg Options
	if len(options) == 1 {
		cfg = options[0]
	}
	if cfg.TencentURL == "" {
		cfg.TencentURL = "https://qt.gtimg.cn"
	}
	if cfg.SearchURL == "" {
		cfg.SearchURL = "https://smartbox.gtimg.cn/s3/"
	}
	if cfg.TimelineURL == "" {
		cfg.TimelineURL = "https://web.ifzq.gtimg.cn/appstock/app/minute/query"
	}
	defaults := CodeEndpoints{CN: "https://assets.linkdiary.cn/shares/zh_a_list.json", HK: "https://assets.linkdiary.cn/shares/hk_list.json", US: "https://assets.linkdiary.cn/shares/us_list.json", Fund: "https://assets.linkdiary.cn/shares/fund_list"}
	endpoints := []*string{&cfg.CodeURLs.CN, &cfg.CodeURLs.HK, &cfg.CodeURLs.US, &cfg.CodeURLs.Fund}
	values := []string{defaults.CN, defaults.HK, defaults.US, defaults.Fund}
	for i, p := range endpoints {
		if *p == "" {
			*p = values[i]
		}
	}
	urls := []*string{&cfg.KlineURLs.CN, &cfg.KlineURLs.HK, &cfg.KlineURLs.US, &cfg.KlineURLs.Trends, &cfg.KlineURLs.HKTrends, &cfg.KlineURLs.USTrends, &cfg.KlineURLs.Tencent, &cfg.KlineURLs.TencentMinute, &cfg.KlineURLs.Sina}
	values = []string{"https://push2his.eastmoney.com/api/qt/stock/kline/get", "https://33.push2his.eastmoney.com/api/qt/stock/kline/get", "https://63.push2his.eastmoney.com/api/qt/stock/kline/get", "https://push2his.eastmoney.com/api/qt/stock/trends2/get", "https://33.push2his.eastmoney.com/api/qt/stock/trends2/get", "https://63.push2his.eastmoney.com/api/qt/stock/trends2/get", "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get", "https://ifzq.gtimg.cn/appstock/app/kline/mkline", "https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData"}
	for i, p := range urls {
		if *p == "" {
			*p = values[i]
		}
	}
	if cfg.TradingCalendarURL == "" {
		cfg.TradingCalendarURL = "https://assets.linkdiary.cn/shares/trade-data-list.txt"
	}
	all := []string{cfg.TencentURL, cfg.SearchURL, cfg.TimelineURL, cfg.TradingCalendarURL}
	all = append(all, configureExtensions(&cfg)...)
	all = append(all, configureDerivatives(&cfg)...)
	for _, p := range append(endpoints, urls...) {
		all = append(all, *p)
	}
	for _, endpoint := range all {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" {
			return nil, &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: "endpoint must be an absolute HTTP(S) URL without a fragment"}
		}
	}
	requestConfig := transport.Config{HTTPClient: cfg.HTTPClient, Timeout: cfg.Timeout, MaxRetries: cfg.MaxRetries, BaseDelay: cfg.BaseDelay, MaxBodyBytes: cfg.MaxBodyBytes}
	if cfg.Request != nil {
		requestConfig = *cfg.Request
		if cfg.HTTPClient != nil {
			requestConfig.HTTPClient = cfg.HTTPClient
		}
		if cfg.Timeout != 0 {
			requestConfig.Timeout = cfg.Timeout
		}
		if cfg.MaxRetries != 0 {
			requestConfig.MaxRetries = cfg.MaxRetries
		}
		if cfg.BaseDelay != 0 {
			requestConfig.BaseDelay = cfg.BaseDelay
		}
		if cfg.MaxBodyBytes != 0 {
			requestConfig.MaxBodyBytes = cfg.MaxBodyBytes
		}
	}
	client, err := transport.New(requestConfig)
	if err != nil {
		return nil, err
	}
	sdk := &SDK{client: client, Requests: client, searchURL: cfg.SearchURL}
	sdk.Quotes = &QuoteService{client: client, baseURL: cfg.TencentURL, timelineURL: cfg.TimelineURL}
	sdk.Codes = &CodeService{calendarURL: cfg.TradingCalendarURL, client: client, endpoints: cfg.CodeURLs, cache: memo.New[[]string](16)}
	sdk.Batch = &BatchService{quotes: sdk.Quotes, codes: sdk.Codes}
	probe := client.WithoutRetries()
	sdk.Kline = &KlineService{client: client, probe: probe, codes: sdk.Codes, endpoints: cfg.KlineURLs, fallback: !cfg.DisableKlineFallback, usCache: memo.New[string](4096)}
	attachExtensions(sdk, cfg)
	client.RegisterCacheClearer(sdk.clearServiceCaches)
	return sdk, nil
}

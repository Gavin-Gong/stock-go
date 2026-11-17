package stockgo

func configureDerivatives(o *Options) []string {
	defaults := []string{"https://push2his.eastmoney.com/api/qt/stock/kline/get", "https://futsseapi.eastmoney.com/list/COMEX,NYMEX,COBOT,SGX,NYBOT,LME,MDEX,TOCOM,IPE", "https://stock.finance.sina.com.cn/futures/api/openapi.php/OptionService.getOptionData", "https://stock.finance.sina.com.cn/futures/api/jsonp.php/{callback}/FutureOptionAllService.getOptionDayline", "https://stock.finance.sina.com.cn/futures/api/openapi.php/StockOptionService.getStockName", "https://stock.finance.sina.com.cn/futures/api/openapi.php/StockOptionService.getRemainderDay", "https://stock.finance.sina.com.cn/futures/api/openapi.php/StockOptionDaylineService.getOptionMinline", "https://stock.finance.sina.com.cn/futures/api/jsonp_v2.php/{callback}/StockOptionDaylineService.getSymbolInfo", "https://stock.finance.sina.com.cn/futures/api/openapi.php/StockOptionDaylineService.getFiveDayLine", "https://futsseapi.eastmoney.com/list/option/221", "https://datacenter-web.eastmoney.com/api/data/get"}
	out := []string{}
	for i, p := range []*string{&o.FuturesURLs.Kline, &o.FuturesURLs.GlobalSpot, &o.OptionURLs.Spot, &o.OptionURLs.Dayline, &o.OptionURLs.ETFMonths, &o.OptionURLs.ETFExpire, &o.OptionURLs.ETFMinute, &o.OptionURLs.ETFDaily, &o.OptionURLs.ETF5Day, &o.OptionURLs.CFFEX, &o.OptionURLs.LHB} {
		if *p == "" {
			*p = defaults[i]
		}
		out = append(out, *p)
	}
	return out
}
func attachDerivatives(s *SDK, o Options) {
	s.Futures = &FuturesService{client: s.client, dc: s.Datacenter, endpoints: o.FuturesURLs}
	s.Options = &OptionService{client: s.client, endpoints: o.OptionURLs}
}

package stockgo

import "github.com/Gavin-Gong/stock-go/internal/memo"

func configureExtensions(o *Options) []string {
	industry := []string{"https://17.push2.eastmoney.com/api/qt/clist/get", "https://91.push2.eastmoney.com/api/qt/stock/get", "https://29.push2.eastmoney.com/api/qt/clist/get", "https://7.push2his.eastmoney.com/api/qt/stock/kline/get", "https://push2his.eastmoney.com/api/qt/stock/trends2/get"}
	concept := []string{"https://79.push2.eastmoney.com/api/qt/clist/get", "https://91.push2.eastmoney.com/api/qt/stock/get", "https://29.push2.eastmoney.com/api/qt/clist/get", "https://91.push2his.eastmoney.com/api/qt/stock/kline/get", "https://push2his.eastmoney.com/api/qt/stock/trends2/get"}
	result := []string{}
	for i, b := range []*BoardEndpoints{&o.IndustryURLs, &o.ConceptURLs} {
		defaults := industry
		if i == 1 {
			defaults = concept
		}
		for index, p := range []*string{&b.List, &b.Spot, &b.Constituents, &b.Kline, &b.Trends} {
			if *p == "" {
				*p = defaults[index]
			}
			result = append(result, *p)
		}
	}
	defaults := []string{"https://push2his.eastmoney.com/api/qt/stock/fflow/daykline/get", "https://push2.eastmoney.com/api/qt/clist/get", "https://push2delay.eastmoney.com/api/qt/stock/get"}
	for i, p := range []*string{&o.FundFlowURLs.History, &o.FundFlowURLs.Rank, &o.FundFlowURLs.Realtime} {
		if *p == "" {
			*p = defaults[i]
		}
		result = append(result, *p)
	}
	extraDefaults := []string{"https://datacenter-web.eastmoney.com/api/data/v1/get", "https://push2.eastmoney.com/api/qt/kamtbs.rtmin/get", "https://fund.eastmoney.com/pingzhongdata", "https://fund.eastmoney.com/Data/funddataIndex_Interface.aspx", "https://fundmobapi.eastmoney.com/FundMNewApi", "https://push2ex.eastmoney.com"}
	for i, p := range []*string{&o.DatacenterURL, &o.NorthboundMinuteURL, &o.FundURLs.PingzhongBase, &o.FundURLs.Dividend, &o.FundURLs.ThemeBase, &o.TopicBaseURL} {
		if *p == "" {
			*p = extraDefaults[i]
		}
		result = append(result, *p)
	}
	return result
}
func attachExtensions(s *SDK, o Options) {
	s.Chips = &ChipService{kline: s.Kline}
	s.Board = &BoardService{Industry: &BoardProvider{client: s.client, endpoints: o.IndustryURLs, kind: "industry", names: memo.New[map[string]string](4)}, Concept: &BoardProvider{client: s.client, endpoints: o.ConceptURLs, kind: "concept", names: memo.New[map[string]string](4)}}
	s.FundFlow = &FundFlowService{client: s.client, endpoints: o.FundFlowURLs}
	s.Quotes.fundFlow = s.FundFlow
	attachReferenceAndFund(s, o)
	attachDerivatives(s, o)
}

func attachReferenceAndFund(s *SDK, o Options) {
	s.Datacenter = &DatacenterService{client: s.client, endpoint: o.DatacenterURL}
	s.DragonTiger = &DragonTigerService{dc: s.Datacenter}
	s.BlockTrade = &BlockTradeService{dc: s.Datacenter}
	s.Margin = &MarginService{dc: s.Datacenter}
	s.MarketEvent = &MarketEventService{client: s.client, dc: s.Datacenter, codes: s.Codes, base: o.TopicBaseURL}
	s.Calendar = &CalendarService{codes: s.Codes}
	s.Reference = &ReferenceService{codes: s.Codes, dc: s.Datacenter}
	s.Northbound = &NorthboundService{dc: s.Datacenter, minuteURL: o.NorthboundMinuteURL}
	s.Fund = &FundService{client: s.client, endpoints: o.FundURLs, Theme: &FundThemeService{client: s.client, base: o.FundURLs.ThemeBase}}
}

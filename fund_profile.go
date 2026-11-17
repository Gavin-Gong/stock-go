package stockgo

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Gavin-Gong/stock-go/internal/markettime"
)

type fundSeries struct {
	Categories []string
	Series     []struct {
		Name string
		Data []json.RawMessage
		Y    json.RawMessage
		Mom  string
	}
}

func parseFundSeries(raw json.RawMessage) fundSeries {
	var value fundSeries
	json.Unmarshal(raw, &value)
	return value
}
func (s fundSeries) named(name string) []json.RawMessage {
	for _, series := range s.Series {
		if series.Name == name {
			return series.Data
		}
	}
	return nil
}
func seriesNumber(data []json.RawMessage, i int) float64 {
	if i < len(data) {
		if v := strictNumber(data[i]); v != nil {
			return *v
		}
	}
	return 0
}
func parseHoldings(raw []string) []FundHolding {
	out := []FundHolding{}
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		market, code, ok := strings.Cut(value, ".")
		if !ok {
			code = value
			market = ""
		}
		out = append(out, FundHolding{Code: code, MarketID: market})
	}
	return out
}
func parsePerformance(raw json.RawMessage) *FundPerformanceEvaluation {
	var value struct {
		Avr        json.RawMessage
		Categories []string
		Data       []float64
		Dsc        []string
	}
	json.Unmarshal(raw, &value)
	if len(value.Categories) == 0 {
		return nil
	}
	overall := 0.0
	if n := strictNumber(value.Avr); n != nil {
		overall = *n
	}
	return &FundPerformanceEvaluation{Overall: overall, Categories: append([]string{}, value.Categories...), Scores: append([]float64{}, value.Data...), Descriptions: append([]string{}, value.Dsc...)}
}
func parsePeers(raw json.RawMessage) *FundSameType {
	out := &FundSameType{Groups: [][]FundSameTypePeer{}}
	for _, group := range tupleRows(raw) {
		peers := []FundSameTypePeer{}
		for _, entry := range group {
			value, ok := rawString(entry)
			if !ok {
				continue
			}
			first, last := strings.IndexByte(value, '_'), strings.LastIndexByte(value, '_')
			if first <= 0 {
				continue
			}
			peer := FundSameTypePeer{Code: value[:first], Name: value[first+1:]}
			if last > first {
				peer.Name = value[first+1 : last]
				encoded, _ := json.Marshal(value[last+1:])
				peer.Value = strictNumber(encoded)
			}
			peers = append(peers, peer)
		}
		if len(peers) > 0 {
			out.Groups = append(out.Groups, peers)
		}
	}
	if len(out.Groups) == 0 {
		return nil
	}
	return out
}
func (s *FundService) Profile(ctx context.Context, code string) (*FundProfile, error) {
	vars, err := s.ping(ctx, code, "fS_code", "fS_name", "fund_sourceRate", "fund_Rate", "fund_minsg", "stockCodesNew", "zqCodesNew", "Data_assetAllocation", "Data_fundSharesPositions", "Data_currentFundManager", "Data_performanceEvaluation", "Data_holderStructure", "Data_fluctuationScale", "Data_buySedemption", "syl_1y", "syl_3y", "syl_6y", "syl_1n", "swithSameType")
	if err != nil {
		return nil, err
	}
	return parseFundProfile(code, vars), nil
}
func parseFundProfile(code string, vars emRow) *FundProfile {
	out := &FundProfile{Code: code, Name: nullableText(vars, "fS_name"), SourceRate: strictNumber(vars["fund_sourceRate"]), Rate: strictNumber(vars["fund_Rate"]), MinSubscription: strictNumber(vars["fund_minsg"]), Holdings: []FundHolding{}, BondHoldings: []FundBondHolding{}, AssetAllocation: []FundAssetAllocation{}, Positions: []FundPositionPoint{}, Managers: []FundManager{}, Performance: parsePerformance(vars["Data_performanceEvaluation"]), HolderStructure: []FundHolderStructure{}, ScaleChanges: []FundScaleChange{}, BuySedemption: []FundBuySedemption{}, StageReturns: FundStageReturns{OneMonth: strictNumber(vars["syl_1y"]), ThreeMonth: strictNumber(vars["syl_3y"]), SixMonth: strictNumber(vars["syl_6y"]), OneYear: strictNumber(vars["syl_1n"])}, SameType: parsePeers(vars["swithSameType"])}
	if value := nullableText(vars, "fS_code"); value != nil {
		out.Code = *value
	}
	var stocks []string
	json.Unmarshal(vars["stockCodesNew"], &stocks)
	out.Holdings = parseHoldings(stocks)
	bonds := parseHoldings(strings.Split(rowText(vars, "zqCodesNew"), ","))
	for _, b := range bonds {
		out.BondHoldings = append(out.BondHoldings, FundBondHolding{Code: b.Code, MarketID: b.MarketID})
	}
	asset := parseFundSeries(vars["Data_assetAllocation"])
	if asset.Series != nil {
		stock, bond, cash, other, net := asset.named("股票占净比"), asset.named("债券占净比"), asset.named("现金占净比"), asset.named("其他占净比"), asset.named("净资产")
		for i, date := range asset.Categories {
			out.AssetAllocation = append(out.AssetAllocation, FundAssetAllocation{Date: date, Timestamp: markettime.Parse(date, markettime.CN), StockRatio: seriesNumber(stock, i), BondRatio: seriesNumber(bond, i), CashRatio: seriesNumber(cash, i), OtherRatio: seriesNumber(other, i), NetAsset: seriesNumber(net, i)})
		}
	}
	for _, tuple := range tupleRows(vars["Data_fundSharesPositions"]) {
		if len(tuple) == 0 {
			continue
		}
		stamp := numericTimestamp(tuple[0])
		if stamp == nil {
			continue
		}
		var pos *float64
		if len(tuple) > 1 {
			pos = strictNumber(tuple[1])
		}
		out.Positions = append(out.Positions, FundPositionPoint{Date: shanghaiDate(*stamp), Timestamp: *stamp, Position: pos})
	}
	for _, r := range arrayRows(vars["Data_currentFundManager"]) {
		var star *float64
		if _, str := rawString(r["star"]); !str {
			star = strictNumber(r["star"])
		}
		trimmed := func(key string) *string {
			value := strings.TrimSpace(rowText(r, key))
			if value == "" {
				return nil
			}
			return &value
		}
		out.Managers = append(out.Managers, FundManager{ID: rowText(r, "id"), Name: rowText(r, "name"), AvatarUrl: trimmed("pic"), Star: star, WorkTime: trimmed("workTime"), FundSize: trimmed("fundSize"), Power: parsePerformance(r["power"])})
	}
	holders := parseFundSeries(vars["Data_holderStructure"])
	if holders.Series != nil {
		institution, individual, internal := holders.named("机构持有比例"), holders.named("个人持有比例"), holders.named("内部持有比例")
		for i, date := range holders.Categories {
			out.HolderStructure = append(out.HolderStructure, FundHolderStructure{Date: date, Timestamp: markettime.Parse(date, markettime.CN), InstitutionRatio: seriesNumber(institution, i), IndividualRatio: seriesNumber(individual, i), InternalRatio: seriesNumber(internal, i)})
		}
	}
	scale := parseFundSeries(vars["Data_fluctuationScale"])
	if scale.Series != nil {
		for i, date := range scale.Categories {
			value, mom := 0.0, ""
			if i < len(scale.Series) {
				if n := strictNumber(scale.Series[i].Y); n != nil {
					value = *n
				}
				mom = scale.Series[i].Mom
			}
			out.ScaleChanges = append(out.ScaleChanges, FundScaleChange{Date: date, Scale: value, Mom: mom})
		}
	}
	buySell := parseFundSeries(vars["Data_buySedemption"])
	if buySell.Series != nil {
		buy, sell, total := buySell.named("期间申购"), buySell.named("期间赎回"), buySell.named("总份额")
		for i, date := range buySell.Categories {
			out.BuySedemption = append(out.BuySedemption, FundBuySedemption{Date: date, Timestamp: markettime.Parse(date, markettime.CN), Buy: seriesNumber(buy, i), Sell: seriesNumber(sell, i), Total: seriesNumber(total, i)})
		}
	}
	return out
}

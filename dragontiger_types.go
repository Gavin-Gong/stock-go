package stockgo

type DragonTigerDetailItem struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	Date             string   `json:"date"`
	Close            *float64 `json:"close"`
	ChangePercent    *float64 `json:"changePercent"`
	NetBuyAmount     *float64 `json:"netBuyAmount"`
	BuyAmount        *float64 `json:"buyAmount"`
	SellAmount       *float64 `json:"sellAmount"`
	DealAmount       *float64 `json:"dealAmount"`
	TotalAmount      *float64 `json:"totalAmount"`
	NetBuyRatio      *float64 `json:"netBuyRatio"`
	DealAmountRatio  *float64 `json:"dealAmountRatio"`
	TurnoverRate     *float64 `json:"turnoverRate"`
	FloatMarketValue *float64 `json:"floatMarketValue"`
	Reason           string   `json:"reason"`
	AfterChange1d    *float64 `json:"afterChange1d"`
	AfterChange2d    *float64 `json:"afterChange2d"`
	AfterChange5d    *float64 `json:"afterChange5d"`
	AfterChange10d   *float64 `json:"afterChange10d"`
}
type DragonTigerStockStatItem struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	LatestDate      string   `json:"latestDate"`
	Close           *float64 `json:"close"`
	ChangePercent   *float64 `json:"changePercent"`
	Count           *float64 `json:"count"`
	TotalBuyAmount  *float64 `json:"totalBuyAmount"`
	TotalSellAmount *float64 `json:"totalSellAmount"`
	TotalNetAmount  *float64 `json:"totalNetAmount"`
	TotalDealAmount *float64 `json:"totalDealAmount"`
	BuyOrgCount     *float64 `json:"buyOrgCount"`
	SellOrgCount    *float64 `json:"sellOrgCount"`
}
type DragonTigerInstitutionItem struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Date          string   `json:"date"`
	Close         *float64 `json:"close"`
	ChangePercent *float64 `json:"changePercent"`
	BuyOrgCount   *float64 `json:"buyOrgCount"`
	SellOrgCount  *float64 `json:"sellOrgCount"`
	OrgBuyAmount  *float64 `json:"orgBuyAmount"`
	OrgSellAmount *float64 `json:"orgSellAmount"`
	OrgNetAmount  *float64 `json:"orgNetAmount"`
}
type DragonTigerBranchItem struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	TotalBuyAmount  *float64 `json:"totalBuyAmount"`
	TotalSellAmount *float64 `json:"totalSellAmount"`
	BuyCount        *float64 `json:"buyCount"`
	SellCount       *float64 `json:"sellCount"`
	TotalCount      *float64 `json:"totalCount"`
}
type DragonTigerSeatItem struct {
	Rank            *float64 `json:"rank"`
	BranchName      string   `json:"branchName"`
	BuyAmount       *float64 `json:"buyAmount"`
	BuyAmountRatio  *float64 `json:"buyAmountRatio"`
	SellAmount      *float64 `json:"sellAmount"`
	SellAmountRatio *float64 `json:"sellAmountRatio"`
	NetAmount       *float64 `json:"netAmount"`
	Side            string   `json:"side"`
}

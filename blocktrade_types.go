package stockgo

type BlockTradeMarketStatItem struct {
	Date            string   `json:"date"`
	ShClose         *float64 `json:"shClose"`
	ShChangePercent *float64 `json:"shChangePercent"`
	TotalAmount     *float64 `json:"totalAmount"`
	PremiumAmount   *float64 `json:"premiumAmount"`
	PremiumRatio    *float64 `json:"premiumRatio"`
	DiscountAmount  *float64 `json:"discountAmount"`
	DiscountRatio   *float64 `json:"discountRatio"`
}
type BlockTradeDetailItem struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Date          string   `json:"date"`
	Close         *float64 `json:"close"`
	ChangePercent *float64 `json:"changePercent"`
	DealPrice     *float64 `json:"dealPrice"`
	DealVolume    *float64 `json:"dealVolume"`
	DealAmount    *float64 `json:"dealAmount"`
	PremiumRate   *float64 `json:"premiumRate"`
	BuyBranch     string   `json:"buyBranch"`
	SellBranch    string   `json:"sellBranch"`
}
type BlockTradeDailyStatItem struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Date            string   `json:"date"`
	ChangePercent   *float64 `json:"changePercent"`
	Close           *float64 `json:"close"`
	DealCount       *float64 `json:"dealCount"`
	DealTotalAmount *float64 `json:"dealTotalAmount"`
	DealTotalVolume *float64 `json:"dealTotalVolume"`
	PremiumAmount   *float64 `json:"premiumAmount"`
	DiscountAmount  *float64 `json:"discountAmount"`
}

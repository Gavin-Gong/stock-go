package stockgo

type OptionTQuote struct {
	Symbol       string   `json:"symbol"`
	BuyVolume    *float64 `json:"buyVolume"`
	BuyPrice     *float64 `json:"buyPrice"`
	Price        *float64 `json:"price"`
	AskPrice     *float64 `json:"askPrice"`
	AskVolume    *float64 `json:"askVolume"`
	OpenInterest *float64 `json:"openInterest"`
	Change       *float64 `json:"change"`
	StrikePrice  *float64 `json:"strikePrice"`
}

type OptionTQuoteResult struct {
	Calls []OptionTQuote `json:"calls"`
	Puts  []OptionTQuote `json:"puts"`
}

type OptionKline struct {
	Date   string   `json:"date"`
	Open   *float64 `json:"open"`
	High   *float64 `json:"high"`
	Low    *float64 `json:"low"`
	Close  *float64 `json:"close"`
	Volume *float64 `json:"volume"`
}

type OptionMinute struct {
	Time         string   `json:"time"`
	Date         string   `json:"date"`
	Price        *float64 `json:"price"`
	Volume       *float64 `json:"volume"`
	OpenInterest *float64 `json:"openInterest"`
	AvgPrice     *float64 `json:"avgPrice"`
}

type ETFOptionMonth struct {
	Months   []string `json:"months"`
	StockId  string   `json:"stockId"`
	CateId   string   `json:"cateId"`
	CateList []string `json:"cateList"`
}

type ETFOptionExpireDay struct {
	ExpireDay     string  `json:"expireDay"`
	RemainderDays float64 `json:"remainderDays"`
	StockId       string  `json:"stockId"`
	Name          string  `json:"name"`
}

type CFFEXOptionQuote struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Price         *float64 `json:"price"`
	Change        *float64 `json:"change"`
	ChangePercent *float64 `json:"changePercent"`
	Volume        *float64 `json:"volume"`
	Amount        *float64 `json:"amount"`
	OpenInterest  *float64 `json:"openInterest"`
	StrikePrice   *float64 `json:"strikePrice"`
	RemainDays    *float64 `json:"remainDays"`
	DailyChange   *float64 `json:"dailyChange"`
	PrevSettle    *float64 `json:"prevSettle"`
	Open          *float64 `json:"open"`
}

type OptionLHBItem struct {
	TradeType          string   `json:"tradeType"`
	Date               string   `json:"date"`
	Symbol             string   `json:"symbol"`
	TargetName         string   `json:"targetName"`
	Rank               float64  `json:"rank"`
	MemberName         string   `json:"memberName"`
	SellVolume         *float64 `json:"sellVolume"`
	SellVolumeChange   *float64 `json:"sellVolumeChange"`
	NetSellVolume      *float64 `json:"netSellVolume"`
	SellVolumeRatio    *float64 `json:"sellVolumeRatio"`
	BuyVolume          *float64 `json:"buyVolume"`
	BuyVolumeChange    *float64 `json:"buyVolumeChange"`
	NetBuyVolume       *float64 `json:"netBuyVolume"`
	BuyVolumeRatio     *float64 `json:"buyVolumeRatio"`
	SellPosition       *float64 `json:"sellPosition"`
	SellPositionChange *float64 `json:"sellPositionChange"`
	NetSellPosition    *float64 `json:"netSellPosition"`
	SellPositionRatio  *float64 `json:"sellPositionRatio"`
	BuyPosition        *float64 `json:"buyPosition"`
	BuyPositionChange  *float64 `json:"buyPositionChange"`
	NetBuyPosition     *float64 `json:"netBuyPosition"`
	BuyPositionRatio   *float64 `json:"buyPositionRatio"`
}

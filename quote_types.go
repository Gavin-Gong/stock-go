// Ported from stock-sdk c6c9a790870c5c607379e542e214a02ea05979c4 (ISC).
// Quote types follow stock-sdk v2; optional numerical fields encode as JSON null.

package stockgo

import "github.com/Gavin-Gong/stock-go/symbols"

type PriceLevel struct {
	Price  float64 `json:"price"`
	Volume float64 `json:"volume"`
}

type FullQuote struct {
	MarketId             string            `json:"marketId"`
	Name                 string            `json:"name"`
	Code                 string            `json:"code"`
	Price                float64           `json:"price"`
	PrevClose            float64           `json:"prevClose"`
	Open                 float64           `json:"open"`
	Volume               float64           `json:"volume"`
	OuterVolume          float64           `json:"outerVolume"`
	InnerVolume          float64           `json:"innerVolume"`
	Bid                  []PriceLevel      `json:"bid"`
	Ask                  []PriceLevel      `json:"ask"`
	Time                 string            `json:"time"`
	Timestamp            *int64            `json:"timestamp"`
	TZ                   string            `json:"tz"`
	Change               float64           `json:"change"`
	ChangePercent        float64           `json:"changePercent"`
	High                 float64           `json:"high"`
	Low                  float64           `json:"low"`
	Volume2              float64           `json:"volume2"`
	Amount               float64           `json:"amount"`
	TurnoverRate         *float64          `json:"turnoverRate"`
	PE                   *float64          `json:"pe"`
	Amplitude            *float64          `json:"amplitude"`
	CirculatingMarketCap *float64          `json:"circulatingMarketCap"`
	TotalMarketCap       *float64          `json:"totalMarketCap"`
	PB                   *float64          `json:"pb"`
	LimitUp              *float64          `json:"limitUp"`
	LimitDown            *float64          `json:"limitDown"`
	VolumeRatio          *float64          `json:"volumeRatio"`
	AvgPrice             *float64          `json:"avgPrice"`
	PEStatic             *float64          `json:"peStatic"`
	PEDynamic            *float64          `json:"peDynamic"`
	High52w              *float64          `json:"high52w"`
	Low52w               *float64          `json:"low52w"`
	CirculatingShares    *float64          `json:"circulatingShares"`
	TotalShares          *float64          `json:"totalShares"`
	Market               symbols.Market    `json:"market"`
	AssetType            symbols.AssetType `json:"assetType"`
	Source               string            `json:"source"`
}

type SimpleQuote struct {
	MarketId      string            `json:"marketId"`
	Name          string            `json:"name"`
	Code          string            `json:"code"`
	Price         float64           `json:"price"`
	Change        float64           `json:"change"`
	ChangePercent float64           `json:"changePercent"`
	Volume        float64           `json:"volume"`
	Amount        float64           `json:"amount"`
	MarketCap     *float64          `json:"marketCap"`
	MarketType    string            `json:"marketType"`
	Market        symbols.Market    `json:"market"`
	AssetType     symbols.AssetType `json:"assetType"`
	Source        string            `json:"source"`
}

type HKQuote struct {
	MarketId             string            `json:"marketId"`
	Name                 string            `json:"name"`
	Code                 string            `json:"code"`
	Price                float64           `json:"price"`
	PrevClose            float64           `json:"prevClose"`
	Open                 float64           `json:"open"`
	Volume               float64           `json:"volume"`
	Time                 string            `json:"time"`
	Timestamp            *int64            `json:"timestamp"`
	TZ                   string            `json:"tz"`
	Change               float64           `json:"change"`
	ChangePercent        float64           `json:"changePercent"`
	High                 float64           `json:"high"`
	Low                  float64           `json:"low"`
	Amount               float64           `json:"amount"`
	LotSize              *float64          `json:"lotSize"`
	CirculatingMarketCap *float64          `json:"circulatingMarketCap"`
	TotalMarketCap       *float64          `json:"totalMarketCap"`
	Currency             string            `json:"currency"`
	Market               symbols.Market    `json:"market"`
	AssetType            symbols.AssetType `json:"assetType"`
	Source               string            `json:"source"`
}

type USQuote struct {
	MarketId       string            `json:"marketId"`
	Name           string            `json:"name"`
	Code           string            `json:"code"`
	Price          float64           `json:"price"`
	PrevClose      float64           `json:"prevClose"`
	Open           float64           `json:"open"`
	Volume         float64           `json:"volume"`
	Time           string            `json:"time"`
	Timestamp      *int64            `json:"timestamp"`
	TZ             string            `json:"tz"`
	Change         float64           `json:"change"`
	ChangePercent  float64           `json:"changePercent"`
	High           float64           `json:"high"`
	Low            float64           `json:"low"`
	Amount         float64           `json:"amount"`
	TurnoverRate   *float64          `json:"turnoverRate"`
	PE             *float64          `json:"pe"`
	Amplitude      *float64          `json:"amplitude"`
	TotalMarketCap *float64          `json:"totalMarketCap"`
	PB             *float64          `json:"pb"`
	High52w        *float64          `json:"high52w"`
	Low52w         *float64          `json:"low52w"`
	Market         symbols.Market    `json:"market"`
	AssetType      symbols.AssetType `json:"assetType"`
	Source         string            `json:"source"`
}

type FundQuote struct {
	Code      string            `json:"code"`
	Name      string            `json:"name"`
	NAV       float64           `json:"nav"`
	AccNAV    float64           `json:"accNav"`
	Change    float64           `json:"change"`
	NavDate   string            `json:"navDate"`
	Timestamp *int64            `json:"timestamp"`
	TZ        string            `json:"tz"`
	Market    symbols.Market    `json:"market"`
	AssetType symbols.AssetType `json:"assetType"`
	Source    string            `json:"source"`
}

type PanelLargeOrder struct {
	BuyLargeRatio  float64 `json:"buyLargeRatio"`
	BuySmallRatio  float64 `json:"buySmallRatio"`
	SellLargeRatio float64 `json:"sellLargeRatio"`
	SellSmallRatio float64 `json:"sellSmallRatio"`
}

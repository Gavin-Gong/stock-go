// Ported from stock-sdk c6c9a790870c5c607379e542e214a02ea05979c4 (ISC).
package stockgo

import (
	"github.com/Gavin-Gong/stock-go/internal/markettime"
	"github.com/Gavin-Gong/stock-go/symbols"
)

func parseFullQuote(f []string) FullQuote {
	q := FullQuote{
		Price:                num(at(f, 3)),
		Volume:               num(at(f, 6)),
		PrevClose:            num(at(f, 4)),
		Open:                 num(at(f, 5)),
		OuterVolume:          num(at(f, 7)),
		InnerVolume:          num(at(f, 8)),
		Change:               num(at(f, 31)),
		ChangePercent:        num(at(f, 32)),
		High:                 num(at(f, 33)),
		Low:                  num(at(f, 34)),
		Volume2:              num(at(f, 36)),
		Amount:               num(at(f, 37)),
		TurnoverRate:         nullable(at(f, 38)),
		PE:                   nullable(at(f, 39)),
		Amplitude:            nullable(at(f, 43)),
		CirculatingMarketCap: nullable(at(f, 44)),
		TotalMarketCap:       nullable(at(f, 45)),
		PB:                   nullable(at(f, 46)),
		LimitUp:              nullable(at(f, 47)),
		LimitDown:            nullable(at(f, 48)),
		VolumeRatio:          nullable(at(f, 49)),
		AvgPrice:             nullable(at(f, 51)),
		PEStatic:             nullable(at(f, 52)),
		PEDynamic:            nullable(at(f, 53)),
		High52w:              nullable(at(f, 67)),
		Low52w:               nullable(at(f, 68)),
		CirculatingShares:    nullable(at(f, 72)),
		TotalShares:          nullable(at(f, 73)),
		MarketId:             at(f, 0),
		Name:                 at(f, 1),
		Code:                 at(f, 2),
		Market:               symbols.CN,
		AssetType:            symbols.Stock,
		Source:               "tencent",
		Time:                 at(f, 30),
		Timestamp:            markettime.Parse(at(f, 30), markettime.CN),
		TZ:                   markettime.CN,
	}
	q.Bid = make([]PriceLevel, 5)
	q.Ask = make([]PriceLevel, 5)
	for i := 0; i < 5; i++ {
		q.Bid[i] = PriceLevel{num(at(f, 9+i*2)), num(at(f, 10+i*2))}
		q.Ask[i] = PriceLevel{num(at(f, 19+i*2)), num(at(f, 20+i*2))}
	}
	return q
}

func parseSimpleQuote(f []string) SimpleQuote {
	q := SimpleQuote{
		Price:         num(at(f, 3)),
		Change:        num(at(f, 4)),
		ChangePercent: num(at(f, 5)),
		Volume:        num(at(f, 6)),
		Amount:        num(at(f, 7)),
		MarketCap:     nullable(at(f, 9)),
		MarketId:      at(f, 0),
		Name:          at(f, 1),
		Code:          at(f, 2),
		MarketType:    at(f, 10),
		Market:        symbols.CN,
		AssetType:     symbols.Stock,
		Source:        "tencent",
	}
	return q
}

func parseHKQuote(f []string) HKQuote {
	q := HKQuote{
		Price:                num(at(f, 3)),
		PrevClose:            num(at(f, 4)),
		Open:                 num(at(f, 5)),
		Volume:               num(at(f, 6)),
		Change:               num(at(f, 31)),
		ChangePercent:        num(at(f, 32)),
		High:                 num(at(f, 33)),
		Low:                  num(at(f, 34)),
		Amount:               num(at(f, 37)),
		LotSize:              nullable(at(f, 40)),
		CirculatingMarketCap: nullable(at(f, 44)),
		TotalMarketCap:       nullable(at(f, 45)),
		MarketId:             at(f, 0),
		Name:                 at(f, 1),
		Market:               symbols.HK,
		AssetType:            symbols.Stock,
		Source:               "tencent",
		Time:                 at(f, 30),
		Timestamp:            markettime.Parse(at(f, 30), markettime.HK),
		TZ:                   markettime.HK,
	}
	q.Code, q.AssetType = quoteMeta(at(f, 2), q.Market)
	if c := at(f, len(f)-3); currencyPattern.MatchString(c) {
		q.Currency = c
	}
	return q
}

func parseUSQuote(f []string) USQuote {
	q := USQuote{
		Price:          num(at(f, 3)),
		PrevClose:      num(at(f, 4)),
		Open:           num(at(f, 5)),
		Volume:         num(at(f, 6)),
		Change:         num(at(f, 31)),
		ChangePercent:  num(at(f, 32)),
		High:           num(at(f, 33)),
		Low:            num(at(f, 34)),
		Amount:         num(at(f, 37)),
		TurnoverRate:   nullable(at(f, 38)),
		PE:             nullable(at(f, 39)),
		Amplitude:      nullable(at(f, 43)),
		TotalMarketCap: nullable(at(f, 45)),
		PB:             nullable(at(f, 47)),
		High52w:        nullable(at(f, 48)),
		Low52w:         nullable(at(f, 49)),
		MarketId:       at(f, 0),
		Name:           at(f, 1),
		Market:         symbols.US,
		AssetType:      symbols.Stock,
		Source:         "tencent",
		Time:           at(f, 30),
		Timestamp:      markettime.Parse(at(f, 30), markettime.US),
		TZ:             markettime.US,
	}
	q.Code, q.AssetType = quoteMeta(at(f, 2), q.Market)
	return q
}

func parseFundQuote(f []string) FundQuote {
	q := FundQuote{
		NAV:       num(at(f, 5)),
		AccNAV:    num(at(f, 6)),
		Change:    num(at(f, 7)),
		Code:      at(f, 0),
		Name:      at(f, 1),
		Market:    symbols.CN,
		AssetType: symbols.Fund,
		Source:    "tencent",
		NavDate:   at(f, 8),
		Timestamp: markettime.Parse(at(f, 8), markettime.CN),
		TZ:        markettime.CN,
	}
	return q
}

func parsePanelLargeOrder(f []string) PanelLargeOrder {
	q := PanelLargeOrder{
		BuyLargeRatio:  num(at(f, 0)),
		BuySmallRatio:  num(at(f, 1)),
		SellLargeRatio: num(at(f, 2)),
		SellSmallRatio: num(at(f, 3)),
	}
	return q
}

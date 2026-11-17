package stockgo

type StockFundFlowDaily struct {
	Date                       string   `json:"date"`
	Close                      *float64 `json:"close"`
	ChangePercent              *float64 `json:"changePercent"`
	MainNetInflow              *float64 `json:"mainNetInflow"`
	MainNetInflowPercent       *float64 `json:"mainNetInflowPercent"`
	SuperLargeNetInflow        *float64 `json:"superLargeNetInflow"`
	SuperLargeNetInflowPercent *float64 `json:"superLargeNetInflowPercent"`
	LargeNetInflow             *float64 `json:"largeNetInflow"`
	LargeNetInflowPercent      *float64 `json:"largeNetInflowPercent"`
	MediumNetInflow            *float64 `json:"mediumNetInflow"`
	MediumNetInflowPercent     *float64 `json:"mediumNetInflowPercent"`
	SmallNetInflow             *float64 `json:"smallNetInflow"`
	SmallNetInflowPercent      *float64 `json:"smallNetInflowPercent"`
}
type FundFlowRankItem struct {
	Code                       string   `json:"code"`
	Name                       string   `json:"name"`
	Price                      *float64 `json:"price"`
	ChangePercent              *float64 `json:"changePercent"`
	MainNetInflow              *float64 `json:"mainNetInflow"`
	MainNetInflowPercent       *float64 `json:"mainNetInflowPercent"`
	SuperLargeNetInflow        *float64 `json:"superLargeNetInflow"`
	SuperLargeNetInflowPercent *float64 `json:"superLargeNetInflowPercent"`
	LargeNetInflow             *float64 `json:"largeNetInflow"`
	LargeNetInflowPercent      *float64 `json:"largeNetInflowPercent"`
	MediumNetInflow            *float64 `json:"mediumNetInflow"`
	MediumNetInflowPercent     *float64 `json:"mediumNetInflowPercent"`
	SmallNetInflow             *float64 `json:"smallNetInflow"`
	SmallNetInflowPercent      *float64 `json:"smallNetInflowPercent"`
}
type SectorFundFlowItem struct {
	Code                 string   `json:"code"`
	Name                 string   `json:"name"`
	ChangePercent        *float64 `json:"changePercent"`
	MainNetInflow        *float64 `json:"mainNetInflow"`
	MainNetInflowPercent *float64 `json:"mainNetInflowPercent"`
	SuperLargeNetInflow  *float64 `json:"superLargeNetInflow"`
	LargeNetInflow       *float64 `json:"largeNetInflow"`
	MediumNetInflow      *float64 `json:"mediumNetInflow"`
	SmallNetInflow       *float64 `json:"smallNetInflow"`
	TopStockName         string   `json:"topStockName,omitempty"`
	TopStockCode         string   `json:"topStockCode,omitempty"`
}
type MarketFundFlow struct {
	Date                       string   `json:"date"`
	ShClose                    *float64 `json:"shClose"`
	ShChangePercent            *float64 `json:"shChangePercent"`
	SzClose                    *float64 `json:"szClose"`
	SzChangePercent            *float64 `json:"szChangePercent"`
	MainNetInflow              *float64 `json:"mainNetInflow"`
	MainNetInflowPercent       *float64 `json:"mainNetInflowPercent"`
	SuperLargeNetInflow        *float64 `json:"superLargeNetInflow"`
	SuperLargeNetInflowPercent *float64 `json:"superLargeNetInflowPercent"`
	LargeNetInflow             *float64 `json:"largeNetInflow"`
	LargeNetInflowPercent      *float64 `json:"largeNetInflowPercent"`
	MediumNetInflow            *float64 `json:"mediumNetInflow"`
	MediumNetInflowPercent     *float64 `json:"mediumNetInflowPercent"`
	SmallNetInflow             *float64 `json:"smallNetInflow"`
	SmallNetInflowPercent      *float64 `json:"smallNetInflowPercent"`
}

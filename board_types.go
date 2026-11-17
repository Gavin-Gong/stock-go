package stockgo

type BoardItem struct {
	Rank                      int      `json:"rank"`
	Name                      string   `json:"name"`
	Code                      string   `json:"code"`
	Price                     *float64 `json:"price"`
	Change                    *float64 `json:"change"`
	ChangePercent             *float64 `json:"changePercent"`
	TotalMarketCap            *float64 `json:"totalMarketCap"`
	TurnoverRate              *float64 `json:"turnoverRate"`
	RiseCount                 *float64 `json:"riseCount"`
	FallCount                 *float64 `json:"fallCount"`
	LeadingStock              *string  `json:"leadingStock"`
	LeadingStockChangePercent *float64 `json:"leadingStockChangePercent"`
}
type BoardSpot struct {
	Item  string   `json:"item"`
	Value *float64 `json:"value"`
}
type BoardConstituent struct {
	Rank          int      `json:"rank"`
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Price         *float64 `json:"price"`
	ChangePercent *float64 `json:"changePercent"`
	Change        *float64 `json:"change"`
	Volume        *float64 `json:"volume"`
	Amount        *float64 `json:"amount"`
	Amplitude     *float64 `json:"amplitude"`
	High          *float64 `json:"high"`
	Low           *float64 `json:"low"`
	Open          *float64 `json:"open"`
	PrevClose     *float64 `json:"prevClose"`
	TurnoverRate  *float64 `json:"turnoverRate"`
	PE            *float64 `json:"pe"`
	PB            *float64 `json:"pb"`
}

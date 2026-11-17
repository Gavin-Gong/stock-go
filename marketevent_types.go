package stockgo

type ZTPoolItem struct {
	Code                 string   `json:"code"`
	Name                 string   `json:"name"`
	Price                *float64 `json:"price"`
	ChangePercent        *float64 `json:"changePercent"`
	LimitPrice           *float64 `json:"limitPrice"`
	Amount               *float64 `json:"amount"`
	FloatMarketValue     *float64 `json:"floatMarketValue"`
	TotalMarketValue     *float64 `json:"totalMarketValue"`
	TurnoverRate         *float64 `json:"turnoverRate"`
	ContinuousBoardCount *float64 `json:"continuousBoardCount"`
	FirstBoardTime       *string  `json:"firstBoardTime"`
	LastBoardTime        *string  `json:"lastBoardTime"`
	BoardAmount          *float64 `json:"boardAmount"`
	SealAmount           *float64 `json:"sealAmount"`
	FailedCount          *float64 `json:"failedCount"`
	Industry             string   `json:"industry"`
	ZtStatistics         string   `json:"ztStatistics"`
	Amplitude            *float64 `json:"amplitude"`
	Speed                *float64 `json:"speed"`
}
type StockChangeItem struct {
	Time            string          `json:"time"`
	Code            string          `json:"code"`
	Name            string          `json:"name"`
	ChangeType      StockChangeType `json:"changeType"`
	TypeCode        string          `json:"typeCode"`
	ChangeTypeLabel string          `json:"changeTypeLabel"`
	Info            string          `json:"info"`
}
type IndividualStockChangeItem struct {
	Time            string          `json:"time"`
	TypeCode        string          `json:"typeCode"`
	ChangeType      StockChangeType `json:"changeType"`
	ChangeTypeLabel string          `json:"changeTypeLabel"`
	Price           *float64        `json:"price"`
	ChangePercent   *float64        `json:"changePercent"`
	Info            string          `json:"info"`
	V               *float64        `json:"v"`
}
type IndividualChangesDay struct {
	Date      string                      `json:"date"`
	Available bool                        `json:"available"`
	Code      string                      `json:"code"`
	Name      string                      `json:"name"`
	Changes   []IndividualStockChangeItem `json:"changes"`
}
type ChangeTypeCount struct {
	Count int    `json:"count"`
	Label string `json:"label"`
}
type BoardChangeItem struct {
	Name                   string             `json:"name"`
	ChangePercent          *float64           `json:"changePercent"`
	MainNetInflow          *float64           `json:"mainNetInflow"`
	TotalChangeCount       *float64           `json:"totalChangeCount"`
	TopStockCode           string             `json:"topStockCode"`
	TopStockName           string             `json:"topStockName"`
	TopStockDirection      string             `json:"topStockDirection"`
	ChangeTypeDistribution map[string]float64 `json:"changeTypeDistribution"`
}
type UnusualFluctuationItem struct {
	Code                string   `json:"code"`
	Name                string   `json:"name"`
	Date                string   `json:"date"`
	Timestamp           *int64   `json:"timestamp"`
	Tz                  string   `json:"tz"`
	Rule                string   `json:"rule"`
	Triggered           bool     `json:"triggered"`
	DeviationValue      *float64 `json:"deviationValue"`
	WindowDays          *float64 `json:"windowDays"`
	ChangePercent       *float64 `json:"changePercent"`
	Direction           string   `json:"direction"`
	TargetChangePercent *float64 `json:"targetChangePercent"`
}

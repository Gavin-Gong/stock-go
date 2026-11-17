package stockgo

type FundDividend struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	EquityRecordDate *string  `json:"equityRecordDate"`
	ExDividendDate   *string  `json:"exDividendDate"`
	DividendPerShare *float64 `json:"dividendPerShare"`
	PayDate          *string  `json:"payDate"`
	DividendType     *string  `json:"dividendType"`
}
type FundDividendListResult struct {
	Items       []FundDividend `json:"items"`
	TotalPages  int            `json:"totalPages"`
	PageSize    int            `json:"pageSize"`
	CurrentPage int            `json:"currentPage"`
}
type FundNavPoint struct {
	Date        string   `json:"date"`
	Timestamp   *int64   `json:"timestamp"`
	NAV         *float64 `json:"nav"`
	AccNAV      *float64 `json:"accNav"`
	DailyReturn *float64 `json:"dailyReturn"`
	UnitMoney   string   `json:"unitMoney"`
}
type FundNavHistory struct {
	Code  string         `json:"code"`
	Name  *string        `json:"name"`
	Items []FundNavPoint `json:"items"`
}
type FundRankPoint struct {
	Date       string   `json:"date"`
	Timestamp  *int64   `json:"timestamp"`
	Rank       *float64 `json:"rank"`
	Total      *float64 `json:"total"`
	Percentile *float64 `json:"percentile"`
}
type FundRankHistory struct {
	Code  string          `json:"code"`
	Name  *string         `json:"name"`
	Items []FundRankPoint `json:"items"`
}
type FundHolding struct {
	Code     string `json:"code"`
	MarketID string `json:"marketId"`
}
type FundBondHolding struct {
	Code     string `json:"code"`
	MarketID string `json:"marketId"`
}
type FundAssetAllocation struct {
	Date       string  `json:"date"`
	Timestamp  *int64  `json:"timestamp"`
	StockRatio float64 `json:"stockRatio"`
	BondRatio  float64 `json:"bondRatio"`
	CashRatio  float64 `json:"cashRatio"`
	OtherRatio float64 `json:"otherRatio"`
	NetAsset   float64 `json:"netAsset"`
}
type FundPositionPoint struct {
	Date      string   `json:"date"`
	Timestamp int64    `json:"timestamp"`
	Position  *float64 `json:"position"`
}
type FundManager struct {
	ID        string                     `json:"id"`
	Name      string                     `json:"name"`
	AvatarUrl *string                    `json:"avatarUrl"`
	Star      *float64                   `json:"star"`
	WorkTime  *string                    `json:"workTime"`
	FundSize  *string                    `json:"fundSize"`
	Power     *FundPerformanceEvaluation `json:"power"`
}
type FundPerformanceEvaluation struct {
	Overall      float64   `json:"overall"`
	Categories   []string  `json:"categories"`
	Scores       []float64 `json:"scores"`
	Descriptions []string  `json:"descriptions"`
}
type FundHolderStructure struct {
	Date             string  `json:"date"`
	Timestamp        *int64  `json:"timestamp"`
	InstitutionRatio float64 `json:"institutionRatio"`
	IndividualRatio  float64 `json:"individualRatio"`
	InternalRatio    float64 `json:"internalRatio"`
}
type FundScaleChange struct {
	Date  string  `json:"date"`
	Scale float64 `json:"scale"`
	Mom   string  `json:"mom"`
}
type FundBuySedemption struct {
	Date      string  `json:"date"`
	Timestamp *int64  `json:"timestamp"`
	Buy       float64 `json:"buy"`
	Sell      float64 `json:"sell"`
	Total     float64 `json:"total"`
}
type FundStageReturns struct {
	OneMonth   *float64 `json:"oneMonth"`
	ThreeMonth *float64 `json:"threeMonth"`
	SixMonth   *float64 `json:"sixMonth"`
	OneYear    *float64 `json:"oneYear"`
}
type FundSameTypePeer struct {
	Code  string   `json:"code"`
	Name  string   `json:"name"`
	Value *float64 `json:"value"`
}
type FundSameType struct {
	Groups [][]FundSameTypePeer `json:"groups"`
}
type FundProfile struct {
	Code            string                     `json:"code"`
	Name            *string                    `json:"name"`
	SourceRate      *float64                   `json:"sourceRate"`
	Rate            *float64                   `json:"rate"`
	MinSubscription *float64                   `json:"minSubscription"`
	Holdings        []FundHolding              `json:"holdings"`
	BondHoldings    []FundBondHolding          `json:"bondHoldings"`
	AssetAllocation []FundAssetAllocation      `json:"assetAllocation"`
	Positions       []FundPositionPoint        `json:"positions"`
	Managers        []FundManager              `json:"managers"`
	Performance     *FundPerformanceEvaluation `json:"performance"`
	HolderStructure []FundHolderStructure      `json:"holderStructure"`
	ScaleChanges    []FundScaleChange          `json:"scaleChanges"`
	BuySedemption   []FundBuySedemption        `json:"buySedemption"`
	StageReturns    FundStageReturns           `json:"stageReturns"`
	SameType        *FundSameType              `json:"sameType"`
}
type ThemeFund struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	DailyChange     *float64 `json:"dailyChange"`
	WeeklyReturn    *float64 `json:"weeklyReturn"`
	MonthlyReturn   *float64 `json:"monthlyReturn"`
	QuarterlyReturn *float64 `json:"quarterlyReturn"`
	HalfYearReturn  *float64 `json:"halfYearReturn"`
	YearlyReturn    *float64 `json:"yearlyReturn"`
	ThreeYearReturn *float64 `json:"threeYearReturn"`
	FiveYearReturn  *float64 `json:"fiveYearReturn"`
	Type            string   `json:"type"`
}
type ThemeFundListResult struct {
	CatalogOnly bool        `json:"catalogOnly,omitempty"`
	Items       []ThemeFund `json:"items"`
	TotalPages  int         `json:"totalPages"`
	PageSize    int         `json:"pageSize"`
	CurrentPage int         `json:"currentPage"`
}
type ThemeFundItem struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	FundType        string   `json:"fundType"`
	DailyChange     *float64 `json:"dailyChange"`
	WeeklyReturn    *float64 `json:"weeklyReturn"`
	MonthlyReturn   *float64 `json:"monthlyReturn"`
	QuarterlyReturn *float64 `json:"quarterlyReturn"`
	YearlyReturn    *float64 `json:"yearlyReturn"`
	NAV             *float64 `json:"nav"`
	ThemeCode       string   `json:"themeCode"`
	ThemeName       string   `json:"themeName,omitempty"`
}
type ThemeFundItemList struct {
	Items     []ThemeFundItem `json:"items"`
	Total     int             `json:"total"`
	PageIndex int             `json:"pageIndex"`
	PageSize  int             `json:"pageSize"`
}

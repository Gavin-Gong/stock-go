package stockgo

import (
	"context"
	"errors"
	"strings"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

// Only explicit report/column schema errors enable compatibility requests.
// Timeouts, cancellations and other upstream failures retain their original meaning.
func reportSchemaChanged(err error) bool {
	var e *sdkerrors.Error
	return errors.As(err, &e) && e.Code == sdkerrors.UpstreamError && strings.Contains(e.Message, "不存在") && (strings.Contains(e.Message, "报表配置") || strings.Contains(e.Message, "列"))
}
func compatibleReport[T any](ctx context.Context, dc *DatacenterService, legacy, modern DatacenterQuery, oldMapper, newMapper func(emRow) T) ([]T, error) {
	rows, err := mapReport(ctx, dc, legacy, oldMapper)
	if !reportSchemaChanged(err) {
		return rows, err
	}
	return mapReport(ctx, dc, modern, newMapper)
}
func scaleNumber(v *float64, scale float64) *float64 {
	if v != nil {
		*v *= scale
	}
	return v
}
func mapModernBranch(r emRow) DragonTigerBranchItem {
	return DragonTigerBranchItem{Code: rowText(r, "OPERATEDEPT_CODE"), Name: rowText(r, "OPERATEDEPT_NAME"), TotalBuyAmount: rowNumber(r, "ACT_BUY"), TotalSellAmount: rowNumber(r, "ACT_SELL"), BuyCount: rowNumber(r, "TOTAL_BUYER_SALESTIMES"), SellCount: rowNumber(r, "TOTAL_SELLER_SALESTIMES"), TotalCount: rowNumber(r, "SALES_ONLIST_TIMES")}
}
func mapModernBlockDetail(r emRow) BlockTradeDetailItem {
	item := mapBlockDetail(r)
	item.BuyBranch = rowText(r, "BUYER_NAME")
	item.SellBranch = rowText(r, "SELLER_NAME")
	item.PremiumRate = scaleNumber(rowNumber(r, "PREMIUM_RATIO"), 100)
	return item
}
func mapModernBlockDaily(r emRow) BlockTradeDailyStatItem {
	item := mapBlockDaily(r)
	item.DealTotalAmount = scaleNumber(rowNumber(r, "DEAL_AMT"), 10000)
	item.DealTotalVolume = scaleNumber(rowNumber(r, "VOLUME"), 10000)
	// The current summary has counts, not amounts, for premium/discount trades.
	item.PremiumAmount, item.DiscountAmount = nil, nil
	return item
}
func mapModernMarginTarget(r emRow) MarginTargetItem {
	return MarginTargetItem{Code: rowText(r, "SCODE"), Name: rowText(r, "SECNAME"), Date: dcDate(rowText(r, "DATE")), FinBalance: rowNumber(r, "RZYE"), FinBuyAmount: rowNumber(r, "RZMRE"), FinRepayAmount: rowNumber(r, "RZCHE"), LoanBalance: rowNumber(r, "RQYL"), LoanSellVolume: rowNumber(r, "RQMCL"), LoanRepayVolume: rowNumber(r, "RQCHL")}
}

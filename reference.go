package stockgo

import (
	"context"
	"regexp"

	"github.com/Gavin-Gong/stock-go/symbols"
)

type ReferenceService struct {
	codes *CodeService
	dc    *DatacenterService
}

var dcDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

func dcDate(raw string) string {
	if prefix := dcDatePattern.FindString(raw); prefix != "" {
		return prefix
	}
	return raw
}
func dcDatePtr(raw string) *string {
	if prefix := dcDatePattern.FindString(raw); prefix != "" {
		return &prefix
	}
	return nil
}
func nullableText(row emRow, key string) *string {
	if _, ok := row[key]; !ok || string(row[key]) == "null" {
		return nil
	}
	text := rowText(row, key)
	return &text
}
func pureCNCode(input string) (string, error) {
	ns, err := symbols.Normalize(input, symbols.Hint{Market: symbols.CN})
	if err != nil {
		return "", err
	}
	if !fundPattern.MatchString(ns.Code) {
		return "", argument("expected a six-digit CN security code")
	}
	return ns.Code, nil
}
func (s *ReferenceService) TradingCalendar(ctx context.Context) ([]string, error) {
	return s.codes.calendar(ctx)
}
func (s *ReferenceService) DividendDetail(ctx context.Context, symbol string) ([]DividendDetail, error) {
	code, err := pureCNCode(symbol)
	if err != nil {
		return nil, err
	}
	rows, err := s.dc.rows(ctx, DatacenterQuery{ReportName: "RPT_SHAREBONUS_DET", SortColumns: "REPORT_DATE", SortTypes: "-1", PageSize: 500, Filter: `(SECURITY_CODE="` + code + `")`})
	if err != nil {
		return nil, err
	}
	out := make([]DividendDetail, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapDividend(r))
	}
	return out, nil
}

func mapDividend(r emRow) DividendDetail {
	return DividendDetail{
		Code:                rowText(r, "SECURITY_CODE"),
		Name:                rowText(r, "SECURITY_NAME_ABBR"),
		ReportDate:          dcDatePtr(rowText(r, "REPORT_DATE")),
		PlanNoticeDate:      dcDatePtr(rowText(r, "PLAN_NOTICE_DATE")),
		DisclosureDate:      dcDatePtr(rowTextDefault(r, "PUBLISH_DATE", "PLAN_NOTICE_DATE")),
		AssignTransferRatio: rowNumber(r, "BONUS_IT_RATIO"),
		BonusRatio:          rowNumber(r, "BONUS_RATIO"),
		TransferRatio:       rowNumber(r, "IT_RATIO"),
		DividendPretax:      rowNumber(r, "PRETAX_BONUS_RMB"),
		DividendDesc:        nullableText(r, "IMPL_PLAN_PROFILE"),
		DividendYield:       rowNumber(r, "DIVIDENT_RATIO"),
		EPS:                 rowNumber(r, "BASIC_EPS"),
		BPS:                 rowNumber(r, "BVPS"),
		CapitalReserve:      rowNumber(r, "PER_CAPITAL_RESERVE"),
		UnassignedProfit:    rowNumber(r, "PER_UNASSIGN_PROFIT"),
		NetProfitYoy:        rowNumber(r, "PNP_YOY_RATIO"),
		TotalShares:         rowNumber(r, "TOTAL_SHARES"),
		EquityRecordDate:    dcDatePtr(rowText(r, "EQUITY_RECORD_DATE")),
		ExDividendDate:      dcDatePtr(rowText(r, "EX_DIVIDEND_DATE")),
		PayDate:             dcDatePtr(rowText(r, "PAY_DATE")),
		AssignProgress:      nullableText(r, "ASSIGN_PROGRESS"),
		NoticeDate:          dcDatePtr(rowText(r, "NOTICE_DATE")),
	}
}
func rowTextDefault(r emRow, primary, fallback string) string {
	if raw, ok := r[primary]; ok && string(raw) != "null" {
		return rowText(r, primary)
	}
	return rowText(r, fallback)
}

package stockgo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestModernReportSchemaCompatibility(t *testing.T) {
	for _, operation := range []string{"branch", "seat", "blockDetail", "blockDaily", "marginDate", "marginLatest"} {
		t.Run(operation, func(t *testing.T) {
			var mu sync.Mutex
			legacyCalls, modernCalls := 0, 0
			sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				report := q.Get("reportName")
				modern := false
				switch report {
				case "RPT_OPERATEDEPT_LIST_STATISTICS", "RPT_DATA_BLOCKTRADE", "RPT_BLOCKTRADE_STAINCLUDE", "RPTA_WEB_RZRQ_GGMX":
					modern = true
				case "RPT_BILLBOARD_DAILYDETAILSBUY", "RPT_BILLBOARD_DAILYDETAILSSELL":
					modern = q.Get("sortColumns") == "BUY" || q.Get("sortColumns") == "SELL"
				}
				mu.Lock()
				if modern {
					modernCalls++
				} else {
					legacyCalls++
				}
				mu.Unlock()
				if !modern {
					fmt.Fprint(w, `{"success":false,"message":"报表配置不存在或排序列不存在"}`)
					return
				}
				row := `{}`
				switch report {
				case "RPT_OPERATEDEPT_LIST_STATISTICS":
					if q.Get("filter") != `(STATISTICSCYCLE="04")` || q.Get("sortColumns") != "TOTAL_BUYER_SALESTIMES" {
						t.Error(q)
					}
					row = `{"OPERATEDEPT_CODE":"01","OPERATEDEPT_NAME":"营业部","ACT_BUY":100,"ACT_SELL":40,"TOTAL_BUYER_SALESTIMES":2,"TOTAL_SELLER_SALESTIMES":1,"SALES_ONLIST_TIMES":3}`
				case "RPT_DATA_BLOCKTRADE":
					if q.Get("filter") != `(TRADE_DATE>='2024-01-02')(TRADE_DATE<='2024-01-03')` {
						t.Error(q)
					}
					row = `{"SECURITY_CODE":"600519","DEAL_PRICE":10,"PREMIUM_RATIO":-0.05,"BUYER_NAME":"买方","SELLER_NAME":"卖方","DEAL_AMT":500000,"DEAL_VOLUME":50000}`
				case "RPT_BLOCKTRADE_STAINCLUDE":
					row = `{"DEAL_AMT":510.3,"VOLUME":45,"DEAL_NUM":2,"PREMIUM_TIMES":1,"DISCOUNT_TIMES":1}`
				case "RPTA_WEB_RZRQ_GGMX":
					if q.Get("sortColumns") == "DATE" {
						if q.Get("pageSize") != "1" {
							t.Error(q)
						}
						fmt.Fprint(w, `{"result":{"pages":1000,"data":[{"DATE":"2024-01-02 00:00:00"}]}}`)
						return
					}
					if q.Get("filter") != `(DATE='2024-01-02')` || q.Get("sortColumns") != "RZYE" {
						t.Error(q)
					}
					row = `{"DATE":"2024-01-02 00:00:00","SCODE":"600519","SECNAME":"茅台","RZYE":100,"RZMRE":20,"RZCHE":10,"RQYL":300,"RQYE":9999,"RQMCL":40,"RQCHL":30}`
				default:
					row = `{"OPERATEDEPT_NAME":"席位","BUY":123,"SELL":23,"NET":100,"TOTAL_BUYRIO":0.125,"TOTAL_SELLRIO":0.025}`
				}
				fmt.Fprintf(w, `{"result":{"pages":1,"count":1,"data":[%s]}}`, row)
			})
			ctx := context.Background()
			switch operation {
			case "branch":
				rows, e := sdk.DragonTiger.BranchRank(ctx, DragonYear)
				if e != nil || len(rows) != 1 {
					t.Fatal(rows, e)
				}
				r := rows[0]
				if r.Code != "01" || *r.TotalBuyAmount != 100 || *r.TotalSellAmount != 40 || *r.BuyCount != 2 || *r.SellCount != 1 || *r.TotalCount != 3 {
					t.Fatal(r)
				}
			case "seat":
				rows, e := sdk.DragonTiger.SeatDetail(ctx, "600519", "20240102")
				if e != nil || len(rows) != 2 {
					t.Fatal(rows, e)
				}
				for _, r := range rows {
					if *r.BuyAmount != 123 || *r.SellAmount != 23 || *r.NetAmount != 100 || *r.BuyAmountRatio != 12.5 || *r.SellAmountRatio != 2.5 {
						t.Fatal(r)
					}
				}
			case "blockDetail":
				rows, e := sdk.BlockTrade.Detail(ctx, BlockTradeDateOptions{StartDate: "20240102", EndDate: "20240103"})
				if e != nil || len(rows) != 1 {
					t.Fatal(rows, e)
				}
				r := rows[0]
				if r.BuyBranch != "买方" || r.SellBranch != "卖方" || *r.PremiumRate != -5 || *r.DealAmount != 500000 || *r.DealVolume != 50000 {
					t.Fatal(r)
				}
			case "blockDaily":
				rows, e := sdk.BlockTrade.DailyStat(ctx)
				if e != nil || len(rows) != 1 {
					t.Fatal(rows, e)
				}
				r := rows[0]
				if math.Abs(*r.DealTotalAmount-5103000) > 0.00001 || *r.DealTotalVolume != 450000 || *r.DealCount != 2 || r.PremiumAmount != nil || r.DiscountAmount != nil {
					t.Fatal(r)
				}
			case "marginDate", "marginLatest":
				var dates []string
				if operation == "marginDate" {
					dates = []string{"20240102"}
				}
				rows, e := sdk.Margin.TargetList(ctx, dates...)
				if e != nil || len(rows) != 1 {
					t.Fatal(rows, e)
				}
				r := rows[0]
				if r.Code != "600519" || r.Name != "茅台" || r.Date != "2024-01-02" || *r.FinBalance != 100 || *r.FinBuyAmount != 20 || *r.FinRepayAmount != 10 || *r.LoanBalance != 300 || *r.LoanSellVolume != 40 || *r.LoanRepayVolume != 30 {
					t.Fatal(r)
				}
			}
			want := 1
			if operation == "seat" {
				want = 2
			}
			if legacyCalls != want {
				t.Fatal(legacyCalls)
			}
			if operation == "marginLatest" {
				want = 2
			}
			if modernCalls != want {
				t.Fatal(modernCalls, want)
			}
		})
	}
}
func TestReportFallbackRetainsErrors(t *testing.T) {
	for _, mode := range []string{"unrelated", "replacementFailure", "latestEmpty", "latestInvalid", "latestFailure", "seatFailure"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			sdk := topicReportTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				report := r.URL.Query().Get("reportName")
				if strings.Contains(report, "MARGIN_TRADE_DETAIL") || strings.Contains(report, "DAILYDETAILS") {
					message := "排序列不存在"
					if mode == "unrelated" {
						message = "服务器繁忙"
					}
					fmt.Fprintf(w, `{"success":false,"message":"%s"}`, message)
					return
				}
				switch mode {
				case "latestEmpty":
					fmt.Fprint(w, `{"result":{"pages":1,"data":[]}}`)
				case "latestInvalid":
					fmt.Fprint(w, `{"result":{"pages":1,"data":[{"DATE":"invalid"}]}}`)
				default:
					fmt.Fprint(w, `{"success":false,"message":"replacement failed"}`)
				}
			})
			var result []MarginTargetItem
			var err error
			if mode == "seatFailure" {
				_, err = sdk.DragonTiger.SeatDetail(context.Background(), "600519", "20240102")
			} else if mode == "replacementFailure" {
				result, err = sdk.Margin.TargetList(context.Background(), "20240102")
			} else {
				result, err = sdk.Margin.TargetList(context.Background())
			}
			if mode == "latestEmpty" {
				if err != nil || result == nil || len(result) != 0 {
					t.Fatal(result, err)
				}
				return
			}
			if err == nil {
				t.Fatal("lost upstream failure")
			}
			if mode == "unrelated" && calls.Load() != 1 {
				t.Fatal("unrelated failure triggered fallback", calls.Load())
			}
		})
	}
	for _, e := range []error{nil, context.Canceled, errors.New("列不存在"), &sdkerrors.Error{Code: sdkerrors.Timeout, Message: "列不存在"}, &sdkerrors.Error{Code: sdkerrors.UpstreamError, Message: "服务器繁忙"}} {
		if reportSchemaChanged(e) {
			t.Fatal(e)
		}
	}
}

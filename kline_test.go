package stockgo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func klineTestSDK(t *testing.T, h http.HandlerFunc, disable bool, retries int) (*SDK, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	sdk, err := New(Options{KlineURLs: KlineEndpoints{CN: srv.URL + "/em", HK: srv.URL + "/hk", US: srv.URL + "/us", Trends: srv.URL + "/trends", HKTrends: srv.URL + "/hktrends", USTrends: srv.URL + "/ustrends", Tencent: srv.URL + "/tx", TencentMinute: srv.URL + "/txm", Sina: srv.URL + "/sina"}, CodeURLs: CodeEndpoints{US: srv.URL + "/codes"}, DisableKlineFallback: disable, MaxRetries: retries, BaseDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return sdk, srv
}
func assertNum(t *testing.T, n *float64, want float64) {
	t.Helper()
	if n == nil || *n != want {
		t.Fatalf("number=%v want=%v", n, want)
	}
}
func TestHistoryPrimaryMarkets(t *testing.T) {
	for _, tt := range []struct{ market, code, secid, zone, currency string }{{"cn", "600519.SH", "1.600519", "Asia/Shanghai", ""}, {"hk", "700.HK", "116.00700", "Asia/Hong_Kong", "HKD"}, {"us", "106.BABA", "106.BABA", "America/New_York", "USD"}, {"us", "100.DJIA", "100.DJIA", "America/New_York", "USD"}} {
		t.Run(tt.market+tt.code, func(t *testing.T) {
			sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("secid") != tt.secid || q.Get("fqt") != "2" || q.Get("klt") != "102" || q.Get("beg") != "20240101" || q.Get("end") != "20240131" || q.Get("ut") == "" || q.Get("fields2") == "" {
					t.Errorf("query=%v", q)
				}
				fmt.Fprint(w, `{"rc":0,"data":{"name":"name","klines":["2024-01-05,10,11,12,9,100,200,3,4,5,--",null,4]}}`)
			}, true, 0)
			o := KlineOptions{Period: "weekly", Adjust: AdjustHFQ, StartDate: "20240101", EndDate: "2024-01-31"}
			var rows []KlineBar
			var err error
			switch tt.market {
			case "cn":
				rows, err = sdk.Kline.CN(context.Background(), tt.code, o)
			case "hk":
				rows, err = sdk.Kline.HK(context.Background(), tt.code, o)
			case "us":
				rows, err = sdk.Kline.US(context.Background(), tt.code, o)
			}
			if err != nil || len(rows) != 1 {
				t.Fatalf("%v %v", rows, err)
			}
			b := rows[0]
			if b.Date != "2024-01-05" || b.TZ != tt.zone || b.Timestamp == nil || b.Currency != tt.currency || b.Source != "eastmoney" || b.Adjust != AdjustHFQ || b.TurnoverRate != nil {
				t.Fatalf("%+v", b)
			}
			assertNum(t, b.Open, 10)
			assertNum(t, b.ChangePercent, 4)
			if tt.secid == "100.DJIA" && b.Code != "DJI" {
				t.Fatal(b.Code)
			}
		})
	}
}
func TestUSCodeListResolutionAndCache(t *testing.T) {
	var listCalls, probes atomic.Int32
	sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/codes" {
			listCalls.Add(1)
			fmt.Fprint(w, `{"success":true,"list":["106.BABA"]}`)
			return
		}
		if r.URL.Query().Get("secid") != "106.BABA" {
			t.Error(r.URL.RawQuery)
		}
		if r.URL.Query().Get("lmt") == "1" {
			probes.Add(1)
		}
		fmt.Fprint(w, `{"data":{"klines":["2024-01-02,1,2,3,1,100"]}}`)
	}, true, 0)
	for i := 0; i < 2; i++ {
		if q, err := sdk.Kline.US(context.Background(), "BABA"); err != nil || len(q) != 1 {
			t.Fatalf("%v %v", q, err)
		}
	}
	if listCalls.Load() != 1 || probes.Load() != 0 {
		t.Fatal(listCalls.Load(), probes.Load())
	}
	sdk.ClearCaches()
	sdk.Kline.US(context.Background(), "BABA")
	if listCalls.Load() != 2 {
		t.Fatal("cache not cleared")
	}
}
func TestUSProbeFallbackAndNotFound(t *testing.T) {
	for _, found := range []bool{true, false} {
		t.Run(fmt.Sprint(found), func(t *testing.T) {
			var prefixes []string
			sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/codes" {
					fmt.Fprint(w, `{"success":true,"list":["105.OTHER"]}`)
					return
				}
				secid := r.URL.Query().Get("secid")
				prefixes = append(prefixes, secid)
				if found && secid == "107.NEW" {
					fmt.Fprint(w, `{"data":{"klines":["2024-01-02,1,2,3,1,100"]}}`)
				} else {
					fmt.Fprint(w, `{"data":null}`)
				}
			}, true, 0)
			q, err := sdk.Kline.US(context.Background(), "NEW")
			if found {
				if err != nil || len(q) != 1 || len(prefixes) != 4 {
					t.Fatalf("%v %v %v", q, err, prefixes)
				}
			} else if sdkerrors.CodeOf(err) != sdkerrors.NotFound || len(prefixes) != 3 {
				t.Fatalf("%v %v", err, prefixes)
			}
		})
	}
}
func TestMinuteMarketConversionAndWindow(t *testing.T) {
	for _, period := range []string{"1", "5"} {
		t.Run(period, func(t *testing.T) {
			sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("secid") != "105.AAPL" {
					t.Error(q)
				}
				field := "klines"
				if period == "1" {
					field = "trends"
					if q.Get("ndays") != "5" || q.Get("iscr") != "0" {
						t.Error(q)
					}
				} else if q.Get("beg") != "20240709" || q.Get("end") != "20240710" || q.Get("klt") != "5" || q.Get("fqt") != "0" {
					t.Error(q)
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{field: []string{"2024-07-09 20:00,10,11,12,9,100,200,11", "2024-07-09 21:30,10,11,12,9,100,200,11", "2024-07-10 04:00,10,11,12,9,100,200,11", "2024-07-10 04:05,10,11,12,9,100,200,11"}}})
			}, true, 0)
			rows, err := sdk.Kline.USMinute(context.Background(), "105.AAPL", MinuteOptions{Period: period, Adjust: AdjustNone, StartDate: "20240709 09:30", EndDate: "2024-07-09 16:00", NDays: 5})
			if err != nil || len(rows) != 2 {
				t.Fatalf("%v %v", rows, err)
			}
			if rows[0].Time != "2024-07-09 09:30" || rows[1].Time != "2024-07-09 16:00" || rows[0].Timestamp == nil || *rows[0].Timestamp != 1720531800000 || rows[0].TZ != "America/New_York" || rows[0].Currency != "USD" {
				t.Fatalf("%+v", rows)
			}
			if period == "1" {
				assertNum(t, rows[0].AvgPrice, 11)
				if rows[0].Amplitude != nil {
					t.Fatal("trend fields confused with kline fields")
				}
			}
		})
	}
}
func TestCNMinuteDateOnlyIncludesLastMinute(t *testing.T) {
	sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ndays") != "5" {
			t.Error(r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"data":{"trends":["2024-01-02 09:30,1,2,3,0,10,20,2","2024-01-02 15:00,1,2,3,0,10,20,2","2024-01-03 09:30,1,2,3,0,10,20,2"]}}`)
	}, true, 0)
	q, err := sdk.Kline.CNMinute(context.Background(), "600519", MinuteOptions{StartDate: "20240102", EndDate: "20240102"})
	if err != nil || len(q) != 2 {
		t.Fatalf("%v %v", q, err)
	}
}
func TestTencentQFQFallback(t *testing.T) {
	var emCalls, txCalls atomic.Int32
	sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/em":
			emCalls.Add(1)
			w.WriteHeader(503)
		case "/tx":
			txCalls.Add(1)
			param := strings.Split(r.URL.Query().Get("param"), ",")
			if param[0] != "sh600519" || param[1] != "day" {
				t.Error(param)
			}
			if param[5] == "hfq" {
				fmt.Fprint(w, `{"code":0,"data":{"sh600519":{"hfqday":[["2024-01-01","100","100","110","90","10"],["2024-01-02","110","120","130","100","20"],["2024-01-03","130","200","210","120","30"]]}}}`)
			} else {
				fmt.Fprint(w, `{"data":{"sh600519":{"day":[["2024-01-03","13","20","21","12","30"]]}}}`)
			}
		default:
			t.Errorf("unexpected source: %s", r.URL.Path)
		}
	}, false, 2)
	q, err := sdk.Kline.CN(context.Background(), "600519", KlineOptions{StartDate: "20240102", EndDate: "20240103"})
	if err != nil || len(q) != 2 {
		t.Fatalf("%v %v", q, err)
	}
	if emCalls.Load() != 1 || txCalls.Load() != 2 {
		t.Fatal(emCalls.Load(), txCalls.Load())
	}
	assertNum(t, q[0].Close, 12)
	assertNum(t, q[0].Change, 2)
	assertNum(t, q[0].ChangePercent, 20)
	if q[0].Amount != nil || q[0].TurnoverRate != nil || q[0].Adjust != AdjustQFQ || q[0].Source != "tencent" {
		t.Fatalf("%+v", q[0])
	}
}
func TestSinaFallbackAggregation(t *testing.T) {
	sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/em":
			fmt.Fprint(w, `{"data":null}`)
		case "/tx":
			fmt.Fprint(w, `{"code":1,"msg":"not available"}`)
		case "/sina":
			if r.URL.Query().Get("scale") != "240" {
				t.Error(r.URL.RawQuery)
			}
			fmt.Fprint(w, `[{"day":"2023-12-29","open":"8","close":"10","high":"11","low":"7","volume":"1000"},{"day":"2024-01-02","open":"10","close":"12","high":"13","low":"9","volume":"2000"},{"day":"2024-01-03","open":"12","close":"14","high":"15","low":"11","volume":"3000"}]`)
		}
	}, false, 0)
	q, err := sdk.Kline.CN(context.Background(), "600519", KlineOptions{Period: "weekly", StartDate: "20240101", EndDate: "20240131"})
	if err != nil || len(q) != 1 {
		t.Fatalf("%v %v", q, err)
	}
	assertNum(t, q[0].Open, 10)
	assertNum(t, q[0].Close, 14)
	assertNum(t, q[0].Volume, 50)
	assertNum(t, q[0].Change, 4)
	if q[0].Source != "sina" || q[0].Adjust != AdjustNone || q[0].Date != "2024-01-03" {
		t.Fatalf("%+v", q[0])
	}
}
func TestMinuteFallbacks(t *testing.T) {
	for _, source := range []string{"tencent", "sina"} {
		t.Run(source, func(t *testing.T) {
			sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/em":
					fmt.Fprint(w, `{"data":null}`)
				case "/txm":
					if !strings.HasPrefix(r.URL.Query().Get("param"), "sh600519,m5,") {
						t.Error(r.URL.RawQuery)
					}
					if source == "sina" {
						fmt.Fprint(w, `{"data":{}}`)
					} else {
						fmt.Fprint(w, `{"data":{"sh600519":{"m5":[["202401020930","10","11","12","9","20"],["202401020935","11","12","13","10","30"]]}}}`)
					}
				case "/sina":
					if r.URL.Query().Get("scale") != "5" {
						t.Error(r.URL.RawQuery)
					}
					fmt.Fprint(w, `[{"day":"2024-01-02 09:30:00","open":"10","close":"11","high":"12","low":"9","volume":"2000","amount":"22000"},{"day":"2024-01-02 09:35:00","open":"11","close":"12","high":"13","low":"10","volume":"3000","amount":"36000"}]`)
				}
			}, false, 0)
			q, err := sdk.Kline.CNMinute(context.Background(), "600519", MinuteOptions{Period: "5", StartDate: "20240102", EndDate: "20240102"})
			if err != nil || len(q) != 2 {
				t.Fatalf("%v %v", q, err)
			}
			if q[0].Source != source || q[0].Adjust != AdjustNone || q[0].Time != "2024-01-02 09:30" || q[0].Date != "" {
				t.Fatalf("%+v", q)
			}
			assertNum(t, q[0].Volume, 20)
			assertNum(t, q[1].Change, 1)
		})
	}
}
func TestKlineNoFallbackOnEmptyValidRowsOrDisabled(t *testing.T) {
	for _, tt := range []struct {
		payload string
		disable bool
		want    sdkerrors.Code
		calls   int
	}{{`{"data":{"klines":[]}}`, false, "", 1}, {`{"data":null}`, true, sdkerrors.UpstreamEmpty, 1}, {`bad`, true, sdkerrors.Parse, 1}, {`{"rc":5}`, true, sdkerrors.UpstreamError, 1}} {
		t.Run(tt.payload, func(t *testing.T) {
			calls := 0
			sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/em" {
					t.Error("unexpected fallback")
				}
				fmt.Fprint(w, tt.payload)
			}, tt.disable, 0)
			q, err := sdk.Kline.CN(context.Background(), "600519")
			if sdkerrors.CodeOf(err) != tt.want || calls != tt.calls || (err == nil && q == nil) {
				t.Fatalf("%v %v calls=%d", q, err, calls)
			}
		})
	}
}
func TestFallbackFailurePreservesPrimaryAndCancel(t *testing.T) {
	sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }, false, 0)
	_, err := sdk.Kline.CN(context.Background(), "600519")
	var se *sdkerrors.Error
	if !errors.As(err, &se) || se.Provider != "eastmoney" || se.Status != 503 {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = sdk.Kline.CN(ctx, "600519")
	if sdkerrors.CodeOf(err) != sdkerrors.Aborted {
		t.Fatal(err)
	}
}

func TestTencentHistoryPagination(t *testing.T) {
	var ends []string
	sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tx" {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		param := strings.Split(r.URL.Query().Get("param"), ",")
		ends = append(ends, param[3])
		var rows [][]string
		if len(ends) == 1 {
			start, _ := time.Parse("2006-01-02", "2024-01-03")
			for i := 0; i < 640; i++ {
				rows = append(rows, []string{start.AddDate(0, 0, i).Format("2006-01-02"), "10", "11", "12", "9", "100"})
			}
		} else {
			rows = [][]string{{"2023-12-29", "9", "10", "11", "8", "50"}, {"2024-01-02", "10", "11", "12", "9", "100"}}
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"sh600519": map[string]any{"day": rows}}})
	}, false, 0)
	o, _ := historyOptions([]KlineOptions{{StartDate: "20240102", EndDate: "20260101", Adjust: AdjustNone}})
	q, err := sdk.Kline.tencentHistory(context.Background(), "600519", o)
	if err != nil || len(q) != 641 || len(ends) != 2 || ends[1] != "2024-01-02" {
		t.Fatalf("len=%d ends=%v err=%v", len(q), ends, err)
	}
	assertNum(t, q[0].Change, 1)
}
func TestKlineOptionsRejectedBeforeRequest(t *testing.T) {
	sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid options caused request") }, false, 0)
	ctx := context.Background()
	for _, o := range []KlineOptions{{Period: "hourly"}, {Adjust: "bad"}, {StartDate: "20240230"}, {StartDate: "20240103", EndDate: "20240102"}} {
		if _, err := sdk.Kline.CN(ctx, "600519", o); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatalf("%+v %v", o, err)
		}
	}
	for _, o := range []MinuteOptions{{Period: "2"}, {Adjust: "bad"}, {NDays: 6}, {NDays: -1}, {StartDate: "invalid"}, {EndDate: "20240132"}, {StartDate: "20240103", EndDate: "20240102"}} {
		if _, err := sdk.Kline.CNMinute(ctx, "600519", o); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatalf("%+v %v", o, err)
		}
	}
	if _, err := sdk.Kline.CN(ctx, "600519", KlineOptions{}, KlineOptions{}); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := sdk.Kline.HKMinute(ctx, "700", MinuteOptions{}, MinuteOptions{}); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
	if _, err := sdk.Kline.HK(ctx, "AAPL"); sdkerrors.CodeOf(err) != sdkerrors.InvalidSymbol {
		t.Fatal(err)
	}
}
func TestOtherKlineBranches(t *testing.T) {
	t.Run("empty foreign and trends", func(t *testing.T) {
		sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":null}`) }, true, 0)
		if q, err := sdk.Kline.HK(context.Background(), "700"); err != nil || q == nil || len(q) != 0 {
			t.Fatalf("%v %v", q, err)
		}
		if q, err := sdk.Kline.HKMinute(context.Background(), "700"); err != nil || q == nil || len(q) != 0 {
			t.Fatalf("%v %v", q, err)
		}
	})
	t.Run("hk minute", func(t *testing.T) {
		sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/hk" || r.URL.Query().Get("klt") != "30" {
				t.Error(r.URL)
			}
			fmt.Fprint(w, `{"data":{"klines":["2024-01-02 10:00,10,11,12,9,10,--,--,--,--,--",null]}}`)
		}, true, 0)
		q, err := sdk.Kline.HKMinute(context.Background(), "700", MinuteOptions{Period: "30"})
		if err != nil || len(q) != 1 || q[0].Currency != "HKD" || q[0].Time != "2024-01-02 10:00" {
			t.Fatalf("%v %v", q, err)
		}
	})
	t.Run("primary retry without fallback", func(t *testing.T) {
		calls := 0
		sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.WriteHeader(503)
				return
			}
			fmt.Fprint(w, `{"data":{"klines":[]}}`)
		}, true, 1)
		if _, err := sdk.Kline.CN(context.Background(), "600519"); err != nil || calls != 2 {
			t.Fatal(err, calls)
		}
	})
	t.Run("unadjusted tencent history", func(t *testing.T) {
		sdk, _ := klineTestSDK(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/em" {
				w.WriteHeader(503)
				return
			}
			if r.URL.Path != "/tx" {
				t.Error(r.URL)
			}
			fmt.Fprint(w, `{"data":{"sh600519":{"day":[["2024-01-02",null,"11","12","9","20"]]}}}`)
		}, false, 0)
		q, err := sdk.Kline.CN(context.Background(), "600519", KlineOptions{StartDate: "20240102", EndDate: "20240103", Adjust: AdjustNone})
		if err != nil || len(q) != 1 || q[0].Open != nil || q[0].Change != nil {
			t.Fatalf("%v %v", q, err)
		}
	})
	t.Run("sina monthly", func(t *testing.T) {
		rows := []KlineBar{csvBar("2024-01-02,1,2,3,0,10"), csvBar("2024-01-03,2,3,4,1,20"), csvBar("2024-02-01,4,5,6,3,30")}
		q := aggregateSina(rows, "monthly")
		if len(q) != 2 || q[0].Date != "2024-01-03" {
			t.Fatal(q)
		}
		assertNum(t, q[0].Volume, 30)
	})
}

package stockgo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestSearch(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("q") != "茅台 & 腾讯" || q.Get("v") != "2" || q.Get("t") != "all" {
			t.Errorf("query=%v", q)
		}
		w.Write([]byte(`v_hint="sh~600519~\u8d35\u5dde\u8305\u53f0~gzmt~GP-A^hk~00700~腾讯~tx~GP^sh~510300~沪深300~hs~ETF";`))
	}))
	defer srv.Close()
	sdk, _ := New(Options{SearchURL: srv.URL})
	result, err := sdk.Search(context.Background(), "茅台 & 腾讯")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 || result[0].Code != "sh600519" || result[0].Name != "贵州茅台" || result[0].Type != "GP-A" || result[2].Category != "fund" {
		t.Fatalf("%+v", result)
	}
	empty, err := sdk.Search(context.Background(), "  ")
	if err != nil || len(empty) != 0 || calls != 1 {
		t.Fatalf("%v %v calls=%d", empty, err, calls)
	}
}
func TestSearchType(t *testing.T) {
	for raw, want := range map[string]string{"GP-B": "stock", "ZS": "index", "QDII-ETF": "fund", "KJ-HB": "fund", "LOF": "fund", "JJ": "fund", "FUND": "fund", "ZQ": "bond", "QH": "futures", "OPTION": "option", "QZ": "option", "GP-A": "stock", "unknown": "other"} {
		if got := NormalizeSearchType(raw); got != want {
			t.Errorf("%s: %s", raw, got)
		}
	}
}
func TestSearchEmptyAndInvalidResponses(t *testing.T) {
	for _, body := range []string{`v_hint="N";`, `v_hint="";`, `other="";`, `v_hint="sh~1~\q~a~GP";`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer srv.Close()
			sdk, _ := New(Options{SearchURL: srv.URL})
			result, err := sdk.Search(context.Background(), "x")
			if body == `v_hint="sh~1~\q~a~GP";` {
				if sdkerrors.CodeOf(err) != sdkerrors.Parse {
					t.Fatal(err)
				}
			} else if err != nil || len(result) != 0 {
				t.Fatalf("%v %v", result, err)
			}
		})
	}
}
func TestSearchGBK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gbkWrite(t, w, `v_hint="sh~600519~贵州茅台~gzmt~GP-A";`)
	}))
	defer srv.Close()
	sdk, _ := New(Options{SearchURL: srv.URL})
	got, err := sdk.Search(context.Background(), "茅台")
	if err != nil || len(got) != 1 || got[0].Name != "贵州茅台" {
		t.Fatalf("%v %v", got, err)
	}
}
func TestTimeline(t *testing.T) {
	for _, tt := range []struct {
		name    string
		lines   []string
		volumes []float64
		avg     []float64
	}{
		{"lots after empty tick", []string{"0930 10 0 0", "0931 10 2 2000", "0932 11 3 3100"}, []float64{0, 200, 300}, []float64{0, 10, 10.33}},
		{"shares", []string{"0930 10 200 2000", "0931 11 300 3100"}, []float64{200, 300}, []float64{10, 10.33}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("code") != "sh600519" {
					t.Errorf("query=%s", r.URL.RawQuery)
				}
				json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"sh600519": map[string]any{"data": map[string]any{"date": "20240102", "data": tt.lines}, "qt": map[string]any{"sh600519": []string{"1", "name", "600519", "10", "9.5"}}}}})
			}))
			defer srv.Close()
			sdk, _ := New(Options{TimelineURL: srv.URL})
			got, err := sdk.Quotes.Timeline(context.Background(), "600519.SH")
			if err != nil {
				t.Fatal(err)
			}
			if got.Code != "sh600519" || got.Date != "20240102" || got.PreClose != 9.5 || got.Timestamp == nil || *got.Timestamp != 1704124800000 || len(got.Data) != len(tt.lines) {
				t.Fatalf("%+v", got)
			}
			volumes, averages := []float64{}, []float64{}
			for i, point := range got.Data {
				volumes = append(volumes, point.Volume)
				averages = append(averages, point.AvgPrice)
				if point.Timestamp == nil || *point.Timestamp != 1704159000000+int64(i)*60000 {
					t.Errorf("invalid point: %+v", point)
				}
			}
			if !reflect.DeepEqual(volumes, tt.volumes) || !reflect.DeepEqual(averages, tt.avg) {
				t.Fatalf("volume=%v avg=%v", volumes, averages)
			}
		})
	}
}
func TestTimelineErrorsAndEmpty(t *testing.T) {
	for _, tt := range []struct {
		body string
		code sdkerrors.Code
	}{
		{`{`, sdkerrors.Parse}, {`{"code":1,"msg":"limited"}`, sdkerrors.UpstreamError}, {`{"data":{}}`, sdkerrors.UpstreamError},
		{`{"code":0,"data":{}}`, ""},
		{`{"code":0,"data":{"sh600519":{"data":{"data":["bad 1 2 3"]}}}}`, sdkerrors.Parse},
	} {
		t.Run(tt.body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(tt.body)) }))
			defer srv.Close()
			sdk, _ := New(Options{TimelineURL: srv.URL})
			got, err := sdk.Quotes.Timeline(context.Background(), "600519")
			if sdkerrors.CodeOf(err) != tt.code {
				t.Fatalf("%v", err)
			}
			if tt.code == "" && (got.Data == nil || len(got.Data) != 0 || got.Timestamp != nil) {
				t.Fatalf("%+v", got)
			}
		})
	}
	sdk, _ := New()
	if _, err := sdk.Quotes.Timeline(context.Background(), "AAPL"); sdkerrors.CodeOf(err) != sdkerrors.InvalidSymbol {
		t.Fatal(err)
	}
	if _, err := sdk.Quotes.Timeline(context.Background(), "930955"); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
}

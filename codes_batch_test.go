package stockgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestCodeListsCacheAndFilters(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/cn":
			fmt.Fprint(w, `{"success":true,"list":["sh600519","sh688001","sz000001","sz300001","bj430047","bj830001","bj920001"]}`)
		case "/us":
			fmt.Fprint(w, `{"success":true,"list":["105.AAPL","106.BABA","107.SPY"]}`)
		case "/hk":
			fmt.Fprint(w, `{"success":true,"list":["00700"]}`)
		case "/fund":
			fmt.Fprint(w, `3,000001,110011, `)
		}
	}))
	defer srv.Close()
	sdk, _ := New(Options{CodeURLs: CodeEndpoints{CN: srv.URL + "/cn", US: srv.URL + "/us", HK: srv.URL + "/hk", Fund: srv.URL + "/fund"}})
	ctx := context.Background()
	for _, tt := range []struct {
		market string
		want   []string
	}{{"sh", []string{"600519", "688001"}}, {"sz", []string{"000001", "300001"}}, {"kc", []string{"688001"}}, {"cy", []string{"300001"}}, {"bj", []string{"430047", "830001", "920001"}}} {
		list, err := sdk.Codes.CN(ctx, CodeOptions{Simple: true, Market: tt.market})
		if err != nil || !reflect.DeepEqual(list, tt.want) {
			t.Fatalf("%s: %v %v", tt.market, list, err)
		}
	}
	list, _ := sdk.Codes.CN(ctx)
	list[0] = "tampered"
	again, _ := sdk.Codes.CN(ctx)
	if again[0] != "sh600519" || calls.Load() != 1 {
		t.Fatal("cache mutation or repeated fetch")
	}
	for market, want := range map[string]string{"NASDAQ": "AAPL", "NYSE": "BABA", "AMEX": "SPY"} {
		list, err := sdk.Codes.US(ctx, CodeOptions{Simple: true, Market: market})
		if err != nil || !reflect.DeepEqual(list, []string{want}) {
			t.Fatalf("%v %v", list, err)
		}
	}
	if list, err := sdk.Codes.HK(ctx); err != nil || !reflect.DeepEqual(list, []string{"00700"}) {
		t.Fatalf("%v %v", list, err)
	}
	if list, err := sdk.Codes.Fund(ctx); err != nil || !reflect.DeepEqual(list, []string{"000001", "110011"}) {
		t.Fatalf("%v %v", list, err)
	}
	sdk.ClearCaches()
	sdk.Codes.CN(ctx)
	if calls.Load() != 5 {
		t.Fatalf("calls=%d", calls.Load())
	}
}
func TestCodeFailureIsNotCached(t *testing.T) {
	for _, bad := range []string{`{"success":false,"list":["bad"]}`, `{"success":true,"list":[]}`, `{"success":true}`, `{`} {
		t.Run(bad, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					fmt.Fprint(w, bad)
				} else {
					fmt.Fprint(w, `{"success":true,"list":["sh600519"]}`)
				}
			}))
			defer srv.Close()
			sdk, _ := New(Options{CodeURLs: CodeEndpoints{CN: srv.URL}})
			if _, err := sdk.Codes.CN(context.Background()); err == nil {
				t.Fatal("expected error")
			}
			if list, err := sdk.Codes.CN(context.Background()); err != nil || len(list) != 1 || calls != 2 {
				t.Fatalf("%v %v %d", list, err, calls)
			}
		})
	}
}
func TestCodeValidation(t *testing.T) {
	sdk, _ := New()
	for _, run := range []func() error{
		func() error { _, err := sdk.Codes.CN(context.Background(), CodeOptions{Market: "bad"}); return err }, func() error { _, err := sdk.Codes.US(context.Background(), CodeOptions{Market: "bad"}); return err }, func() error { _, err := sdk.Codes.CN(context.Background(), CodeOptions{}, CodeOptions{}); return err },
	} {
		if err := run(); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatal(err)
		}
	}
}
func TestBatchConcurrencyOrderAndProgress(t *testing.T) {
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var active, maxActive atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := maxActive.Load()
			if old >= n || maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		<-release
		keys := strings.Split(r.URL.Query().Get("q"), ",")
		for _, key := range keys {
			gbkWrite(t, w, line(key, fields(key[2:], 38)))
		}
	}))
	defer srv.Close()
	sdk, _ := New(Options{TencentURL: srv.URL})
	codes := []string{"600001", "600002", "600003", "600004", "600005", "600006"}
	var progress []int
	done := make(chan []FullQuote, 1)
	errs := make(chan error, 1)
	go func() {
		q, err := sdk.Batch.ByCodes(context.Background(), codes, BatchOptions{BatchSize: 2, Concurrency: 3, Progress: func(n, total int) {
			if total != 3 {
				t.Error(total)
			}
			progress = append(progress, n)
		}})
		done <- q
		errs <- err
	}()
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("batch did not start expected workers")
		}
	}
	close(release)
	q := <-done
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range q {
		got = append(got, row.Code)
	}
	if !reflect.DeepEqual(got, codes) || !reflect.DeepEqual(progress, []int{1, 2, 3}) || maxActive.Load() != 3 {
		t.Fatalf("codes=%v progress=%v concurrency=%d", got, progress, maxActive.Load())
	}
}
func TestBatchFailureCancelsOutstanding(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("q"), "600001") {
			<-started
			w.WriteHeader(503)
			return
		}
		once.Do(func() { close(started) })
		<-r.Context().Done()
		close(canceled)
	}))
	defer srv.Close()
	sdk, _ := New(Options{TencentURL: srv.URL})
	_, err := sdk.Batch.ByCodes(context.Background(), []string{"600001", "600002"}, BatchOptions{BatchSize: 1, Concurrency: 2})
	if sdkerrors.CodeOf(err) != sdkerrors.HTTP {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("outstanding request not canceled")
	}
}
func TestBatchMarketsAndRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/codes" {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "list": []string{"sh600519"}})
			return
		}
		q := r.URL.Query().Get("q")
		for _, key := range strings.Split(q, ",") {
			code := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(key, "sh"), "us"), "r_hk")
			gbkWrite(t, w, line(key, fields(code, 38)))
		}
	}))
	defer srv.Close()
	sdk, _ := New(Options{TencentURL: srv.URL, CodeURLs: CodeEndpoints{CN: srv.URL + "/codes", HK: srv.URL + "/codes", US: srv.URL + "/codes"}})
	ctx := context.Background()
	if q, err := sdk.Batch.CN(ctx); err != nil || len(q) != 1 {
		t.Fatalf("%v %v", q, err)
	}
	if q, err := sdk.Batch.USByCodes(ctx, []string{"AAPL"}); err != nil || len(q) != 1 {
		t.Fatalf("%v %v", q, err)
	}
	if q, err := sdk.Batch.HKByCodes(ctx, []string{"700"}); err != nil || len(q) != 1 {
		t.Fatalf("%v %v", q, err)
	}
	if q, err := sdk.Batch.Raw(ctx, "sh600519"); err != nil || len(q) != 1 {
		t.Fatalf("%v %v", q, err)
	}
	if q, err := sdk.Batch.Raw(ctx, ""); err != nil || q == nil || len(q) != 0 {
		t.Fatalf("%v %v", q, err)
	}
	if q, err := sdk.Batch.ByCodes(ctx, nil); err != nil || q == nil || len(q) != 0 {
		t.Fatalf("%v %v", q, err)
	}
	for _, o := range []BatchOptions{{BatchSize: -1}, {Concurrency: -1}} {
		if _, err := sdk.Batch.ByCodes(ctx, nil, o); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatal(err)
		}
	}
	if _, err := sdk.Batch.HK(ctx, BatchOptions{Market: "bad"}); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := sdk.Batch.ByCodes(ctx, []string{"600519"}); sdkerrors.CodeOf(err) != sdkerrors.Aborted {
		t.Fatal(err)
	}
	if o, err := batchOption([]BatchOptions{{BatchSize: 999}}); err != nil || o.BatchSize != 500 {
		t.Fatalf("%+v %v", o, err)
	}
}

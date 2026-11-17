package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

func TestRetries(t *testing.T) {
	for _, status := range []int{400, 404, 408, 429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Test") != "yes" {
					t.Error("missing injected header")
				}
				if attempts.Add(1) < 3 {
					w.WriteHeader(status)
					return
				}
				w.Write([]byte("ok"))
			}))
			defer srv.Close()
			c, err := New(Config{MaxRetries: 2, BaseDelay: time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			body, err := c.Get(context.Background(), "test", srv.URL, http.Header{"X-Test": []string{"yes"}})
			if status == 400 || status == 404 {
				if attempts.Load() != 1 || sdkerrors.CodeOf(err) != sdkerrors.HTTP {
					t.Fatalf("attempts=%d err=%v", attempts.Load(), err)
				}
			} else if err != nil || string(body) != "ok" || attempts.Load() != 3 {
				t.Fatalf("attempts=%d body=%q err=%v", attempts.Load(), body, err)
			}
		})
	}
}
func TestCancellationDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503); cancel() }))
	defer srv.Close()
	c, _ := New(Config{MaxRetries: 2, BaseDelay: time.Hour})
	_, err := c.Get(ctx, "test", srv.URL, nil)
	if sdkerrors.CodeOf(err) != sdkerrors.Aborted || !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}
func TestTimeoutAndBodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		w.Write([]byte("12345"))
	}))
	defer srv.Close()
	c, _ := New(Config{Timeout: 20 * time.Millisecond, MaxBodyBytes: 4})
	_, err := c.Get(context.Background(), "test", srv.URL+"/slow", nil)
	if sdkerrors.CodeOf(err) != sdkerrors.Timeout || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%v", err)
	}
	_, err = c.Get(context.Background(), "test", srv.URL, nil)
	if sdkerrors.CodeOf(err) != sdkerrors.Parse {
		t.Fatalf("%v", err)
	}
}
func TestInvalidConfig(t *testing.T) {
	for _, cfg := range []Config{{MaxRetries: -1}, {MaxRetries: 11}, {Timeout: -1}, {BaseDelay: -1}, {MaxBodyBytes: -1}} {
		if _, err := New(cfg); sdkerrors.CodeOf(err) != sdkerrors.InvalidArgument {
			t.Fatalf("%v", err)
		}
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildCLI(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stock-go")
	cmd := exec.Command("go", "build", "-ldflags", "-X main.version=test-version", "-o", path, ".")
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(output))
	}
	return path
}
func TestCLIProcess(t *testing.T) {
	bin := buildCLI(t)
	for _, args := range [][]string{{"--help"}, {"--version"}, {"search", "--help"}, {"quote", "600519", "--unknown"}, {"call", "client.get"}} {
		cmd := exec.Command(bin, args...)
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		e := cmd.Run()
		want := 0
		if args[0] == "call" || args[0] == "quote" {
			want = 2
		}
		got := 0
		if e != nil {
			var exit *exec.ExitError
			if !errors.As(e, &exit) {
				t.Fatal(e)
			}
			got = exit.ExitCode()
		}
		if got != want {
			t.Fatal(args, got, out.String(), stderr.String())
		}
		if want == 2 {
			if out.Len() != 0 || !json.Valid(stderr.Bytes()) {
				t.Fatal(out.String(), stderr.String())
			}
		} else if out.Len() == 0 || stderr.Len() != 0 {
			t.Fatal(out.String(), stderr.String())
		}
	}
	// Actual fd 1 EPIPE must exit cleanly, including the platform's SIGPIPE behavior.
	reader, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	reader.Close()
	defer writer.Close()
	cmd := exec.Command(bin, "--help")
	cmd.Stdout = writer
	if e = cmd.Run(); e != nil {
		t.Fatal(e)
	}
}
func TestLiveCLI(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers from the CLI")
	}
	bin := buildCLI(t)
	start := time.Now().AddDate(0, 0, -14).Format("20060102")
	cases := []struct {
		name  string
		args  []string
		count int
	}{
		{"quote", []string{"quote", "600519", "--limit", "1"}, 1},
		{"csv", []string{"kline", "600519", "--start", start, "--limit", "2", "-f", "csv"}, 2},
		{"indicators", []string{"indicators", "600519", "--start", start, "--ma", "5,10", "--macd", "--limit", "2"}, 2},
		{"futures", []string{"futures", "kline", "RBM", "--start", start, "--limit", "1"}, 1},
		{"changes", []string{"marketEvent", "stockChanges", "all", "--page", "1", "--pageSize", "3", "--limit", "2"}, 2},
		{"rawcall", []string{"call", "search", "--args", `["茅台"]`, "--limit", "1"}, 1},
		{"table", []string{"board", "industry", "list", "--limit", "2", "-f", "table"}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, c.args...)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, e := cmd.Output()
			if e != nil {
				t.Fatal(e, stderr.String())
			}
			switch c.name {
			case "csv":
				rows, e := csv.NewReader(bytes.NewReader(out)).ReadAll()
				if e != nil || len(rows) != c.count+1 {
					t.Fatal(len(rows), e, string(out))
				}
			case "table":
				if len(strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")) != c.count+2 {
					t.Fatal(string(out))
				}
			default:
				var rows []map[string]any
				if e := json.Unmarshal(out, &rows); e != nil || len(rows) != c.count {
					t.Fatal(e, string(out))
				}
				if c.name == "indicators" {
					if rows[0]["ma"] == nil || rows[0]["macd"] == nil {
						t.Fatal(rows[0])
					}
				}
			}
			t.Log("PASS", c.count, "rows")
		})
	}
	t.Run("expiry", func(t *testing.T) {
		out, e := exec.Command(bin, "options", "etf", "months", "50ETF").Output()
		if e != nil {
			t.Fatal(e)
		}
		var months struct{ Months []string }
		if e = json.Unmarshal(out, &months); e != nil || len(months.Months) == 0 {
			t.Fatal(e, string(out))
		}
		out, e = exec.Command(bin, "options", "etf", "expireDay", "50ETF", months.Months[0]).Output()
		if e != nil {
			t.Fatal(e)
		}
		var day struct{ ExpireDay string }
		if e = json.Unmarshal(out, &day); e != nil || day.ExpireDay == "" {
			t.Fatal(e, string(out))
		}
		t.Log(day.ExpireDay)
	})
}

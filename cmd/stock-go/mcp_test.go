package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestMCPProcess(t *testing.T) {
	bin := buildCLI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "mcp")
	cmd.Env = append(os.Environ(), "STOCK_GO_MCP_TOOLS=full", "STOCK_GO_MCP_PROMPTS=full")
	input, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	output, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	var log bytes.Buffer
	cmd.Stderr = &log
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	lines := []string{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, `{"jsonrpc":"2.0","id":3,"method":"prompts/list"}`, `{"jsonrpc":"2.0","id":4,"method":"prompts/get","params":{"name":"analyze_stock","arguments":{"symbol":"600519"}}}`, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_market_status","arguments":{"market":"A"}}}`, `invalid`, strings.Repeat("x", 1000001), `{"jsonrpc":"2.0","id":6,"method":"ping"}`}
	for _, line := range lines {
		if _, e = fmt.Fprintln(input, line); e != nil {
			t.Fatal(e)
		}
	}
	input.Close()
	body, e := io.ReadAll(output)
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Wait(); e != nil {
		t.Fatal(e, log.String())
	}
	responses := map[int]map[string]any{}
	nullErrors := map[int]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
		var r map[string]any
		if e = json.Unmarshal(line, &r); e != nil {
			t.Fatal("stdout protocol pollution", string(line), e)
		}
		if id, is := r["id"].(float64); is {
			responses[int(id)] = r
		} else if err, is := r["error"].(map[string]any); is {
			nullErrors[int(err["code"].(float64))] = true
		}
	}
	if len(responses) != 6 || !nullErrors[-32700] || !nullErrors[-32600] {
		t.Fatal(string(body))
	}
	init := responses[1]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" || init["serverInfo"].(map[string]any)["version"] != "test-version" {
		t.Fatal(init)
	}
	if len(responses[2]["result"].(map[string]any)["tools"].([]any)) != 93 || len(responses[3]["result"].(map[string]any)["prompts"].([]any)) != 7 {
		t.Fatal("catalog incomplete")
	}
	if responses[5]["result"].(map[string]any)["isError"] == true {
		t.Fatal(responses[5])
	}
	if !strings.Contains(log.String(), "ready") || bytes.Contains(body, []byte("[stock-go mcp]")) {
		t.Fatal(log.String(), string(body))
	}
}
func TestMCPProcessClosedPipe(t *testing.T) {
	bin := buildCLI(t)
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	r.Close()
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "mcp")
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	cmd.Stdout = w
	var log bytes.Buffer
	cmd.Stderr = &log
	if e = cmd.Run(); e != nil {
		t.Fatal(e, log.String())
	}
}
func TestLiveMCP(t *testing.T) {
	if os.Getenv("STOCK_GO_LIVE") != "1" {
		t.Skip("set STOCK_GO_LIVE=1 to query real providers through MCP")
	}
	bin := buildCLI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "mcp")
	cmd.Env = append(os.Environ(), "STOCK_GO_MCP_TOOLS=full", "STOCK_GO_MCP_PROMPTS=full")
	input, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	var log bytes.Buffer
	cmd.Stderr = &log
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	calls := []struct {
		name      string
		args      map[string]any
		oversized bool
	}{
		{"get_a_share_quotes", map[string]any{"codes": []string{"600519"}}, false},
		{"search", map[string]any{"keyword": "茅台"}, false},
		{"get_futures_kline", map[string]any{"symbol": "RBM", "startDate": time.Now().AddDate(0, 0, -7).Format("20060102")}, false},
		{"get_fund_nav_history", map[string]any{"code": "110011"}, true},
		{"get_stock_changes", map[string]any{"type": "all", "page": 1, "pageSize": 3}, false},
	}
	for i, c := range calls {
		msg := map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": map[string]any{"name": c.name, "arguments": c.args}}
		if e = json.NewEncoder(input).Encode(msg); e != nil {
			t.Fatal(e)
		}
	}
	input.Close()
	body, e := io.ReadAll(out)
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Wait(); e != nil {
		t.Fatal(e, log.String())
	}
	received := 0
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
		var response struct {
			ID     int
			Error  any
			Result struct {
				IsError bool
				Meta    map[string]any `json:"_meta"`
				Content []struct{ Text string }
			}
		}
		if e = json.Unmarshal(line, &response); e != nil || response.Error != nil || response.Result.IsError || len(response.Result.Content) != 1 {
			t.Fatal(e, string(line))
		}
		c := calls[response.ID-1]
		text := response.Result.Content[0].Text
		if c.oversized {
			if response.Result.Meta["truncated"] != true || !strings.Contains(text, "oversized") {
				t.Fatal("large result wasn't bounded", text)
			}
		} else if text == "[]" || text == "null" {
			t.Fatal("empty live result", c.name)
		}
		received++
		t.Log(c.name, "PASS", len(text), "text bytes")
	}
	if received != len(calls) {
		t.Fatal(received, len(calls), log.String())
	}
}

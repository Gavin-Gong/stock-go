package transport

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

var historyHosts = []string{"push2his.eastmoney.com", "7.push2his.eastmoney.com", "33.push2his.eastmoney.com", "63.push2his.eastmoney.com", "91.push2his.eastmoney.com"}
var pushHosts = []string{"17.push2.eastmoney.com", "29.push2.eastmoney.com", "79.push2.eastmoney.com", "91.push2.eastmoney.com", "push2delay.eastmoney.com"}
var emHostPattern = regexp.MustCompile(`^(?:[0-9]+\.)?push2(his)?\.eastmoney\.com$`)

type HostHealthStats struct {
	Host          string         `json:"host"`
	FailureCount  int            `json:"failureCount"`
	SuccessCount  int            `json:"successCount"`
	CooldownUntil int64          `json:"cooldownUntil"`
	LastFailureAt int64          `json:"lastFailureAt"`
	LastErrorCode sdkerrors.Code `json:"lastErrorCode,omitempty"`
}
type hostManager struct {
	mu     sync.Mutex
	states map[string]HostHealthStats
}

func (h *hostManager) candidates(endpoint, provider string, now time.Time) []string {
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.Port() != "" || provider != "eastmoney" || (u.Scheme != "http" && u.Scheme != "https") {
		return []string{endpoint}
	}
	hostname := strings.ToLower(u.Hostname())
	pool := []string{}
	if match := emHostPattern.FindStringSubmatch(hostname); match != nil {
		if match[1] == "his" {
			pool = historyHosts
		} else {
			pool = pushHosts
		}
	} else if hostname == "push2delay.eastmoney.com" {
		pool = pushHosts
	} else {
		return []string{endpoint}
	}
	// Keep the exact configured primary when healthy, even when it is the unnumbered push2 host.
	all := []string{hostname}
	for _, host := range pool {
		if host != hostname {
			all = append(all, host)
		}
	}
	h.mu.Lock()
	ordered := []string{}
	for _, cooling := range []bool{false, true} {
		for _, host := range all {
			if (h.states[host].CooldownUntil > now.UnixMilli()) == cooling {
				ordered = append(ordered, host)
			}
		}
	}
	h.mu.Unlock()
	out := []string{}
	for _, host := range ordered {
		candidate := *u
		candidate.Host = host
		out = append(out, candidate.String())
	}
	return out
}
func (h *hostManager) record(endpoint string, err error, now time.Time) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Hostname() == "" {
		return
	}
	host := strings.ToLower(u.Hostname())
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.states[host]
	state.Host = host
	if err == nil {
		state.FailureCount = 0
		state.SuccessCount++
		state.CooldownUntil = 0
		state.LastErrorCode = ""
	} else {
		state.FailureCount++
		state.LastFailureAt = now.UnixMilli()
		state.CooldownUntil = now.Add(30 * time.Second).UnixMilli()
		state.LastErrorCode = sdkerrors.CodeOf(err)
	}
	h.states[host] = state
}
func (h *hostManager) stats(provider string) []HostHealthStats {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []HostHealthStats{}
	for _, state := range h.states {
		if provider == "" || InferProvider("https://"+state.Host, "") == provider {
			out = append(out, state)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}
func shouldFallback(err error) bool {
	switch sdkerrors.CodeOf(err) {
	case sdkerrors.Network, sdkerrors.Timeout, sdkerrors.Parse:
		return true
	}
	var e *sdkerrors.Error
	if asError(err, &e) {
		return e.Status == 408 || e.Status == 429 || e.Status >= 500
	}
	return false
}

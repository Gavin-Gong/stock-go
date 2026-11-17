// Package mcp exposes stock-go through an MCP JSON-RPC stdio server.
package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	stockgo "github.com/Gavin-Gong/stock-go"
	"github.com/Gavin-Gong/stock-go/internal/operations"
)

const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)
const (
	LatestProtocolVersion = "2025-11-25"
	MaxArrayItems         = 200
	MaxTextChars          = 80000
	DefaultMaxLineBytes   = 1000000
)

var supportedVersions = []string{LatestProtocolVersion, "2025-06-18"}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Tier        string         `json:"-"`
	Path        string         `json:"-"`
}
type promptArg struct {
	Name, Description string
	Required          bool
	Default           *string
}
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}
type Prompt struct {
	Name                    string           `json:"name"`
	Title                   string           `json:"title"`
	Description             string           `json:"description"`
	Arguments               []PromptArgument `json:"arguments"`
	Tier                    string           `json:"-"`
	UsesTools               []string         `json:"-"`
	Args                    []promptArg      `json:"-"`
	Template, EmptyTemplate string           `json:"-"`
}

//go:embed tools.json
var toolManifest []byte

//go:embed prompts.json
var promptManifest []byte

// Options selects the exposed subset and an existing or newly configured SDK.
type Options struct {
	SDK                                     *stockgo.SDK
	SDKOptions                              *stockgo.Options
	ToolSet, PromptSet                      string
	Tools, Prompts                          []string
	Version                                 string
	LookupEnv                               func(string) string
	MaxLineBytes, MaxConcurrent, MaxPending int
	// Invoke is an optional query adapter; nil uses the typed Go SDK registry.
	Invoke func(context.Context, string, []any) (any, error)
}
type Server struct {
	sdk                       *stockgo.SDK
	tools                     []Tool
	toolMap                   map[string]Tool
	prompts                   []Prompt
	promptMap                 map[string]Prompt
	specs                     map[string]operations.Spec
	invoke                    func(context.Context, string, []any) (any, error)
	version                   string
	warnings                  []string
	maxLine, workers, pending int
}

func filter(explicit string, names []string, env string) (string, []string) {
	if names != nil {
		if len(names) == 0 {
			return "core", nil
		}
		return "", names
	}
	if explicit != "" {
		env = explicit
	}
	trimmed := strings.TrimSpace(env)
	if trimmed == "" {
		return "core", nil
	}
	lower := strings.ToLower(trimmed)
	if lower == "core" || lower == "full" {
		return lower, nil
	}
	out := []string{}
	for _, v := range strings.Split(trimmed, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return "core", nil
	}
	return "", out
}
func selected(name, tier, set string, names []string) bool {
	if set == "full" {
		return true
	}
	if set == "core" {
		return tier == "core"
	}
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
func envValue(lookup func(string) string, suffix string) string {
	if v := lookup("STOCK_GO_MCP_" + suffix); v != "" {
		return v
	}
	return lookup("STOCK_SDK_MCP_" + suffix)
}
func New(options ...Options) (*Server, error) {
	if len(options) > 1 {
		return nil, fmt.Errorf("at most one MCP Options is allowed")
	}
	var o Options
	if len(options) == 1 {
		o = options[0]
	}
	if o.LookupEnv == nil {
		o.LookupEnv = os.Getenv
	}
	if o.MaxLineBytes < 0 || o.MaxConcurrent < 0 || o.MaxPending < 0 {
		return nil, fmt.Errorf("MCP limits must be nonnegative")
	}
	if o.MaxLineBytes == 0 {
		o.MaxLineBytes = DefaultMaxLineBytes
	}
	if o.MaxConcurrent == 0 {
		o.MaxConcurrent = 16
	}
	if o.MaxPending == 0 {
		o.MaxPending = o.MaxConcurrent * 4
	}
	if o.MaxLineBytes > 64<<20 || o.MaxConcurrent > 1024 || o.MaxPending > 65536 {
		return nil, fmt.Errorf("MCP limits exceed supported bounds")
	}
	sdk := o.SDK
	if sdk == nil {
		cfg := stockgo.Options{}
		if o.SDKOptions != nil {
			cfg = *o.SDKOptions
		} else if raw := envValue(o.LookupEnv, "TIMEOUT"); raw != "" {
			var milliseconds float64
			if parsed, e := strconv.ParseFloat(strings.TrimSpace(raw), 64); e == nil && parsed > 0 && parsed < float64((24*time.Hour)/time.Millisecond) {
				milliseconds = parsed
				cfg.Timeout = time.Duration(milliseconds * float64(time.Millisecond))
			}
		}
		var e error
		sdk, e = stockgo.New(cfg)
		if e != nil {
			return nil, e
		}
	}
	registry, e := operations.New(sdk)
	if e != nil {
		return nil, e
	}
	invoke := o.Invoke
	if invoke == nil {
		invoke = registry.Invoke
	}
	version := o.Version
	if version == "" {
		version = "dev"
	}
	s := &Server{sdk: sdk, toolMap: map[string]Tool{}, promptMap: map[string]Prompt{}, specs: map[string]operations.Spec{}, invoke: invoke, version: version, maxLine: o.MaxLineBytes, workers: o.MaxConcurrent, pending: o.MaxPending, tools: []Tool{}, prompts: []Prompt{}}
	for _, spec := range operations.Specs() {
		s.specs[strings.Join(spec.Path, ".")] = spec
	}
	var tools []struct {
		Name, Description, Tier, Path string
		InputSchema                   map[string]any
	}
	if e = json.Unmarshal(toolManifest, &tools); e != nil {
		return nil, e
	}
	set, names := filter(o.ToolSet, o.Tools, envValue(o.LookupEnv, "TOOLS"))
	known := map[string]bool{}
	for _, raw := range tools {
		if !registry.Has(raw.Path) {
			return nil, fmt.Errorf("unknown MCP binding: %s", raw.Path)
		}
		if known[raw.Name] {
			return nil, fmt.Errorf("duplicate tool: %s", raw.Name)
		}
		spec := s.specs[raw.Path]
		if spec.ToolName != raw.Name {
			return nil, fmt.Errorf("tool/spec mismatch: %s", raw.Name)
		}
		known[raw.Name] = true
		if selected(raw.Name, raw.Tier, set, names) {
			t := Tool{Name: raw.Name, Description: raw.Description, Tier: raw.Tier, Path: raw.Path, InputSchema: raw.InputSchema}
			s.tools = append(s.tools, t)
			s.toolMap[t.Name] = t
		}
	}
	for _, n := range names {
		if !known[n] {
			s.warnings = append(s.warnings, "unknown tool: "+n)
		}
	}
	var prompts []struct {
		Name, Title, Description, Tier, Template, EmptyTemplate string
		Arguments                                               []PromptArgument
		Args                                                    []promptArg
		UsesTools                                               []string
	}
	if e = json.Unmarshal(promptManifest, &prompts); e != nil {
		return nil, e
	}
	set, names = filter(o.PromptSet, o.Prompts, envValue(o.LookupEnv, "PROMPTS"))
	known = map[string]bool{}
	for _, raw := range prompts {
		known[raw.Name] = true
		if selected(raw.Name, raw.Tier, set, names) {
			p := Prompt{Name: raw.Name, Title: raw.Title, Description: raw.Description, Tier: raw.Tier, Arguments: raw.Arguments, Args: raw.Args, UsesTools: raw.UsesTools, Template: raw.Template, EmptyTemplate: raw.EmptyTemplate}
			s.prompts = append(s.prompts, p)
			s.promptMap[p.Name] = p
			for _, tool := range p.UsesTools {
				if _, ok := s.toolMap[tool]; !ok {
					s.warnings = append(s.warnings, "prompt "+p.Name+" requires disabled tool: "+tool)
				}
			}
		}
	}
	for _, n := range names {
		if !known[n] {
			s.warnings = append(s.warnings, "unknown prompt: "+n)
		}
	}
	return s, nil
}
func (s *Server) Warnings() []string { return append([]string{}, s.warnings...) }
func NegotiateProtocolVersion(requested any) string {
	for _, v := range supportedVersions {
		if requested == v {
			return v
		}
	}
	return LatestProtocolVersion
}

var placeholder = regexp.MustCompile(`__STOCK_GO_ARG_([A-Za-z0-9_]+)__`)

func (p Prompt) render(raw map[string]any) (string, error) {
	filled := map[string]string{}
	for _, a := range p.Args {
		v := raw[a.Name]
		if v == nil || v == "" {
			if a.Required {
				return "", fmt.Errorf("缺少必填参数 %q", a.Name)
			}
			if a.Default != nil {
				filled[a.Name] = *a.Default
			}
		} else {
			filled[a.Name] = jsString(v)
		}
	}
	template := p.Template
	if p.Name == "analyze_capital_flow" && filled["symbol"] == "" {
		template = p.EmptyTemplate
	}
	return placeholder.ReplaceAllStringFunc(template, func(marker string) string { key := placeholder.FindStringSubmatch(marker)[1]; return filled[key] }), nil
}
func jsString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "null"
	case []any:
		parts := []string{}
		for _, v := range x {
			if v == nil {
				parts = append(parts, "")
			} else {
				parts = append(parts, jsString(v))
			}
		}
		return strings.Join(parts, ",")
	case map[string]any:
		return "[object Object]"
	default:
		return fmt.Sprint(v)
	}
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case float64:
		return x != 0
	default:
		return true
	}
}

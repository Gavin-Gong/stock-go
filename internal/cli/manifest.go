package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Gavin-Gong/stock-go/internal/operations"
)

type Option struct {
	Flag, Field, Type, Desc string
	Enum                    []string
	Required, Upper         bool
	Map                     map[string]string
}
type Command struct {
	Path, Alias       []string
	Summary, ArgShape string
	Positional        []operations.Positional
	Options           []Option
}
type Manifest struct{ Namespaces, Aliases []Command }

//go:embed commands.json
var commandJSON []byte
var commands = func() Manifest {
	var m Manifest
	if e := json.Unmarshal(commandJSON, &m); e != nil {
		panic(e)
	}
	// Make the source's passthrough-only options visible and typed in Go.
	for i := range m.Namespaces {
		for _, s := range operations.Specs() {
			if strings.Join(s.Path, ".") != strings.Join(m.Namespaces[i].Path, ".") {
				continue
			}
			seen := map[string]bool{}
			for _, o := range m.Namespaces[i].Options {
				seen[o.Flag] = true
			}
			for _, p := range s.Params {
				if !seen[p.Flag] {
					m.Namespaces[i].Options = append(m.Namespaces[i].Options, Option{Flag: p.Flag, Field: p.Field, Type: p.Type, Desc: p.Desc, Enum: p.Enum, Required: p.Required, Upper: p.Upper, Map: p.Map})
				}
			}
		}
	}
	return m
}()

func booleanFlags() map[string]bool {
	out := map[string]bool{}
	for _, k := range []string{"help", "h", "version", "V", "quiet", "q", "pretty"} {
		out[k] = true
	}
	for _, c := range append(append([]Command{}, commands.Namespaces...), commands.Aliases...) {
		for _, o := range c.Options {
			if o.Type == "boolean" {
				out[o.Flag] = true
			}
		}
	}
	return out
}
func find(tokens []string) (Command, []string, bool) {
	for n := min(3, len(tokens)); n >= 2; n-- {
		prefix := strings.Join(tokens[:n], ".")
		for _, c := range commands.Namespaces {
			if strings.Join(c.Path, ".") == prefix {
				return c, tokens[n:], true
			}
		}
	}
	if len(tokens) == 0 {
		return Command{}, nil, false
	}
	for _, c := range commands.Aliases {
		for _, alias := range c.Alias {
			if alias == tokens[0] {
				return c, tokens[1:], true
			}
		}
	}
	for _, c := range commands.Namespaces {
		if len(c.Path) == 1 && c.Path[0] == tokens[0] {
			return c, tokens[1:], true
		}
	}
	return Command{}, nil, false
}
func help(command *Command) string {
	if command == nil {
		lines := []string{"stock-go — 股票数据命令行", "", "用法: stock-go <command> [args] [--flags]", "      stock-go <namespace> <method> [args] [--flags]", "", "常用命令:"}
		for _, c := range commands.Aliases {
			lines = append(lines, fmt.Sprintf("  %-12s %s", c.Alias[0], c.Summary))
		}
		lines = append(lines, "  mcp          MCP stdio server", "", "全局选项: --format/-f json|table|csv  --pretty  --quiet/-q", "          --timeout <ms|duration>  --limit <N>  --help/-h  --version/-V", "", "命名空间:")
		seen := map[string]bool{}
		for _, c := range commands.Namespaces {
			if !seen[c.Path[0]] {
				lines = append(lines, "  "+c.Path[0])
				seen[c.Path[0]] = true
			}
		}
		return strings.Join(lines, "\n")
	}
	c := *command
	name := strings.Join(c.Path, " ")
	if len(c.Alias) > 0 {
		name = c.Alias[0]
	}
	parts := []string{}
	for _, p := range c.Positional {
		v := "[" + p.Name + "]"
		if p.Required {
			v = "<" + p.Name + ">"
		}
		if p.Variadic {
			v += "..."
		}
		parts = append(parts, v)
	}
	lines := []string{name + " — " + c.Summary, "", "用法: stock-go " + name + " " + strings.Join(parts, " ") + " [--flags]"}
	for _, o := range c.Options {
		extra := ""
		if len(o.Enum) > 0 {
			extra = " (" + strings.Join(o.Enum, "/") + ")"
		}
		lines = append(lines, fmt.Sprintf("  --%-12s %s%s", o.Flag, o.Desc, extra))
	}
	return strings.Join(lines, "\n")
}

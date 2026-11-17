package cli

import "strings"

type Parsed struct {
	Positional []string       `json:"positional"`
	Options    map[string]any `json:"options"`
}

func negativeNumber(s string) bool { return len(s) > 1 && s[0] == '-' && s[1] >= '0' && s[1] <= '9' }

// Parse accepts long/short names, equals values, negative values, repeated flags and --.
func Parse(args []string, booleans map[string]bool) Parsed {
	out := Parsed{Positional: []string{}, Options: map[string]any{}}
	stopped := false
	set := func(key string, value any) {
		if s, ok := value.(string); ok {
			switch old := out.Options[key].(type) {
			case string:
				out.Options[key] = []string{old, s}
				return
			case []string:
				out.Options[key] = append(old, s)
				return
			}
		}
		out.Options[key] = value
	}
	for i := 0; i < len(args); i++ {
		token := args[i]
		if stopped {
			out.Positional = append(out.Positional, token)
			continue
		}
		if token == "--" {
			stopped = true
			continue
		}
		prefix := 0
		if strings.HasPrefix(token, "--") && len(token) > 2 {
			prefix = 2
		} else if strings.HasPrefix(token, "-") && len(token) > 1 && !negativeNumber(token) {
			prefix = 1
		}
		if prefix == 0 {
			out.Positional = append(out.Positional, token)
			continue
		}
		body := token[prefix:]
		if eq := strings.IndexByte(body, '='); eq >= 0 {
			set(body[:eq], body[eq+1:])
			continue
		}
		if booleans[body] {
			set(body, true)
			continue
		}
		if i+1 < len(args) {
			next := args[i+1]
			if next == "-" || !strings.HasPrefix(next, "-") || negativeNumber(next) {
				set(body, next)
				i++
				continue
			}
		}
		set(body, true)
	}
	return out
}

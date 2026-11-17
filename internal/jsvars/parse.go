// Package jsvars extracts top-level JSON literal declarations without evaluating JavaScript.
package jsvars

import (
	"encoding/json"
	"strings"
)

func ident(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
func space(c byte) bool { return c == ' ' || c == '\r' || c == '\n' || c == '\t' }
func trivia(s string, i int) int {
	for i < len(s) {
		if space(s[i]) {
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "//") {
			if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
				i += j + 1
				continue
			}
			return len(s)
		}
		if strings.HasPrefix(s[i:], "/*") {
			if j := strings.Index(s[i+2:], "*/"); j >= 0 {
				i += j + 4
				continue
			}
			return len(s)
		}
		break
	}
	return i
}
func quoted(s string, i int) int {
	quote := s[i]
	i++
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == quote {
			return i + 1
		}
		i++
	}
	return len(s)
}

// Extract returns the first declaration of each requested variable if its complete value is JSON.
// Comments, strings, function bodies and executable expressions are never evaluated.
func Extract(text string, names []string) map[string]json.RawMessage {
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	seen := map[string]bool{}
	out := map[string]json.RawMessage{}
	depth := 0
	for i := 0; i < len(text); {
		i = trivia(text, i)
		if i >= len(text) {
			break
		}
		c := text[i]
		if c == '"' || c == '\'' || c == '`' {
			i = quoted(text, i)
			continue
		}
		if c == '{' || c == '[' || c == '(' {
			depth++
			i++
			continue
		}
		if c == '}' || c == ']' || c == ')' {
			if depth > 0 {
				depth--
			}
			i++
			continue
		}
		if !ident(c) {
			i++
			continue
		}
		start := i
		for i < len(text) && ident(text[i]) {
			i++
		}
		keyword := text[start:i]
		if depth != 0 || (keyword != "var" && keyword != "let" && keyword != "const") {
			continue
		}
		j := trivia(text, i)
		nameStart := j
		for j < len(text) && ident(text[j]) {
			j++
		}
		name := text[nameStart:j]
		j = trivia(text, j)
		if !wanted[name] || seen[name] || j >= len(text) || text[j] != '=' || strings.HasPrefix(text[j:], "==") {
			continue
		}
		seen[name] = true
		j = trivia(text, j+1)
		decoder := json.NewDecoder(strings.NewReader(text[j:]))
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		end := j + int(decoder.InputOffset())
		if err != nil {
			end = valueEnd(text, j)
			normalized := normalizeQuotes(strings.TrimSpace(text[j:end]))
			if !json.Valid([]byte(normalized)) {
				continue
			}
			raw = json.RawMessage(normalized)
		}
		next := trivia(text, end)
		if next < len(text) && text[next] != ';' {
			continue
		}
		out[name] = raw
		i = end
	}
	return out
}

// valueEnd balances delimiters and skips quoted strings. It never evaluates the value.
func valueEnd(text string, start int) int {
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '"', '\'', '`':
			i = quoted(text, i) - 1
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
		case ';':
			if depth == 0 {
				return i
			}
		}
	}
	return len(text)
}
func normalizeQuotes(text string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote == '"' {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(text) {
				i++
				out.WriteByte(text[i])
			} else if c == '"' {
				quote = 0
			}
		} else if quote == '\'' {
			if c == '\\' && i+1 < len(text) {
				i++
				if text[i] == '\'' {
					out.WriteByte('\'')
				} else {
					out.WriteByte('\\')
					out.WriteByte(text[i])
				}
			} else if c == '\'' {
				out.WriteByte('"')
				quote = 0
			} else if c == '"' {
				out.WriteString(`\"`)
			} else {
				out.WriteByte(c)
			}
		} else {
			if c == '"' {
				quote = c
				out.WriteByte(c)
			} else if c == '\'' {
				quote = c
				out.WriteByte('"')
			} else {
				out.WriteByte(c)
			}
		}
	}
	return out.String()
}

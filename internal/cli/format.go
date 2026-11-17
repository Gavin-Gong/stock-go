package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

type pair struct {
	key   string
	value any
}
type ordered []pair

func readValue(d *json.Decoder) (any, error) {
	token, e := d.Token()
	if e != nil {
		return nil, e
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		obj := ordered{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return nil, e
			}
			v, e := readValue(d)
			if e != nil {
				return nil, e
			}
			obj = append(obj, pair{key.(string), v})
		}
		_, e = d.Token()
		return obj, e
	case '[':
		arr := []any{}
		for d.More() {
			v, e := readValue(d)
			if e != nil {
				return nil, e
			}
			arr = append(arr, v)
		}
		_, e = d.Token()
		return arr, e
	}
	return nil, fmt.Errorf("invalid JSON delimiter")
}
func encode(value any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
func compactValue(v any) string {
	switch x := v.(type) {
	case ordered:
		parts := []string{}
		for _, p := range x {
			b, _ := encode(p.key)
			parts = append(parts, string(b)+":"+compactValue(p.value))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := []string{}
		for _, v := range x {
			parts = append(parts, compactValue(v))
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		b, _ := encode(v)
		return string(b)
	}
}
func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return string(x)
	case ordered, []any:
		return compactValue(x)
	default:
		return fmt.Sprint(x)
	}
}
func width(s string) int {
	n := 0
	for _, r := range s {
		wide := (r >= 0x1100 && r <= 0x115f) || (r >= 0x2e80 && r <= 0xa4cf) || (r >= 0xac00 && r <= 0xd7a3) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0xfe30 && r <= 0xfe4f) || (r >= 0xff00 && r <= 0xff60) || (r >= 0xffe0 && r <= 0xffe6) || (r >= 0x20000 && r <= 0x3fffd)
		n++
		if wide {
			n++
		}
	}
	return n
}
func numeric(s string) bool {
	if s == "" {
		return false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if n, e := strconv.ParseFloat(s, 64); e == nil {
		return !math.IsNaN(n)
	}
	if _, e := strconv.ParseInt(s, 0, 64); e == nil {
		return true
	}
	return false
}
func Format(result any, format string, pretty bool) (string, error) {
	body, e := encode(result)
	if e != nil {
		return "", e
	}
	return FormatJSON(body, format, pretty)
}

// FormatJSON preserves incoming object key order when producing table/CSV columns.
func FormatJSON(body []byte, format string, pretty bool) (string, error) {
	if !json.Valid(body) {
		return "", fmt.Errorf("invalid JSON output")
	}
	if format == "json" {
		var b bytes.Buffer
		if pretty {
			e := json.Indent(&b, body, "", "  ")
			return b.String(), e
		}
		e := json.Compact(&b, body)
		return b.String(), e
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	v, e := readValue(d)
	if e != nil {
		return "", e
	}
	rows := []ordered{}
	row := func(v any) ordered {
		if obj, ok := v.(ordered); ok {
			return obj
		}
		return ordered{{"value", v}}
	}
	if arr, ok := v.([]any); ok {
		for _, v := range arr {
			rows = append(rows, row(v))
		}
	} else if v != nil {
		rows = append(rows, row(v))
	}
	if len(rows) == 0 {
		return "", nil
	}
	cols := []string{}
	seen := map[string]bool{}
	for _, r := range rows {
		for _, p := range r {
			if !seen[p.key] {
				cols = append(cols, p.key)
				seen[p.key] = true
			}
		}
	}
	cells := [][]string{}
	for _, r := range rows {
		m := map[string]any{}
		for _, p := range r {
			m[p.key] = p.value
		}
		line := []string{}
		for _, c := range cols {
			line = append(line, cell(m[c]))
		}
		cells = append(cells, line)
	}
	if format == "csv" {
		escape := func(text string) string {
			if strings.ContainsAny(text, ",\"\n\r") {
				return "\"" + strings.ReplaceAll(text, "\"", "\"\"") + "\""
			}
			return text
		}
		line := func(row []string) string {
			parts := make([]string, len(row))
			for i, v := range row {
				parts[i] = escape(v)
			}
			return strings.Join(parts, ",")
		}
		lines := []string{line(cols)}
		for _, row := range cells {
			lines = append(lines, line(row))
		}
		return strings.Join(lines, "\n"), nil
	}

	widths := make([]int, len(cols))
	numbers := make([]bool, len(cols))
	for i, c := range cols {
		widths[i] = width(c)
		numbers[i] = true
		for _, r := range cells {
			widths[i] = max(widths[i], width(r[i]))
			if r[i] != "" && !numeric(r[i]) {
				numbers[i] = false
			}
		}
	}
	render := func(r []string) string {
		parts := []string{}
		for i, s := range r {
			gap := strings.Repeat(" ", max(0, widths[i]-width(s)))
			if numbers[i] {
				s = gap + s
			} else {
				s += gap
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, "  ")
	}
	lines := []string{render(cols)}
	sep := []string{}
	for _, n := range widths {
		sep = append(sep, strings.Repeat("-", n))
	}
	lines = append(lines, strings.Join(sep, "  "))
	for _, r := range cells {
		lines = append(lines, render(r))
	}
	return strings.Join(lines, "\n"), nil
}
func writeLine(w io.Writer, text string) error {
	if text == "" {
		return nil
	}
	_, e := io.WriteString(w, text+"\n")
	return e
}

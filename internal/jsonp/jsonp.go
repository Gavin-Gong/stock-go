// Package jsonp extracts JSON payloads without evaluating JavaScript.
package jsonp

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
)

var callbackPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)*$`)

// Extract accepts JSON or one callback invocation, preceded by block comments.
// Expressions, assignments and trailing executable statements are rejected.
func Extract(body []byte) (json.RawMessage, error) {
	body = bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}))
	for bytes.HasPrefix(body, []byte("/*")) {
		end := bytes.Index(body, []byte("*/"))
		if end < 0 {
			return nil, errors.New("unterminated JSONP comment")
		}
		body = bytes.TrimSpace(body[end+2:])
	}
	if json.Valid(body) {
		return body, nil
	}
	start, end := bytes.IndexByte(body, '('), bytes.LastIndexByte(body, ')')
	if start <= 0 || end <= start || !callbackPattern.Match(bytes.TrimSpace(body[:start])) {
		return nil, errors.New("invalid JSONP callback")
	}
	suffix := bytes.TrimSpace(body[end+1:])
	if len(suffix) > 0 && !bytes.Equal(suffix, []byte(";")) {
		return nil, errors.New("unexpected text after JSONP callback")
	}
	payload := bytes.TrimSpace(body[start+1 : end])
	if !json.Valid(payload) {
		return nil, errors.New("invalid JSONP payload")
	}
	return payload, nil
}

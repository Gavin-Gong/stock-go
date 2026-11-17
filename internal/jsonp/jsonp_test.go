package jsonp

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestExtract(t *testing.T) {
	for _, text := range []string{`{"a":1}`, `[1,null]`, `null`, `cb({"a":1});`, ` /* comment */ cb ( {"a":1} ) `, "\xef\xbb\xbf/*a*//*b*/ ns.cb([\"(x)\",\")\"]);"} {
		payload, err := Extract([]byte(text))
		if err != nil || !json.Valid(payload) {
			t.Fatal(text, string(payload), err)
		}
	}
	for _, text := range []string{"", `/* unterminated`, `cb()`, `cb(1,2)`, `cb({a:1})`, `cb({});alert(1)`, `cb({},alert(1))`, `var cb=({})`, `(()=>({}))()`, `x&&cb({})`, `cb({}) junk`, `cb(1`, `)cb(`, `cb(null);;`} {
		if _, err := Extract([]byte(text)); err == nil {
			t.Fatal("accepted executable/malformed JSONP", text)
		}
	}
	payload, _ := Extract([]byte(`cb({"text":"x(y)"});`))
	if !bytes.Equal(payload, []byte(`{"text":"x(y)"}`)) {
		t.Fatal(string(payload))
	}
}
func FuzzExtract(f *testing.F) {
	for _, s := range []string{`cb({"x":1});`, `/* comment */ cb([1,2])`, `cb(null)`, `cb({});alert(1)`, "\xef\xbb\xbfcb(null)"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		raw, err := Extract([]byte(text))
		if err == nil && !json.Valid(raw) {
			t.Fatalf("non-JSON accepted: %q", raw)
		}
	})
}

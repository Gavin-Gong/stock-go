package gbk

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"unicode/utf8"
)

func TestAllSourceMappings(t *testing.T) {
	body, e := os.ReadFile("../../testdata/gbk-parity.json")
	if e != nil {
		t.Fatal(e)
	}
	var cases map[string]string
	if e = json.Unmarshal(body, &cases); e != nil {
		t.Fatal(e)
	}
	for key, want := range cases {
		n, e := strconv.ParseUint(key, 16, 16)
		if e != nil {
			t.Fatal(e)
		}
		data := []byte{byte(n)}
		if len(key) > 2 {
			data = []byte{byte(n >> 8), byte(n)}
		}
		got, e := Decode(data)
		if e != nil || got != want {
			t.Fatalf("%s: got %q want %q err %v", key, got, want, e)
		}
	}
	t.Logf("%d fixed upstream GBK byte/pair cases passed", len(cases))
}
func FuzzDecode(f *testing.F) {
	f.Add([]byte{0x81, 0xff, 0xa1, 0x40, 65})
	f.Fuzz(func(t *testing.T, body []byte) {
		decoded, e := Decode(body)
		if e != nil || !utf8.ValidString(decoded) {
			t.Fatalf("invalid result %q %v", decoded, e)
		}
	})
}

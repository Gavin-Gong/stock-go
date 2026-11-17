package jsvars

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExtractLiteralsWithoutExecution(t *testing.T) {
	text := `// var code="comment";
 /* var name="comment"; */
 "var code=\"string\";";
 function never(){var code="nested";}
 var code="110011";
 let name="测试基金; \\\"";
 const data={"text":"}; var code=evil","rows":[1,{"x":null}],"ok":true};
 var bad=[1].concat([2]); var invoke=(function(){throw "must not run"})();
 var single='not JSON';var missing=undefined;
 `
	got := Extract(text, []string{"code", "name", "data", "bad", "invoke", "single", "missing"})
	if len(got) != 4 {
		t.Fatalf("%v", got)
	}
	var code string
	if err := json.Unmarshal(got["code"], &code); err != nil || code != "110011" {
		t.Fatalf("%s %v", code, err)
	}
	var data map[string]any
	if err := json.Unmarshal(got["data"], &data); err != nil || data["ok"] != true {
		t.Fatalf("%v %v", data, err)
	}
}
func TestValuesAndFirstDeclaration(t *testing.T) {
	got := Extract(`var a=123;var b=false;var c=null;var d=[1,2];var a=999;`, []string{"a", "b", "c", "d"})
	want := map[string]json.RawMessage{"a": json.RawMessage("123"), "b": json.RawMessage("false"), "c": json.RawMessage("null"), "d": json.RawMessage("[1,2]")}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if got := Extract(`var a=1+2;var a=3;var b=5 /* trailing comment */;`, []string{"a", "b"}); len(got) != 1 || string(got["b"]) != "5" {
		t.Fatal(got)
	}
	if got := Extract(`var b=2;`, []string{"a"}); len(got) != 0 {
		t.Fatal("declaration leaked from earlier call")
	}
}
func FuzzExtract(f *testing.F) {
	f.Add(`var x={"a":[1,";"]};`)
	f.Add(`/*`)
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 10000 {
			text = text[:10000]
		}
		for _, raw := range Extract(text, []string{"x", "code", "data"}) {
			if !json.Valid(raw) {
				t.Fatalf("invalid JSON: %s", raw)
			}
		}
	})
}

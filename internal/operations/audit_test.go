package operations

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	stockgo "github.com/Gavin-Gong/stock-go"
)

func TestActualUpstreamAPICoverage(t *testing.T) {
	body, e := os.ReadFile("../../testdata/api-audit.json")
	if e != nil {
		t.Fatal(e)
	}
	var audit struct {
		SourceMethods, GoBindings, CLIMethods []string
		LifecycleMethods                      []struct{ Source, Go string }
		MCPTools                              []struct{ Name, Path string }
	}
	if e = json.Unmarshal(body, &audit); e != nil {
		t.Fatal(e)
	}
	sdk, e := stockgo.New()
	if e != nil {
		t.Fatal(e)
	}
	if len(audit.LifecycleMethods) != 1 || audit.LifecycleMethods[0].Source != "clearCaches" || audit.LifecycleMethods[0].Go != "SDK.ClearCaches" {
		t.Fatal("SDK lifecycle audit drift")
	}
	if _, ok := reflect.TypeOf(sdk).MethodByName("ClearCaches"); !ok {
		t.Fatal("missing SDK.ClearCaches")
	}
	r, e := New(sdk)
	if e != nil {
		t.Fatal(e)
	}
	actual := []string{}
	for path := range r.handlers {
		actual = append(actual, path)
	}
	sort.Strings(actual)
	if len(actual) != 94 || !reflect.DeepEqual(actual, audit.SourceMethods) || !reflect.DeepEqual(actual, audit.GoBindings) || !reflect.DeepEqual(actual, audit.CLIMethods) {
		t.Fatal("public namespace coverage drift")
	}
	declared := []string{}
	for _, spec := range Specs() {
		declared = append(declared, strings.Join(spec.Path, "."))
	}
	sort.Strings(declared)
	if !reflect.DeepEqual(actual, declared) {
		t.Fatal("manifest does not match actual SDK")
	}
	if len(audit.MCPTools) != 93 {
		t.Fatal(len(audit.MCPTools))
	}
}

// Read declarations from Go syntax trees so the saved mapping cannot claim an
// entry exists merely by listing its name in the maintainer's TypeScript script.
func TestPublicHelperDeclarations(t *testing.T) {
	body, e := os.ReadFile("../../testdata/api-audit.json")
	if e != nil {
		t.Fatal(e)
	}
	var audit struct {
		PendingPublicHelpers []string
		PublicHelperGroups   []struct {
			Name, GoPackage string
			Mappings        [][2]string
		}
	}
	if e = json.Unmarshal(body, &audit); e != nil {
		t.Fatal(e)
	}
	if len(audit.PendingPublicHelpers) != 0 || len(audit.PublicHelperGroups) != 5 {
		t.Fatal("public helper audit incomplete")
	}
	for _, group := range audit.PublicHelperGroups {
		dir := filepath.Join("../..", group.GoPackage)
		packages, e := parser.ParseDir(token.NewFileSet(), dir, func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
		if e != nil {
			t.Fatal(e)
		}
		names := map[string]bool{}
		for _, pkg := range packages {
			for _, file := range pkg.Files {
				for _, decl := range file.Decls {
					switch d := decl.(type) {
					case *ast.FuncDecl:
						if d.Recv == nil {
							names[d.Name.Name] = true
						}
					case *ast.GenDecl:
						for _, spec := range d.Specs {
							switch s := spec.(type) {
							case *ast.ValueSpec:
								for _, name := range s.Names {
									names[name.Name] = true
								}
							case *ast.TypeSpec:
								names[s.Name.Name] = true
							}
						}
					}
				}
			}
		}
		for _, mapping := range group.Mappings {
			if !ast.IsExported(mapping[1]) || !names[mapping[1]] {
				t.Errorf("%s.%s -> %s: missing Go declaration", group.Name, mapping[0], mapping[1])
			}
		}
	}
}

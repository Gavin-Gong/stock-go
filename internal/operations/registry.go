// Package operations binds the pinned stock-sdk method manifest to typed Go SDK calls.
package operations

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	stockgo "github.com/Gavin-Gong/stock-go"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
)

type Param struct {
	Flag, Field, JSONKey, Type, Desc, MCPDesc string
	Enum                                      []string
	Map                                       map[string]string
	Upper, Required                           bool
	Default                                   any
	CLI, MCP                                  *bool
}
type Positional struct {
	Name, Desc                string
	Required, Variadic, Upper bool
	Enum                      []string
	Default                   any
}
type Spec struct {
	Path                                                  []string
	ToolName, Tier, Summary, MCPDesc, ArgShape, CodesDesc string
	Positional                                            []Positional
	Params                                                []Param
	MCP                                                   *bool
	MCPCustom                                             bool
}

//go:embed methods.json
var manifest []byte
var specs = func() []Spec {
	var out []Spec
	if err := json.Unmarshal(manifest, &out); err != nil {
		panic(err)
	}
	return out
}()

// Specs returns an independent copy so consumers cannot mutate the shared manifest.
func Specs() []Spec { var out []Spec; _ = json.Unmarshal(manifest, &out); return out }

type Handler func(context.Context, []any) (any, error)
type Registry struct{ handlers map[string]Handler }

func usage(message string) error {
	return &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: message}
}
func decode[T any](value any) (T, error) {
	var result T
	body, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(body, &result)
	}
	if err != nil {
		return result, usage("invalid method argument: " + err.Error())
	}
	return result, nil
}
func arity(args []any, min, max int) error {
	if len(args) < min || len(args) > max {
		return usage(fmt.Sprintf("expected %d..%d arguments, got %d", min, max, len(args)))
	}
	return nil
}
func zero[T any](fn func(context.Context) (T, error)) Handler {
	return func(ctx context.Context, args []any) (any, error) {
		if e := arity(args, 0, 0); e != nil {
			return nil, e
		}
		return fn(ctx)
	}
}
func one[A, T any](fn func(context.Context, A) (T, error)) Handler {
	return func(ctx context.Context, args []any) (any, error) {
		if e := arity(args, 1, 1); e != nil {
			return nil, e
		}
		a, e := decode[A](args[0])
		if e != nil {
			return nil, e
		}
		return fn(ctx, a)
	}
}
func two[A, B, T any](fn func(context.Context, A, B) (T, error)) Handler {
	return func(ctx context.Context, args []any) (any, error) {
		if e := arity(args, 2, 2); e != nil {
			return nil, e
		}
		a, e := decode[A](args[0])
		if e != nil {
			return nil, e
		}
		b, e := decode[B](args[1])
		if e != nil {
			return nil, e
		}
		return fn(ctx, a, b)
	}
}
func opt[O, T any](fn func(context.Context, ...O) (T, error)) Handler {
	return func(ctx context.Context, args []any) (any, error) {
		if e := arity(args, 0, 1); e != nil {
			return nil, e
		}
		if len(args) == 0 || args[0] == nil {
			return fn(ctx)
		}
		o, e := decode[O](args[0])
		if e != nil {
			return nil, e
		}
		return fn(ctx, o)
	}
}
func withOpt[A, O, T any](fn func(context.Context, A, ...O) (T, error)) Handler {
	return func(ctx context.Context, args []any) (any, error) {
		if e := arity(args, 1, 2); e != nil {
			return nil, e
		}
		a, e := decode[A](args[0])
		if e != nil {
			return nil, e
		}
		if len(args) == 1 || args[1] == nil {
			return fn(ctx, a)
		}
		o, e := decode[O](args[1])
		if e != nil {
			return nil, e
		}
		return fn(ctx, a, o)
	}
}
func (r *Registry) Invoke(ctx context.Context, path string, args []any) (any, error) {
	handler := r.handlers[path]
	if handler == nil {
		return nil, usage("unknown method: " + path)
	}
	prepared, err := prepare(path, args)
	if err != nil {
		return nil, err
	}
	return handler(ctx, prepared)
}
func (r *Registry) Has(path string) bool { return r.handlers[path] != nil }

// New checks all 94 bindings eagerly; a manifest change cannot silently omit a method.
func New(sdk *stockgo.SDK) (*Registry, error) {
	if sdk == nil {
		return nil, usage("SDK must not be nil")
	}
	r := &Registry{handlers: bindings(sdk)}
	for _, s := range specs {
		key := strings.Join(s.Path, ".")
		if r.handlers[key] == nil {
			return nil, fmt.Errorf("missing Go binding: %s", key)
		}
	}
	if len(r.handlers) != len(specs) {
		return nil, fmt.Errorf("method manifest/bindings count mismatch")
	}
	return r, nil
}

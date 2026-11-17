// Package screener provides pure local screening and single-asset long-only backtests.
package screener

import (
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type Direction string

const (
	Ascending  Direction = "asc"
	Descending Direction = "desc"
)

// Builder owns a copy of the input. Errors remain attached until Err/Top/ToArray is called.
type Builder[T any] struct {
	items []T
	err   error
}

func Screen[T any](items []T) *Builder[T] { return &Builder[T]{items: append([]T{}, items...)} }
func (s *Builder[T]) Where(predicate func(T) bool) *Builder[T] {
	if s.err != nil {
		return s
	}
	if predicate == nil {
		s.err = invalid("where predicate must not be nil")
		return s
	}
	out := make([]T, 0, len(s.items))
	for _, item := range s.items {
		if predicate(item) {
			out = append(out, item)
		}
	}
	s.items = out
	return s
}

// SortBy is stable; nil and nonfinite values always sort last in either direction.
func (s *Builder[T]) SortBy(selector func(T) *float64, directions ...Direction) *Builder[T] {
	if s.err != nil {
		return s
	}
	d := Descending
	if len(directions) > 1 {
		s.err = invalid("at most one direction is allowed")
		return s
	}
	if len(directions) == 1 {
		d = directions[0]
	}
	if d != Ascending && d != Descending {
		s.err = invalid("direction must be asc or desc")
		return s
	}
	if selector == nil {
		s.err = invalid("sort selector must not be nil")
		return s
	}
	sort.SliceStable(s.items, func(i, j int) bool {
		a, b := selector(s.items[i]), selector(s.items[j])
		validA, validB := valid(a), valid(b)
		if !validA {
			return false
		}
		if !validB {
			return true
		}
		if d == Ascending {
			return *a < *b
		}
		return *a > *b
	})
	return s
}
func (s *Builder[T]) Err() error { return s.err }
func (s *Builder[T]) Top(n int) ([]T, error) {
	if s.err != nil {
		return nil, s.err
	}
	if n < 0 {
		return nil, invalid("top n must be nonnegative")
	}
	return append([]T{}, s.items[:min(n, len(s.items))]...), nil
}
func (s *Builder[T]) ToArray() ([]T, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]T{}, s.items...), nil
}
func valid(v *float64) bool  { return v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) }
func invalid(s string) error { return &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: s} }

// SortByValue accepts numeric strings from raw JSON as well as Go numeric values.
func (s *Builder[T]) SortByValue(selector func(T) any, direction ...Direction) *Builder[T] {
	if selector == nil {
		s.err = invalid("sort selector must not be nil")
		return s
	}
	return s.SortBy(func(item T) *float64 { return SortNumber(selector(item)) }, direction...)
}

// SortNumber uses strict whole-string conversion, unlike quote parsing's numeric prefixes.
func SortNumber(value any) *float64 {
	if value == nil {
		return nil
	}
	if s, ok := value.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		if len(s) > 2 && s[0] == '0' && strings.ContainsRune("xXoObB", rune(s[1])) {
			n, err := strconv.ParseUint(s, 0, 64)
			if err != nil {
				return nil
			}
			v := float64(n)
			return &v
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || !valid(&n) {
			return nil
		}
		return &n
	}
	v := reflect.ValueOf(value)
	var n float64
	switch v.Kind() {
	case reflect.String:
		return SortNumber(v.String())
	case reflect.Float32, reflect.Float64:
		n = v.Float()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n = float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n = float64(v.Uint())
	default:
		return nil
	}
	if !valid(&n) {
		return nil
	}
	return &n
}

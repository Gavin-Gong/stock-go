package cache

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Scope owns named caches without a global registry keyed by client pointers.
type Scope struct {
	mu     sync.Mutex
	caches map[string]*MemoryCache[any]
}

func NewScope() *Scope { return &Scope{caches: map[string]*MemoryCache[any]{}} }
func (s *Scope) Get(namespace string, o ...Options) (*MemoryCache[any], error) {
	cfg, e := options(o)
	if e != nil {
		return nil, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.caches[namespace]; c != nil {
		return c, nil
	}
	if s.caches == nil {
		s.caches = map[string]*MemoryCache[any]{}
	}
	c, e := New[any](cfg)
	if e == nil {
		s.caches[namespace] = c
	}
	return c, e
}
func (s *Scope) Configure(namespace string, o UpdateOptions) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.caches[namespace]
	if c == nil {
		return false, nil
	}
	return true, c.Configure(o)
}
func (s *Scope) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.caches {
		c.Clear()
	}
	s.caches = map[string]*MemoryCache[any]{}
}

var shared = NewScope()

func GetShared(namespace string, o ...Options) (*MemoryCache[any], error) {
	return shared.Get(namespace, o...)
}
func ConfigureShared(namespace string, o UpdateOptions) (bool, error) {
	return shared.Configure(namespace, o)
}

// ClearShared retains named shared cache instances, matching the upstream shared-cache semantics.
func ClearShared() {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	for _, c := range shared.caches {
		c.Clear()
	}
}

type ScopedOwner interface {
	CacheScope() *Scope
	ClearCaches()
}

func nilOwner(owner ScopedOwner) bool {
	v := reflect.ValueOf(owner)
	return !v.IsValid() || (v.Kind() == reflect.Pointer && v.IsNil())
}
func GetClientScoped(owner ScopedOwner, namespace string, o ...Options) (*MemoryCache[any], error) {
	if nilOwner(owner) {
		return nil, invalid("cache owner is nil")
	}
	scope := owner.CacheScope()
	if scope == nil {
		return nil, invalid("cache owner scope is nil")
	}
	return scope.Get(namespace, o...)
}
func ClearClientScoped(owner ScopedOwner) {
	if !nilOwner(owner) {
		owner.ClearCaches()
	}
}

// CreateKey preserves optional positions using ∅. Like the source helper, ':' is not escaped.
func CreateKey(parts ...any) string {
	values := make([]string, len(parts))
	for i, p := range parts {
		switch v := p.(type) {
		case nil:
			values[i] = "∅"
		case string:
			values[i] = v
		case bool:
			values[i] = strconv.FormatBool(v)
		case json.Number:
			n, e := v.Float64()
			if e == nil {
				values[i] = numberKey(n)
			} else {
				values[i] = string(v)
			}
		case float64:
			values[i] = numberKey(v)
		case float32:
			values[i] = numberKey(float64(v))
		default:
			values[i] = fmt.Sprint(v)
		}
	}
	return strings.Join(values, ":")
}

func numberKey(value float64) string {
	if value == 0 {
		return "0"
	}
	raw, e := json.Marshal(value)
	if e == nil {
		return string(raw)
	}
	switch {
	case value > 0:
		return "Infinity"
	case value < 0:
		return "-Infinity"
	default:
		return "NaN"
	}
}

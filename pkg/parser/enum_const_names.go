package parser

import (
	"reflect"
	"sort"
	"strings"
)

// ReferencedIdentifiers lists, sorted, the identifier names an expression
// mentions (declarations and property names included - callers only use it to
// decide which enum members an initializer might refer to).
func ReferencedIdentifiers(expr Expression) []string {
	if expr == nil {
		return nil
	}
	seen := map[string]bool{}
	visited := map[uintptr]bool{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Ptr:
			if v.IsNil() {
				return
			}
			elem := v.Elem()
			if elem.Kind() != reflect.Struct || !strings.HasSuffix(elem.Type().PkgPath(), "pkg/parser") {
				return
			}
			if visited[v.Pointer()] {
				return
			}
			visited[v.Pointer()] = true
			if id, ok := v.Interface().(*Identifier); ok {
				seen[id.Value] = true
				return
			}
			walk(elem)
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				f := v.Field(i)
				if f.CanInterface() {
					walk(f)
				}
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(expr))
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

package checker

import (
	"reflect"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// collectInferNames lists the type parameters a conditional type's extends
// clause declares with `infer X`. They are in scope in the true branch only.
func collectInferNames(node parser.Expression) []string {
	if node == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
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
			if ite, ok := v.Interface().(*parser.InferTypeExpression); ok {
				if ite.TypeParameter != "" && !seen[ite.TypeParameter] {
					seen[ite.TypeParameter] = true
					names = append(names, ite.TypeParameter)
				}
				return
			}
			walk(elem)
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				f := v.Field(i)
				if !f.CanInterface() {
					continue
				}
				// Skip computed type caches: they hold checker types, not syntax.
				if f.Type() == reflect.TypeOf((*types.Type)(nil)).Elem() {
					continue
				}
				walk(f)
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(node))
	sort.Strings(names)
	return names
}

// withInferScope resolves fn with the infer type parameters of extendsType
// bound as InferType placeholders, which type substitution replaces once the
// conditional type is evaluated.
func (c *Checker) withInferScope(extendsType parser.Expression, fn func()) {
	names := collectInferNames(extendsType)
	if len(names) == 0 {
		fn()
		return
	}
	saved := c.env
	c.env = NewEnclosedEnvironment(saved)
	for _, name := range names {
		c.env.DefineTypeAlias(name, &types.InferType{TypeParameter: name})
	}
	defer func() { c.env = saved }()
	fn()
}

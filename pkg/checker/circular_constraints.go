package checker

import (
	"fmt"
	"reflect"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

var typeParameterListType = reflect.TypeOf([]*parser.TypeParameter(nil))

// checkCircularTypeParameterConstraints reports TS2313 for every type
// parameter whose constraint leads back to itself through the constraints of
// its sibling parameters (`<T extends U, U extends T>`), as tsc's
// getConstraintOfTypeParameter does when resolving the base constraint
// circles. Only constraints that name another parameter of the same list are
// followed: that is the whole chain such a cycle can take.
// FIXME: cycles through indexed access or keyof constraints are not seen.
func (c *Checker) checkCircularTypeParameterConstraints(program *parser.Program) {
	if program == nil {
		return
	}
	seen := map[uintptr]bool{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		for v.Kind() == reflect.Interface {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		if v.Kind() == reflect.Ptr {
			if v.IsNil() || v.Elem().Kind() != reflect.Struct || v.Type().Elem().PkgPath() != nodeInterface.PkgPath() {
				return
			}
			if seen[v.Pointer()] {
				return
			}
			seen[v.Pointer()] = true
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanInterface() {
				continue
			}
			switch f.Kind() {
			case reflect.Ptr, reflect.Interface:
				walk(f)
			case reflect.Slice:
				if f.Type() == typeParameterListType {
					c.reportCircularConstraints(f.Interface().([]*parser.TypeParameter))
				}
				for j := 0; j < f.Len(); j++ {
					walk(f.Index(j))
				}
			}
		}
	}
	walk(reflect.ValueOf(program))
}

func (c *Checker) reportCircularConstraints(params []*parser.TypeParameter) {
	if len(params) == 0 {
		return
	}
	index := map[string]int{}
	for i, p := range params {
		if p != nil && p.Name != nil {
			if _, dup := index[p.Name.Value]; !dup {
				index[p.Name.Value] = i
			}
		}
	}
	next := make([]int, len(params))
	for i, p := range params {
		next[i] = -1
		if p == nil {
			continue
		}
		if id, ok := p.Constraint.(*parser.Identifier); ok {
			if j, found := index[id.Value]; found {
				next[i] = j
			}
		}
	}
	for i, p := range params {
		// i is on a cycle when following the chain from i comes back to i.
		j := next[i]
		for steps := 0; j >= 0 && j != i && steps < len(params); steps++ {
			j = next[j]
		}
		if j == i {
			c.addErrorWithCode(p.Constraint, errors.TS2313, fmt.Sprintf("Type parameter '%s' has a circular constraint.", p.Name.Value))
		}
	}
}

// withComputedKeyOwnTypeParams runs fn, which checks a class body, with the
// class's own type parameters recorded for its member keys (TS2467).
func (c *Checker) withComputedKeyOwnTypeParams(params []*types.TypeParameter, fn func() *types.ObjectType) *types.ObjectType {
	saved := c.computedKeyOwnTypeParams
	c.computedKeyOwnTypeParams = params
	defer func() { c.computedKeyOwnTypeParams = saved }()
	return fn()
}

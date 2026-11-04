package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// downlevelArrayIteration reports whether for-of and array destructuring
// iterate only arrays (and strings): a target below ES2015 without
// --downlevelIteration, as in tsc's getIteratedTypeOrElementType.
func (c *Checker) downlevelArrayIteration() bool {
	return c.target() < 2015 && !c.downlevelIteration
}

// isArrayLikeForIteration is tsc's isArrayLikeType: assignable to
// `readonly any[]`.
func (c *Checker) isArrayLikeForIteration(t types.Type, depth int) bool {
	if t == nil || depth > 8 {
		return true
	}
	switch tt := t.(type) {
	case *types.Primitive:
		return tt == types.Any
	case *types.ArrayType, *types.TupleType:
		return true
	case *types.ReadonlyType:
		return c.isArrayLikeForIteration(tt.InnerType, depth+1)
	case *types.UnionType:
		for _, m := range tt.Types {
			if !c.isArrayLikeForIteration(m, depth+1) {
				return false
			}
		}
		return true
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if c.isArrayLikeForIteration(m, depth+1) {
				return true
			}
		}
		return false
	case *types.TypeParameterType:
		if tt.Parameter == nil || tt.Parameter.Constraint == nil {
			return false
		}
		return c.isArrayLikeForIteration(tt.Parameter.Constraint, depth+1)
	case *types.LiteralType, *types.EnumMemberType, *types.TemplateLiteralType:
		return false
	case *types.ObjectType:
		return c.rel.IsAssignable(tt, &types.ReadonlyType{InnerType: &types.ArrayType{ElementType: types.Any}})
	}
	// Types not modelled here (deferred generics and the like): assume an
	// array rather than report.
	return true
}

func isStringLikeForIteration(t types.Type) bool {
	switch tt := t.(type) {
	case *types.Primitive:
		return tt == types.String
	case *types.LiteralType:
		return tt.Value.IsString()
	case *types.TemplateLiteralType:
		return true
	}
	return false
}

// reportDownlevelIteration checks the type iterated by a for-of (allowStrings)
// or destructured by an array pattern below ES2015, as tsc's
// getIteratedTypeOrElementType does without the iteration protocol: what is
// left once strings are removed must be array-like. It reports TS2802 when
// the type is iterable (the protocol would do), else TS2461 or TS2495, and
// says whether it reported.
func (c *Checker) reportDownlevelIteration(node parser.Node, t types.Type, allowStrings bool) bool {
	if !c.downlevelArrayIteration() || t == nil {
		return false
	}
	arrayType := t
	hasString := false
	if allowStrings {
		if u, ok := t.(*types.UnionType); ok {
			var kept []types.Type
			for _, m := range u.Types {
				if !isStringLikeForIteration(m) {
					kept = append(kept, m)
				}
			}
			if len(kept) != len(u.Types) {
				hasString = true
				if len(kept) == 0 {
					return false
				}
				arrayType = types.NewUnionType(kept...)
			}
		} else if isStringLikeForIteration(t) {
			return false
		}
	}
	if c.isArrayLikeForIteration(arrayType, 0) {
		return false
	}
	shown := arrayType.String()
	switch {
	case isStringLikeForIteration(t) || (!c.isDefinitelyNotIterable(arrayType) && (c.isGeneratorType(arrayType) || c.isSpreadableIterableType(arrayType))):
		c.addErrorWithCode(node, errors.TS2802, fmt.Sprintf("Type '%s' can only be iterated through when using the '--downlevelIteration' flag or with a '--target' of 'es2015' or higher.", shown))
	case allowStrings && !hasString:
		c.addErrorWithCode(node, errors.TS2495, fmt.Sprintf("Type '%s' is not an array type or a string type.", shown))
	default:
		c.addErrorWithCode(node, errors.TS2461, fmt.Sprintf("Type '%s' is not an array type.", shown))
	}
	return true
}

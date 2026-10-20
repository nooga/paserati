package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/types"
)

// unionSignatures is tsc's getUnionSignatures for the call (or construct)
// signatures of a union's members, restricted to non-generic signatures
// without `this` parameters and with plain array rest parameters. A signature
// is kept when every member has one matching it (the same parameters, or as a
// fallback one needing no more arguments whose parameters accept its own); it
// then returns the union of the matches' return types. When nothing matches
// and at most one member is overloaded, each of that member's signatures is
// combined with the other members' first signature. ok is false when the
// members are not all like that, and the caller keeps its own handling.
func (c *Checker) unionSignatures(u *types.UnionType, construct bool) (sigs []*types.Signature, ok bool) {
	var lists [][]*types.Signature
	overloaded := -2 // -2: none, -1: several members, else that member's index
	for i, m := range u.Types {
		obj, isObj := m.(*types.ObjectType)
		if !isObj {
			return nil, false
		}
		list := obj.CallSigs()
		if construct {
			list = obj.ConstructSigs()
		}
		if len(list) == 0 {
			return nil, false
		}
		for _, s := range list {
			if len(s.TypeParameters) > 0 || s.ThisType != nil {
				return nil, false
			}
			if s.RestParameterType != nil {
				if _, isArr := s.RestParameterType.(*types.ArrayType); !isArr {
					return nil, false
				}
			}
		}
		if len(list) > 1 {
			if overloaded == -2 {
				overloaded = i
			} else {
				overloaded = -1
			}
		}
		lists = append(lists, list)
	}
	if len(lists) < 2 {
		return nil, false
	}

	for i, l := range lists {
		for _, s := range l {
			if c.findMatchingUnionSignature(sigs, s, false) != nil {
				continue
			}
			var matches []*types.Signature
			for j, other := range lists {
				var match *types.Signature
				if j == i {
					match = s
				} else if match = c.findMatchingUnionSignature(other, s, false); match == nil {
					match = c.findMatchingUnionSignature(other, s, true)
				}
				if match == nil {
					matches = nil
					break
				}
				if !containsSignature(matches, match) {
					matches = append(matches, match)
				}
			}
			if matches == nil {
				continue
			}
			combined := *s
			if len(matches) > 1 {
				returns := make([]types.Type, len(matches))
				for k, m := range matches {
					returns[k] = m.ReturnType
				}
				combined.ReturnType = types.NewUnionType(returns...)
			}
			sigs = append(sigs, &combined)
		}
	}
	if len(sigs) > 0 || overloaded == -1 {
		return sigs, true
	}

	master := 0
	if overloaded >= 0 {
		master = overloaded
	}
	for _, s := range lists[master] {
		acc := s
		for j, l := range lists {
			if j != master {
				acc = c.combineUnionSignatures(acc, l[0])
			}
		}
		sigs = append(sigs, acc)
	}
	return sigs, true
}

func containsSignature(list []*types.Signature, s *types.Signature) bool {
	for _, o := range list {
		if o == s {
			return true
		}
	}
	return false
}

// paramCount is tsc's getParameterCount: the parameters, a rest counting once.
func paramCount(s *types.Signature) int {
	if s.RestParameterType != nil {
		return len(s.ParameterTypes) + 1
	}
	return len(s.ParameterTypes)
}

// typeAtPosition is tsc's tryGetTypeAtPosition: the type an argument at
// position i is checked against, nil past the parameters.
func typeAtPosition(s *types.Signature, i int) types.Type {
	if i < len(s.ParameterTypes) {
		return s.ParameterTypes[i]
	}
	if arr, ok := s.RestParameterType.(*types.ArrayType); ok {
		return arr.ElementType
	}
	return nil
}

// findMatchingUnionSignature is tsc's findMatchingSignature ignoring return
// types: a signature of list with s's parameter counts and identical
// parameter types, or with partial, one needing no more arguments than s
// whose parameters accept s's.
func (c *Checker) findMatchingUnionSignature(list []*types.Signature, s *types.Signature, partial bool) *types.Signature {
	for _, o := range list {
		if o == s {
			return o
		}
		exact := paramCount(o) == paramCount(s) && requiredParamCount(o) == requiredParamCount(s) &&
			(o.RestParameterType != nil) == (s.RestParameterType != nil)
		if !exact && !(partial && requiredParamCount(o) <= requiredParamCount(s)) {
			continue
		}
		related := true
		for i := 0; i < paramCount(s) && related; i++ {
			ot, st := typeAtPosition(o, i), typeAtPosition(s, i)
			if ot == nil {
				ot = types.Any
			}
			if partial {
				related = c.rel.IsAssignable(st, ot)
			} else {
				related = c.rel.IsAssignable(st, ot) && c.rel.IsAssignable(ot, st)
			}
		}
		if related {
			return o
		}
	}
	return nil
}

// combineUnionSignatures is tsc's combineSignaturesOfUnionMembers: each
// parameter is the intersection of both signatures' parameters at its
// position (unknown where one has none), the argument count needed is the
// larger, a rest parameter survives, and the return types unite.
func (c *Checker) combineUnionSignatures(left, right *types.Signature) *types.Signature {
	longest, shorter := left, right
	if paramCount(right) > paramCount(left) {
		longest, shorter = right, left
	}
	count := paramCount(longest)
	eitherRest := left.RestParameterType != nil || right.RestParameterType != nil
	extraRest := eitherRest && longest.RestParameterType == nil
	minLongest, minShorter := requiredParamCount(longest), requiredParamCount(shorter)
	required := minLongest
	if minShorter > required {
		required = minShorter
	}
	combined := &types.Signature{ReturnType: types.NewUnionType(left.ReturnType, right.ReturnType)}
	for i := 0; i < count; i++ {
		lt := typeAtPosition(longest, i)
		st := typeAtPosition(shorter, i)
		t := lt
		if st != nil {
			t = c.intersectParameterTypes([]types.Type{lt, st})
		}
		name := paramNameAt(left, i)
		if rn := paramNameAt(right, i); name == "" || (rn != "" && rn != name) {
			if name == "" {
				name = rn
			} else {
				name = fmt.Sprintf("arg%d", i)
			}
		}
		if eitherRest && !extraRest && i == count-1 {
			combined.RestParameterType = &types.ArrayType{ElementType: t}
			combined.IsVariadic = true
			continue
		}
		combined.ParameterTypes = append(combined.ParameterTypes, t)
		combined.ParameterNames = append(combined.ParameterNames, name)
		combined.OptionalParams = append(combined.OptionalParams, i >= required)
	}
	if extraRest {
		t := typeAtPosition(shorter, count)
		if t == nil {
			t = types.Any
		}
		combined.RestParameterType = &types.ArrayType{ElementType: t}
		combined.IsVariadic = true
	}
	return combined
}

func paramNameAt(s *types.Signature, i int) string {
	if i < len(s.ParameterNames) {
		return s.ParameterNames[i]
	}
	return ""
}

// intersectParameterTypes is the intersection of parameter types, never for
// disjoint primitives (number & string).
func (c *Checker) intersectParameterTypes(parts []types.Type) types.Type {
	acc := parts[0]
	for _, p := range parts[1:] {
		switch {
		case c.rel.IsAssignable(acc, p):
		case c.rel.IsAssignable(p, acc):
			acc = p
		case isPrimitiveLike(acc) && isPrimitiveLike(p):
			return types.Never
		default:
			acc = types.NewIntersectionType(acc, p)
		}
	}
	return acc
}

func isPrimitiveLike(t types.Type) bool {
	switch t {
	case types.Number, types.String, types.Boolean, types.BigInt, types.Symbol, types.Null, types.Undefined:
		return true
	}
	_, lit := t.(*types.LiteralType)
	return lit
}

// requiredParamCount is the number of leading non-optional parameters.
func requiredParamCount(s *types.Signature) int {
	n := 0
	for i := range s.ParameterTypes {
		if i < len(s.OptionalParams) && s.OptionalParams[i] {
			break
		}
		n++
	}
	return n
}

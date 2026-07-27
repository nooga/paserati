package types

import (
	"github.com/nooga/paserati/pkg/vm"
)

// Splitting a type by truthiness is what gives `&&`, `||` and `??` their
// result types. `a && b` can only produce a falsy `a` or whatever `b` is, and
// `a || b` only a truthy `a` or whatever `b` is, so each operator unions one
// half of its left operand with its right one.

// FalseType is the `false` literal type, the falsy half of `boolean`.
var FalseType Type = &LiteralType{Value: vm.BooleanValue(false)}

// TrueType is the `true` literal type, the truthy half of `boolean`.
var TrueType Type = &LiteralType{Value: vm.BooleanValue(true)}

// emptyStringType and zeroType are the falsy inhabitants TypeScript names for
// `string` and `number`. There is no such single value for an object type,
// which is why those collapse to never below.
var (
	emptyStringType Type = &LiteralType{Value: vm.String("")}
	zeroType        Type = &LiteralType{Value: vm.Number(0)}
)

// ExtractFalsyTypes returns the part of a type that could be falsy, or Never
// if none of it could be. This is the left half of `a && b`: when `a` is
// falsy, `a && b` evaluates to `a`, and this is what `a` can be in that case.
func ExtractFalsyTypes(t Type) Type {
	if t == nil {
		return Never
	}
	if union, ok := t.(*UnionType); ok {
		parts := make([]Type, 0, len(union.Types))
		for _, member := range union.Types {
			parts = append(parts, ExtractFalsyTypes(member))
		}
		return NewUnionType(parts...)
	}
	switch t {
	case String:
		return emptyStringType
	case Number:
		return zeroType
	case BigInt:
		return zeroType
	case Boolean:
		return FalseType
	case Null, Undefined, Void:
		return t
	case Any, Unknown:
		// Anything at all could be falsy, and there is no smaller type to
		// name, so the whole thing stands.
		return t
	}
	if lit, ok := t.(*LiteralType); ok {
		if !lit.Value.IsTruthy() {
			return t
		}
		return Never
	}
	// Objects, arrays, functions and the rest are always truthy.
	return Never
}

// RemoveFalsyTypes returns the part of a type that could be truthy, or Never
// if none of it could be. This is the left half of `a || b`.
func RemoveFalsyTypes(t Type) Type {
	if t == nil {
		return Never
	}
	if union, ok := t.(*UnionType); ok {
		parts := make([]Type, 0, len(union.Types))
		for _, member := range union.Types {
			parts = append(parts, RemoveFalsyTypes(member))
		}
		return NewUnionType(parts...)
	}
	switch t {
	case Null, Undefined, Void, Never:
		return Never
	case Boolean:
		return TrueType
	case Any, Unknown:
		return t
	}
	if lit, ok := t.(*LiteralType); ok {
		if lit.Value.IsTruthy() {
			return t
		}
		return Never
	}
	// `string`, `number` and `bigint` keep their full type: TypeScript does not
	// narrow them to "non-empty string" or "non-zero number", since neither is
	// expressible.
	return t
}

// UnionWithSubtypeReduction builds a union and then drops any member already
// covered by another, so `number | 2` reduces to `number`.
//
// TypeScript applies this reduction to `||` and `??` but not to `&&`. The
// asymmetry is deliberate on its part: `a || 2` is usually a defaulting
// idiom where the literal is incidental, whereas `a && b` is usually a guard
// whose literal result the caller cares about.
func UnionWithSubtypeReduction(ts ...Type) Type {
	union := NewUnionType(ts...)
	members, ok := union.(*UnionType)
	if !ok {
		return union
	}
	kept := make([]Type, 0, len(members.Types))
	for i, candidate := range members.Types {
		covered := false
		for j, other := range members.Types {
			if i == j || candidate.Equals(other) {
				continue
			}
			if IsAssignable(candidate, other) {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, candidate)
		}
	}
	if len(kept) == 0 {
		return union
	}
	return NewUnionType(kept...)
}

// ExtractNullishTypes returns the null and undefined part of a type, or Never
// if it has none. Never means `??` can never take its right branch.
func ExtractNullishTypes(t Type) Type {
	if t == nil {
		return Never
	}
	if union, ok := t.(*UnionType); ok {
		parts := make([]Type, 0, len(union.Types))
		for _, member := range union.Types {
			parts = append(parts, ExtractNullishTypes(member))
		}
		return NewUnionType(parts...)
	}
	switch t {
	case Null, Undefined, Void:
		return t
	case Any, Unknown:
		return t
	}
	return Never
}

// RemoveNullishTypes returns the part of a type that is neither null nor
// undefined — the left half of `a ?? b`. Unlike `||`, this keeps falsy values
// such as `0` and `""`, which is the whole point of the operator.
func RemoveNullishTypes(t Type) Type {
	if t == nil {
		return Never
	}
	if union, ok := t.(*UnionType); ok {
		parts := make([]Type, 0, len(union.Types))
		for _, member := range union.Types {
			parts = append(parts, RemoveNullishTypes(member))
		}
		return NewUnionType(parts...)
	}
	switch t {
	case Null, Undefined, Void:
		return Never
	}
	return t
}

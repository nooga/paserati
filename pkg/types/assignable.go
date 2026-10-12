package types

import (
	"github.com/nooga/paserati/pkg/vm"
)

// --- Type Assignability ---

// IsAssignable checks if a value of type `source` can be assigned to a variable
// of type `target`. This is moved from checker to types package for clean separation.
// simplifyMappedType converts a MappedType with a simple constraint (string/number)
// to an ObjectType with an index signature, so assignability checks work correctly.
func simplifyMappedType(t Type) Type {
	mt, ok := t.(*MappedType)
	if !ok {
		return t
	}
	if mt.ConstraintType == String || mt.ConstraintType == Number {
		obj := NewObjectType()
		valueType := mt.ValueType
		if valueType == nil {
			valueType = Any
		}
		obj.Materialize()
		obj.IndexSignatures = append(obj.IndexSigs(), &IndexSignature{
			KeyType:   mt.ConstraintType,
			ValueType: valueType,
		})
		return obj
	}
	return t
}

func (r *Relation) IsAssignable(source, target Type) bool {
	return r.isAssignable(source, target)
}

// IsAssignableForCallArgument checks assignability like IsAssignable, but
// additionally accepts a `void`-returning function argument wherever the
// parameter's declared type is itself a function type, regardless of the
// specific return type that function type declares. This matches
// TypeScript's special-casing of void-returning callbacks (see the TS
// handbook's discussion of `void`): a callback written without an explicit
// return (e.g. `(v, i) => { console.log(v, i); }`) is assignable to a
// callback parameter typed as returning `undefined` or any other type,
// because the return value is ignored by the caller (e.g.
// Array.prototype.forEach).
//
// This is intentionally narrower than IsAssignable: it only kicks in when
// both source and target are callable function types, and it is meant to be
// used specifically when checking values passed as call arguments — not for
// general variable/property assignability, where a mismatched return type
// should still be reported.
func (r *Relation) IsAssignableForCallArgument(source, target Type) bool {
	if r.isAssignable(source, target) {
		return true
	}

	// An optional callback parameter (e.g. `sort(comparefn?: (a, b) => number)`)
	// is represented as `T | undefined` at the call site. Unwrap the union and
	// try each member so the void-tolerant check still applies to the
	// function-typed member.
	if targetUnion, ok := target.(*UnionType); ok {
		for _, member := range targetUnion.Types {
			if r.IsAssignableForCallArgument(source, member) {
				return true
			}
		}
		return false
	}

	sourceObj, sourceOk := source.(*ObjectType)
	targetObj, targetOk := target.(*ObjectType)
	if !sourceOk || !targetOk || !sourceObj.IsCallable() || !targetObj.IsCallable() {
		return false
	}

	// Only the call-signature return type gets the void-tolerant treatment;
	// a target that also declares required properties must still have those
	// checked normally, so don't take the lenient path for it.
	if len(targetObj.Props()) > 0 {
		return false
	}

	sourceSigs := sourceObj.GetCallSignatures()
	targetSigs := targetObj.GetCallSignatures()
	if len(sourceSigs) == 0 || len(targetSigs) == 0 {
		return false
	}

	for _, targetSig := range targetSigs {
		for _, sourceSig := range sourceSigs {
			if r.isSignatureAssignableForCallbackArgument(sourceSig, targetSig) {
				return true
			}
		}
	}
	return false
}

// maxRelatedNesting is how deep one generic's instantiations may nest on
// each side before objectAssignable assumes they relate.
const maxRelatedNesting = 3

// assignPair keys Relation.visited.
type assignPair struct{ source, target Type }

func (r *Relation) isAssignable(source, target Type) bool {
	if source == nil || target == nil {
		return false
	}

	// Every Type is a pointer, so the pair itself is the key.
	pairKey := assignPair{source, target}
	if r.visited[pairKey] {
		return true
	}
	if r.visited == nil {
		r.visited = make(map[assignPair]bool)
	}
	r.visited[pairKey] = true
	defer delete(r.visited, pairKey)

	// Simplify mapped types (e.g., Record<string, T>) to ObjectType with index signatures
	source = simplifyMappedType(source)
	target = simplifyMappedType(target)
	source = simplifyKeyof(source)
	target = simplifyKeyof(target)

	// Handle forward references - they should be treated as equivalent to each other
	// This is a simple approach for now - in a full implementation, we'd resolve them properly
	if sourceRef, ok := source.(*TypeAliasForwardReference); ok {
		if targetRef, ok := target.(*TypeAliasForwardReference); ok {
			return sourceRef.AliasName == targetRef.AliasName
		}
		// For now, we'll be permissive with forward references in one direction
		// In a full implementation, we'd resolve the forward reference first
		return true
	}
	if _, ok := target.(*TypeAliasForwardReference); ok {
		// Target is a forward reference - be permissive for now
		return true
	}

	// Handle generic forward references
	if sourceGenRef, ok := source.(*GenericTypeAliasForwardReference); ok {
		if targetGenRef, ok := target.(*GenericTypeAliasForwardReference); ok {
			return sourceGenRef.AliasName == targetGenRef.AliasName
		}
		// For now, be permissive with generic forward references
		return true
	}
	if _, ok := target.(*GenericTypeAliasForwardReference); ok {
		// Target is a generic forward reference - be permissive for now
		return true
	}

	// Handle parameterized forward references (for recursive generic classes)
	if sourceParamRef, ok := source.(*ParameterizedForwardReferenceType); ok {
		if targetParamRef, ok := target.(*ParameterizedForwardReferenceType); ok {
			// Both are parameterized forward references - check if they're the same class with same type args
			if sourceParamRef.ClassName != targetParamRef.ClassName {
				return false
			}
			if len(sourceParamRef.TypeArguments) != len(targetParamRef.TypeArguments) {
				return false
			}
			for i := range sourceParamRef.TypeArguments {
				if !r.isAssignable(sourceParamRef.TypeArguments[i], targetParamRef.TypeArguments[i]) {
					return false
				}
			}
			return true
		}
		// For now, be permissive when source is parameterized forward reference
		return true
	}
	if _, ok := target.(*ParameterizedForwardReferenceType); ok {
		// Target is a parameterized forward reference - be permissive for now
		// This allows object types to be assigned to forward references
		return true
	}

	// Basic rules:
	if target == Any || source == Any {
		return true
	}

	if target == Unknown {
		return true
	}
	if source == Unknown {
		return target == Unknown
	}

	if source == Never {
		return true
	}

	// Type predicate results are boolean values at runtime, but carry extra
	// metadata in function return types for control-flow narrowing.
	if _, ok := source.(*TypePredicateType); ok && target == Boolean {
		return true
	}
	if sourcePred, ok := source.(*TypePredicateType); ok {
		if targetPred, ok := target.(*TypePredicateType); ok {
			return r.isAssignable(sourcePred.Type, targetPred.Type)
		}
	}

	// TypeScript compatibility: undefined is assignable to void (and null too
	// when strictNullChecks is off)
	if target == Void && (source == Undefined || (source == Null && !r.StrictNullChecks)) {
		return true
	}

	// strictNullChecks: false (TypeScript default) — null and undefined are assignable
	// to any non-never, non-void type.
	if !r.StrictNullChecks && (source == Null || source == Undefined) && target != Never && target != Void {
		return true
	}

	// Check for identical types before type-specific handling.
	if source == target {
		return true
	}

	// Template literal types (#616): a string literal is assignable when it
	// matches the pattern.
	if tlt, ok := target.(*TemplateLiteralType); ok {
		if result, decided := r.assignableToTemplateLiteral(source, tlt); decided {
			return result
		}
	}
	if _, ok := source.(*TemplateLiteralType); ok && target == String {
		return true // every template literal type is a string
	}

	// NonPrimitive (the `object` keyword) only accepts non-primitive types (ObjectType,
	// arrays, functions). Primitive types are NOT assignable to `object`.
	if target == NonPrimitive {
		switch source {
		case Number, String, Boolean, Symbol, BigInt, Null, Undefined, Void, Never:
			return false
		}
		if _, ok := source.(*Primitive); ok {
			return false // Any other primitive-like singleton
		}
		return true // ObjectTypes, arrays, functions, etc.
	}

	// `object` has no members, so it is assignable to an object type that
	// requires none (`{ t?: string }`, `{}`).
	if source == NonPrimitive {
		if tgt, ok := target.(*ObjectType); ok && !tgt.IsCallable() && len(tgt.ConstructSigs()) == 0 && len(tgt.IndexSigs()) == 0 {
			required := false
			for name := range tgt.GetEffectiveProperties() {
				if !tgt.IsPropertyOptional(name) {
					required = true
					break
				}
			}
			if !required {
				return true
			}
		}
	}

	// A primitive, array or tuple has the members of its wrapper type, so it
	// satisfies an object type that asks only for those (`string` is a
	// `{ length: number }`).
	if tgt, ok := target.(*ObjectType); ok && r.apparentMembersAssignable(source, tgt) {
		return true
	}

	// Check using type-specific Equals method for complex types
	if source.Equals(target) {
		return true
	}

	// Enum members and their base primitives (isSimpleTypeRelatedTo): a
	// numeric/string enum member is assignable to number/string, and number
	// (or a number literal equal to a member value) is assignable to a
	// numeric enum.
	if enumAssignable(source, target) {
		return true
	}

	// Handle InstantiatedType - substitute to get concrete type and check assignability
	if sourceInst, ok := source.(*InstantiatedType); ok {
		// Substitute the generic type with concrete type arguments
		concreteSource := sourceInst.Substitute()
		return r.isAssignable(concreteSource, target)
	}
	if targetInst, ok := target.(*InstantiatedType); ok {
		// Substitute the generic type with concrete type arguments
		concreteTarget := targetInst.Substitute()
		return r.isAssignable(source, concreteTarget)
	}

	// GenericType handling - for generic methods in interfaces
	// When target is a GenericType (generic method signature) and source is an ObjectType (method implementation)
	if targetGeneric, ok := target.(*GenericType); ok {
		// A constructable source against a generic construct signature.
		if sourceObj, ok := source.(*ObjectType); ok && len(sourceObj.ConstructSigs()) > 0 {
			if bodyObj, ok := targetGeneric.Body.(*ObjectType); ok && len(bodyObj.ConstructSigs()) > 0 {
				if len(sourceObj.ConstructSigs()[0].ParameterTypes) == len(bodyObj.ConstructSigs()[0].ParameterTypes) {
					return true
				}
			}
		}
		// Check if source is a callable ObjectType
		if sourceObj, ok := source.(*ObjectType); ok && sourceObj.IsCallable() {
			// Get the body of the generic type (the function signature)
			if bodyObj, ok := targetGeneric.Body.(*ObjectType); ok && bodyObj.IsCallable() {
				// Compare call signatures structurally, ignoring type parameters
				sourceSigs := sourceObj.GetCallSignatures()
				bodySigs := bodyObj.GetCallSignatures()
				if len(sourceSigs) > 0 && len(bodySigs) > 0 {
					// Check if parameter counts match (simplified check)
					if len(sourceSigs[0].ParameterTypes) == len(bodySigs[0].ParameterTypes) {
						return true
					}
				}
			}
		}
		// Also handle GenericType to GenericType comparison
		if sourceGeneric, ok := source.(*GenericType); ok {
			// Compare type parameter counts and body types
			if len(sourceGeneric.TypeParameters) == len(targetGeneric.TypeParameters) &&
				r.isAssignable(sourceGeneric.Body, targetGeneric.Body) {
				return true
			}
			return r.isAssignable(eraseGenericType(sourceGeneric), targetGeneric.Body)
		}
	}

	if sourceGeneric, ok := source.(*GenericType); ok {
		if targetObj, ok := target.(*ObjectType); ok {
			if targetObj.IsCallable() {
				return r.isAssignable(sourceGeneric.Body, targetObj) || r.isAssignable(eraseGenericType(sourceGeneric), targetObj)
			}
			return r.isAssignable(eraseGenericType(sourceGeneric), targetObj)
		}
	}

	// Union type handling
	sourceUnion, sourceIsUnion := source.(*UnionType)
	targetUnion, targetIsUnion := target.(*UnionType)

	if targetIsUnion {
		if sourceIsUnion {
			// Union to union: every type in source must be assignable to at least one in target
			for _, sType := range sourceUnion.Types {
				assignable := false
				for _, tType := range targetUnion.Types {
					if r.isAssignable(sType, tType) {
						assignable = true
						break
					}
				}
				if !assignable {
					return false
				}
			}
			return true
		} else {
			// Non-union to union: source must be assignable to at least one type in target
			for _, tType := range targetUnion.Types {
				if r.isAssignable(source, tType) {
					return true
				}
			}
			return false
		}
	} else if sourceIsUnion {
		// Union to non-union: every type in source must be assignable to target
		for _, sType := range sourceUnion.Types {
			if !r.isAssignable(sType, target) {
				return false
			}
		}
		return true
	}

	// Intersection type handling
	sourceIntersection, sourceIsIntersection := source.(*IntersectionType)
	targetIntersection, targetIsIntersection := target.(*IntersectionType)

	if targetIsIntersection {
		// Source must be assignable to ALL types in target intersection
		for _, tType := range targetIntersection.Types {
			if srcObj, ok := source.(*ObjectType); ok {
				if tgtObj, ok := tType.(*ObjectType); ok {
					if !r.objectAssignable(srcObj, tgtObj, false) {
						return false
					}
					continue
				}
			}
			if !r.isAssignable(source, tType) {
				return false
			}
		}
		return true
	} else if sourceIsIntersection {
		// At least one type in source intersection must be assignable to target
		for _, sType := range sourceIntersection.Types {
			if r.isAssignable(sType, target) {
				return true
			}
		}
		// Otherwise the members' combined structure may satisfy an object
		// target ({a} & {b} is assignable to {a; b}).
		if tgtObj, ok := target.(*ObjectType); ok {
			if merged := mergeIntersectionObjects(sourceIntersection); merged != nil {
				return r.objectAssignable(merged, tgtObj, true)
			}
		}
		return false
	}

	// Literal type handling
	sourceLiteral, sourceIsLiteral := source.(*LiteralType)
	targetLiteral, targetIsLiteral := target.(*LiteralType)

	if sourceIsLiteral && targetIsLiteral {
		// Both literals: values must be equal
		if sourceLiteral.Value.Type() != targetLiteral.Value.Type() {
			return false
		}
		switch sourceLiteral.Value.Type() {
		case vm.TypeNull, vm.TypeUndefined:
			return true
		case vm.TypeBoolean:
			return sourceLiteral.Value.AsBoolean() == targetLiteral.Value.AsBoolean()
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return vm.AsNumber(sourceLiteral.Value) == vm.AsNumber(targetLiteral.Value)
		case vm.TypeString:
			return vm.AsString(sourceLiteral.Value) == vm.AsString(targetLiteral.Value)
		default:
			return false
		}
	} else if sourceIsLiteral {
		// Literal to non-literal: check if literal's primitive type is assignable
		if len(numericEnumMembers(target)) > 0 {
			return false // numeric literals relate to enums only by value (enumAssignable)
		}
		var primitiveType Type
		switch sourceLiteral.Value.Type() {
		case vm.TypeString:
			primitiveType = String
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			primitiveType = Number
		case vm.TypeBoolean:
			primitiveType = Boolean
		case vm.TypeBigInt:
			primitiveType = BigInt

		default:
			return false
		}
		return r.isAssignable(primitiveType, target)
	} else if targetIsLiteral {
		// Non-literal to literal: generally false except for special cases
		return false
	}

	// Array type handling
	sourceArray, sourceIsArray := source.(*ArrayType)
	targetArray, targetIsArray := target.(*ArrayType)

	if sourceIsArray && targetIsArray {
		if sourceArray.ElementType == nil || targetArray.ElementType == nil {
			return false
		}
		return r.isAssignable(sourceArray.ElementType, targetArray.ElementType)
	}

	// Tuple type handling
	sourceTuple, sourceIsTuple := source.(*TupleType)
	targetTuple, targetIsTuple := target.(*TupleType)

	// Handle tuple to array assignability: [string, number] should be assignable to any[]
	if sourceIsTuple && targetIsArray {
		if targetArray.ElementType == nil {
			return false
		}
		// All tuple elements must be assignable to the array element type
		for _, tupleElementType := range sourceTuple.ElementTypes {
			if !r.isAssignable(tupleElementType, targetArray.ElementType) {
				return false
			}
		}
		return true
	}

	if sourceIsTuple && targetIsTuple {
		sourceLen := len(sourceTuple.ElementTypes)
		targetLen := len(targetTuple.ElementTypes)

		// Check each target element against source
		for i := 0; i < targetLen; i++ {
			targetElementType := targetTuple.ElementTypes[i]
			targetIsOptional := i < len(targetTuple.OptionalElements) && targetTuple.OptionalElements[i]

			if i < sourceLen {
				sourceElementType := sourceTuple.ElementTypes[i]
				if !r.isAssignable(sourceElementType, targetElementType) {
					return false
				}
			} else if !targetIsOptional {
				// Target element is required but source doesn't have it
				return false
			}
		}

		// Check if source has extra elements that target can't handle
		if sourceLen > targetLen && targetTuple.RestElementType == nil {
			return false
		}

		return true
	}

	// Object type handling
	sourceObj, sourceIsObj := source.(*ObjectType)
	targetObj, targetIsObj := target.(*ObjectType)

	// An empty object type {} (no properties, no index sigs, no call/construct sigs) is
	// essentially the TypeScript `{}` / `Object` type — any non-null, non-undefined,
	// non-never value is assignable to it. This matches strictNullChecks:false semantics.
	if targetIsObj &&
		len(targetObj.Props()) == 0 &&
		len(targetObj.IndexSigs()) == 0 &&
		len(targetObj.CallSigs()) == 0 &&
		len(targetObj.ConstructSigs()) == 0 {
		// Only exclude null/undefined/never/void (already handled above).
		if source != Never && source != Void && (!r.StrictNullChecks || (source != Null && source != Undefined)) {
			return true
		}
	}

	if sourceIsObj && targetIsObj {
		return r.objectAssignable(sourceObj, targetObj, true)
	}

	// Readonly type handling
	sourceReadonly, sourceIsReadonly := source.(*ReadonlyType)
	targetReadonly, targetIsReadonly := target.(*ReadonlyType)

	if sourceIsReadonly && targetIsReadonly {
		// readonly T to readonly U: T must be assignable to U
		return r.isAssignable(sourceReadonly.InnerType, targetReadonly.InnerType)
	} else if sourceIsReadonly && !targetIsReadonly {
		// A readonly array or tuple never fits a mutable one (TS4104);
		// otherwise readonly T to T is allowed (covariance).
		if IsReadonlyArrayLike(source) && isMutableArrayLike(target) {
			return false
		}
		return r.isAssignable(sourceReadonly.InnerType, target)
	} else if !sourceIsReadonly && targetIsReadonly {
		// T to readonly T: allowed (source is assignable to target inner type)
		// This is safe because we're making something more restrictive
		if r.isAssignable(source, targetReadonly.InnerType) {
			return true
		}
		// A type parameter whose constraint is itself readonly relates
		// through that constraint, readonly intact.
		if tp, ok := source.(*TypeParameterType); ok && tp.Parameter != nil && tp.Parameter.Constraint != nil {
			return r.isAssignable(tp.Parameter.Constraint, target)
		}
		return false
	}

	// TypeParameterType handling - type parameters with the same identity are assignable
	sourceTypeParam, sourceIsTypeParam := source.(*TypeParameterType)
	targetTypeParam, targetIsTypeParam := target.(*TypeParameterType)

	if sourceIsTypeParam && targetIsTypeParam {
		// Type parameters are assignable if they refer to the same type parameter
		// Since we might have different instances of the same logical type parameter,
		// compare by name as a fallback (this is a simplification - in a full implementation
		// we'd track scoping more carefully)
		if sourceTypeParam.Parameter == targetTypeParam.Parameter {
			return true
		}

		// Fallback: compare by name if they're different instances
		sourceName := sourceTypeParam.Parameter.Name
		targetName := targetTypeParam.Parameter.Name
		if sourceName == targetName {
			// fmt.Printf("// [TypeParam Debug] Allowing name-based match: '%s'\n", sourceName)
			return true
		}

		if c := typeParameterConstraint(sourceTypeParam); c != nil {
			return r.isAssignable(c, target)
		}

		return false
	}

	// Handle case where source is a type parameter and target is a concrete type
	if sourceIsTypeParam && !targetIsTypeParam {
		// Check if the source type parameter's constraint is assignable to the target
		// This handles cases like: U extends Date should be assignable to Date
		if c := typeParameterConstraint(sourceTypeParam); c != nil {
			return r.isAssignable(c, target)
		}
		// If no constraint, fall back to checking if the type parameter itself can be assigned
		// (this would typically be false for concrete types)
		return false
	}

	// Note: Readonly<T> utility type is now handled via mapped type expansion
	// The expandMappedType system will convert Readonly<T> to concrete object types
	// so no special handling is needed here

	// Legacy FunctionType compatibility removed - use ObjectType with CallSignatures instead

	return false
}

func (r *Relation) signaturesRelated(sourceSigs, targetSigs []*Signature) bool {
	for _, targetSig := range targetSigs {
		matched := false
		for _, sourceSig := range sourceSigs {
			if r.isSignatureAssignable(sourceSig, targetSig) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// Helper function to check signature assignability
func (r *Relation) isSignatureAssignable(source, target *Signature) bool {
	return r.isSignatureAssignableImpl(source, target, false)
}

// isSignatureAssignableForCallbackArgument is like isSignatureAssignable but
// additionally tolerates a `void` actual return type on the source signature,
// matching TypeScript's special-casing of void-returning callbacks: "a
// void-returning function can have any other return type in its actual
// implementation... calling it without using the return value is common."
// (see the TS handbook section on the void type). This is only used when
// checking a function literal/value used as a call argument (e.g. the
// callback passed to Array.prototype.forEach), not for general function-type
// assignability, so it doesn't loosen plain variable/property assignments.
func (r *Relation) isSignatureAssignableForCallbackArgument(source, target *Signature) bool {
	return r.isSignatureAssignableImpl(source, target, true)
}

// sigMinArgs is TypeScript's getMinArgumentCount for a Signature: the index
// just past the last required fixed parameter.
func sigMinArgs(sig *Signature) int {
	n := len(sig.ParameterTypes)
	for n > 0 && n-1 < len(sig.OptionalParams) && sig.OptionalParams[n-1] {
		n--
	}
	// Trailing parameters that accept void may be omitted by callers.
	for n > 0 && acceptsVoid(sig.ParameterTypes[n-1]) {
		n--
	}
	return n
}

// sigRestElement returns the element type of a signature's rest parameter.
func sigRestElement(sig *Signature) Type {
	if sig.RestParameterType == nil {
		return nil
	}
	switch r := sig.RestParameterType.(type) {
	case *ArrayType:
		return r.ElementType
	case *ReadonlyType:
		if a, ok := r.InnerType.(*ArrayType); ok {
			return a.ElementType
		}
	}
	return Any
}

// sigTypeAtPosition mirrors tryGetTypeAtPosition: the type of the i-th
// argument position, falling back to the rest element type.
func (r *Relation) sigTypeAtPosition(sig *Signature, i int) Type {
	if i < len(sig.ParameterTypes) {
		pt := sig.ParameterTypes[i]
		// Under strictNullChecks an optional parameter also accepts undefined.
		if r.StrictNullChecks && i < len(sig.OptionalParams) && sig.OptionalParams[i] && pt != nil {
			return NewUnionType(pt, Undefined)
		}
		return pt
	}
	return sigRestElement(sig)
}

// eraseSignatureTypeParams replaces a signature's own type parameters with
// any (a permissive stand-in for TypeScript's instantiateSignatureInContextOf).
func EraseSignatureTypeParams(sig *Signature) *Signature {
	return eraseSignatureTypeParams(sig)
}

func eraseSignatureTypeParams(sig *Signature) *Signature {
	if sig == nil || len(sig.TypeParameters) == 0 {
		return sig
	}
	subs := make(map[*TypeParameter]Type, len(sig.TypeParameters))
	for _, tp := range sig.TypeParameters {
		subs[tp] = Any
	}
	out := substituteSignature(sig, subs)
	out.ParameterNames = sig.ParameterNames
	return out
}

// isSignatureAssignableImpl follows compareSignaturesRelated: the source may
// not require more arguments than the target can supply, parameters are
// compared pairwise (bivariantly), and the return type is covariant unless
// the target returns void/any.
func (r *Relation) isSignatureAssignableImpl(source, target *Signature, tolerateVoidSourceReturn bool) bool {
	return r.signatureRelated(source, target, tolerateVoidSourceReturn, false)
}

// signatureRelated is the signature relation; callbackParams relaxes strict
// variance for signatures compared as callback parameters.
func (r *Relation) signatureRelated(source, target *Signature, tolerateVoidSourceReturn bool, callbackParams bool) bool {
	if source == nil || target == nil {
		return source == target
	}
	if source == target {
		return true
	}

	source = eraseSignatureTypeParams(source)
	strict := r.StrictFunctionTypes && target.StrictVariance && !callbackParams

	targetHasRest := target.RestParameterType != nil
	if !targetHasRest && sigMinArgs(source) > len(target.ParameterTypes) {
		return false
	}

	n := len(source.ParameterTypes)
	if len(target.ParameterTypes) > n {
		n = len(target.ParameterTypes)
	}
	for i := 0; i < n; i++ {
		sp := r.sigTypeAtPosition(source, i)
		tp := r.sigTypeAtPosition(target, i)
		if sp == nil || tp == nil {
			continue
		}
		if !r.paramRelated(sp, tp, strict) {
			return false
		}
	}
	if source.RestParameterType != nil && targetHasRest {
		sp, tp := sigRestElement(source), sigRestElement(target)
		if sp != nil && tp != nil && !r.paramRelated(sp, tp, strict) {
			return false
		}
	}

	// A void/any return type on the target accepts any source return.
	if target.ReturnType == Void || target.ReturnType == Any {
		return true
	}
	// A void-returning callback argument is assignable regardless of what
	// specific return type the parameter's function type declares, since the
	// caller (e.g. forEach) ignores the return value.
	if tolerateVoidSourceReturn && source.ReturnType == Void {
		return true
	}

	return r.isAssignable(source.ReturnType, target.ReturnType)
}

// Helper function removed - FunctionType deprecated, use ObjectType with CallSignatures

// acceptsVoid reports whether a parameter type admits `void` (void itself or a
// union containing it), which makes a trailing parameter omittable.
func acceptsVoid(t Type) bool {
	if t == Void {
		return true
	}
	if u, ok := t.(*UnionType); ok {
		for _, m := range u.Types {
			if m == Void {
				return true
			}
		}
	}
	return false
}

// IsWeakObject reports whether an object type is "weak": it declares at least
// one property, every declared property is optional, and it has no call,
// construct or index signatures (TypeScript's isWeakType).
func IsWeakObject(t *ObjectType) bool {
	if t == nil || len(t.CallSigs()) > 0 || len(t.ConstructSigs()) > 0 || len(t.IndexSigs()) > 0 {
		return false
	}
	props := t.GetEffectiveProperties()
	if len(props) == 0 {
		return false
	}
	for name := range props {
		if !t.IsPropertyOptional(name) {
			return false
		}
	}
	return true
}

// ObjectHasMembers reports whether an object type has any property or
// call/construct signature (the source condition of the weak type check).
func ObjectHasMembers(t *ObjectType) bool {
	return len(t.GetEffectiveProperties()) > 0 || len(t.CallSigs()) > 0 || len(t.ConstructSigs()) > 0
}

// ObjectsShareProperty reports whether two object types have a property name
// in common (TypeScript's hasCommonProperties).
func ObjectsShareProperty(a, b *ObjectType) bool {
	bProps := b.GetEffectiveProperties()
	for name := range a.GetEffectiveProperties() {
		if _, ok := bProps[name]; ok {
			return true
		}
	}
	return false
}

// objectAssignable relates two object types structurally. checkWeak enables
// the weak-type (no common properties) rule, which TypeScript skips when the
// target is a constituent of an intersection.
func (r *Relation) objectAssignable(sourceObj, targetObj *ObjectType, checkWeak bool) bool {
	if sameGenericWithLooserArgs(sourceObj, targetObj, 0) {
		return true
	}
	// Copies of one non-generic class's instance type (made by substitution
	// or rebinding, sharing its metadata) are the same nominal type, however
	// differently their members have been settled since.
	if sm := sourceObj.ClassMeta; sm != nil && sm == targetObj.ClassMeta && sm.IsClassInstance &&
		len(sourceObj.GenericArgs) == 0 && len(targetObj.GenericArgs) == 0 {
		return true
	}
	// Relating Grow<A> to Grow<B> relates their next(): Grow<Wrap<A>> to
	// Grow<Wrap<B>>, and so on without end. Like tsc's isDeeplyNestedType,
	// once instantiations of one generic are being related several levels
	// deep on both sides, assume the rest relates.
	sg, tg := sourceObj.Instantiates, targetObj.Instantiates
	if sg != nil && tg != nil {
		if r.relatingInstances[sg] >= maxRelatedNesting && r.relatingInstances[tg] >= maxRelatedNesting {
			return true
		}
		if r.relatingInstances == nil {
			r.relatingInstances = make(map[*GenericType]int)
		}
		r.relatingInstances[sg]++
		r.relatingInstances[tg]++
		defer func() {
			r.relatingInstances[sg]--
			r.relatingInstances[tg]--
		}()
	}
	// Check that all required properties in target exist in source and are assignable
	targetProps := targetObj.GetEffectiveProperties()
	sourceProps := sourceObj.GetEffectiveProperties()

	// Weak type detection: a target whose properties are all optional
	// demands that the source share at least one of them.
	if checkWeak && IsWeakObject(targetObj) && ObjectHasMembers(sourceObj) && !ObjectsShareProperty(sourceObj, targetObj) {
		return false
	}

	for propName, targetPropType := range targetProps {
		targetOptional := targetObj.IsPropertyOptional(propName)
		// Private and protected members are nominal: the source must derive
		// from the class that declares them.
		if declaring, ok := nonPublicMemberOwner(targetObj, propName, 0); ok {
			if !classDerivesFrom(sourceObj, declaring, 0) {
				return false
			}
		}
		sourcePropType, exists := sourceProps[propName]
		if !exists {
			if !targetOptional {
				return false
			}
			continue
		}
		if r.StrictNullChecks {
			if sourceObj.IsPropertyOptional(propName) && !targetOptional {
				return false
			}
			if targetOptional {
				targetPropType = NewUnionType(targetPropType, Undefined)
			}
		}
		if !r.isAssignable(sourcePropType, targetPropType) {
			return false
		}
	}

	if !r.indexSignaturesRelated(sourceObj, targetObj, sourceProps) {
		return false
	}

	// Call and construct signatures: every target signature must be
	// matched by some source signature (signaturesRelatedTo).
	if !r.signaturesRelated(sourceObj.CallSigs(), targetObj.CallSigs()) ||
		!r.signaturesRelated(sourceObj.ConstructSigs(), targetObj.ConstructSigs()) {
		return false
	}

	return true
}

// paramRelated compares one pair of parameter types. Under strict variance the
// target parameter must be assignable to the source parameter; otherwise
// either direction suffices. Parameters that are themselves callbacks are
// related with their own parameters compared bivariantly.
func (r *Relation) paramRelated(sp, tp Type, strict bool) bool {
	if !strict {
		return r.isAssignable(tp, sp) || r.isAssignable(sp, tp)
	}
	if sObj, ok := sp.(*ObjectType); ok && len(sObj.CallSigs()) == 1 && len(sObj.Props()) == 0 {
		if tObj, ok := tp.(*ObjectType); ok && len(tObj.CallSigs()) == 1 && len(tObj.Props()) == 0 {
			return r.signatureRelated(tObj.CallSigs()[0], sObj.CallSigs()[0], false, true)
		}
	}
	return r.isAssignable(tp, sp)
}

// enumAssignable implements the enum <-> primitive relations. It returns true
// only when the relation holds; callers fall through to other rules otherwise.
func enumAssignable(source, target Type) bool {
	if em, ok := source.(*EnumMemberType); ok {
		switch em.Value.(type) {
		case string:
			if target == String {
				return true
			}
		default:
			if target == Number {
				return true
			}
		}
	}
	members := numericEnumMembers(target)
	if len(members) == 0 {
		return false
	}
	if source == Number {
		return true
	}
	if lit, ok := source.(*LiteralType); ok {
		switch lit.Value.Type() {
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			n := vm.AsNumber(lit.Value)
			for _, m := range members {
				if v, ok := m.Value.(int); ok && float64(v) == n {
					return true
				}
			}
		}
	}
	return false
}

// numericEnumMembers returns the members of a numeric enum member type or a
// union consisting solely of numeric enum members.
func numericEnumMembers(t Type) []*EnumMemberType {
	switch tt := t.(type) {
	case *EnumMemberType:
		if _, ok := tt.Value.(int); ok {
			return []*EnumMemberType{tt}
		}
	case *UnionType:
		var out []*EnumMemberType
		for _, m := range tt.Types {
			em, ok := m.(*EnumMemberType)
			if !ok {
				return nil
			}
			if _, ok := em.Value.(int); !ok {
				return nil
			}
			out = append(out, em)
		}
		return out
	}
	return nil
}

// eraseGenericType replaces a generic type's own parameters with any in its
// body (a permissive stand-in for instantiating it in the context of the type
// it is related to).
func eraseGenericType(g *GenericType) Type {
	subs := make(map[*TypeParameter]Type, len(g.TypeParameters))
	for _, tp := range g.TypeParameters {
		subs[tp] = Any
	}
	return substituteType(g.Body, subs)
}

// findIndexSignature returns the signature of obj (or its base types) that
// applies to keys of keyType: a string signature covers numeric keys too.
func findIndexSignature(obj *ObjectType, keyType Type) *IndexSignature {
	var stringSig, anySig *IndexSignature
	var visit func(o *ObjectType, depth int) *IndexSignature
	visit = func(o *ObjectType, depth int) *IndexSignature {
		if depth > 8 {
			return nil
		}
		for _, sig := range o.IndexSigs() {
			if sig == nil || sig.IsMapped || sig.Synthetic {
				continue
			}
			switch {
			case sig.KeyType == keyType:
				return sig
			case sig.KeyType == String && stringSig == nil:
				stringSig = sig
			case sig.KeyType == Any && anySig == nil:
				anySig = sig
			}
		}
		for _, base := range o.Bases() {
			if bo, ok := base.(*ObjectType); ok {
				if sig := visit(bo, depth+1); sig != nil {
					return sig
				}
			}
		}
		return nil
	}
	if sig := visit(obj, 0); sig != nil {
		return sig
	}
	if keyType == Number || keyType == String {
		if stringSig != nil {
			return stringSig
		}
	}
	return anySig
}

// indexSignaturesRelated is TypeScript's indexSignaturesRelatedTo: every index
// signature of the target needs a related signature in the source or, when
// the source has none, an implicit one made from its properties (only for
// object literal and type literal types, never interfaces or classes).
func (r *Relation) indexSignaturesRelated(sourceObj, targetObj *ObjectType, sourceProps map[string]Type) bool {
	for _, tIdx := range targetObj.IndexSigs() {
		if tIdx == nil || tIdx.IsMapped || tIdx.Synthetic || tIdx.ValueType == nil {
			continue
		}
		if tIdx.KeyType != String && tIdx.KeyType != Number && tIdx.KeyType != Any {
			continue
		}
		if sIdx := findIndexSignature(sourceObj, tIdx.KeyType); sIdx != nil {
			if sIdx.ValueType == nil || !r.isAssignable(sIdx.ValueType, tIdx.ValueType) {
				return false
			}
			continue
		}
		if sourceObj.IsInterface || sourceObj.ClassMeta != nil ||
			len(sourceObj.CallSigs()) > 0 || len(sourceObj.ConstructSigs()) > 0 {
			// No implicit index signature: tolerate only when nothing is declared
			// that could violate it (an empty source relates vacuously).
			if len(sourceProps) == 0 {
				continue
			}
			return false
		}
		for name, pt := range sourceProps {
			if tIdx.KeyType == Number && !isNumericName(name) {
				continue
			}
			if r.StrictNullChecks && sourceObj.IsPropertyOptional(name) {
				pt = NewUnionType(pt, Undefined)
			}
			if !r.isAssignable(pt, tIdx.ValueType) {
				return false
			}
		}
	}
	return true
}

func isNumericName(name string) bool {
	if name == "" {
		return false
	}
	for _, ch := range name {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// typeParameterConstraint returns the meaningful constraint of a type
// parameter. An unconstrained parameter (the checker records `any`) or one
// constrained to `unknown` has no constraint that could make it assignable to
// anything other than any/unknown.
func typeParameterConstraint(tp *TypeParameterType) Type {
	c := tp.Parameter.Constraint
	if c == nil || c == Any || c == Unknown {
		return nil
	}
	return c
}

// mergeIntersectionObjects flattens an intersection of object types into one
// object type that has all of their members, or returns nil when a member is
// not an object type.
func mergeIntersectionObjects(inter *IntersectionType) *ObjectType {
	merged := &ObjectType{
		Properties:         make(map[string]Type),
		OptionalProperties: make(map[string]bool),
	}
	for _, m := range inter.Types {
		obj, ok := m.(*ObjectType)
		if !ok {
			return nil
		}
		effective := obj.GetEffectiveProperties()
		for _, name := range obj.EffectivePropertyNames() {
			pt := effective[name]
			if prev, exists := merged.Props()[name]; exists {
				merged.SetProperty(name, NewIntersectionType(prev, pt))
				if !obj.IsPropertyOptional(name) {
					merged.OptionalProps()[name] = false
				}
				continue
			}
			merged.SetProperty(name, pt)
			merged.OptionalProps()[name] = obj.IsPropertyOptional(name)
		}
		merged.Materialize()
		merged.CallSignatures = append(merged.CallSigs(), obj.CallSigs()...)
		merged.Materialize()
		merged.ConstructSignatures = append(merged.ConstructSigs(), obj.ConstructSigs()...)
		merged.Materialize()
		merged.IndexSignatures = append(merged.IndexSigs(), obj.IndexSigs()...)
	}
	return merged
}

// nonPublicMemberOwner returns the name of the class that declares a private
// or protected instance member of obj (looking through base types).
func nonPublicMemberOwner(obj *ObjectType, name string, depth int) (string, bool) {
	if obj == nil || depth > 8 {
		return "", false
	}
	if obj.ClassMeta != nil {
		if info := obj.ClassMeta.GetMemberAccess(name); info != nil && !info.IsStatic {
			if info.AccessLevel != AccessPublic {
				return obj.ClassMeta.ClassName, true
			}
			return "", false
		}
	}
	for _, base := range obj.Bases() {
		if bo, ok := resolveBaseType(base).(*ObjectType); ok {
			if owner, found := nonPublicMemberOwner(bo, name, depth+1); found {
				return owner, true
			}
		}
	}
	return "", false
}

// classDerivesFrom reports whether obj is an instance of the named class or of
// a class derived from it.
func classDerivesFrom(obj *ObjectType, className string, depth int) bool {
	if obj == nil || depth > 8 {
		return false
	}
	if obj.ClassMeta != nil && obj.ClassMeta.ClassName == className {
		return true
	}
	for _, base := range obj.Bases() {
		if bo, ok := resolveBaseType(base).(*ObjectType); ok && classDerivesFrom(bo, className, depth+1) {
			return true
		}
	}
	return false
}

// IsReadonlyArrayLike reports whether t is `readonly T[]` or a readonly tuple.
func IsReadonlyArrayLike(t Type) bool {
	ro, ok := t.(*ReadonlyType)
	if !ok {
		return false
	}
	switch ro.InnerType.(type) {
	case *ArrayType, *TupleType:
		return true
	}
	return false
}

// isMutableArrayLike reports whether t is a mutable array or tuple type.
func isMutableArrayLike(t Type) bool {
	switch t.(type) {
	case *ArrayType, *TupleType:
		return true
	}
	return false
}

// apparentMembersAssignable reports whether source's apparent type (the wrapper
// prototype of a primitive, array or tuple) satisfies the plain object type tgt.
func (r *Relation) apparentMembersAssignable(source Type, tgt *ObjectType) bool {
	if r.PrimitivePrototypeMembers == nil || tgt.IsCallable() || len(tgt.ConstructSigs()) > 0 || len(tgt.IndexSigs()) > 0 {
		return false
	}
	kind := ""
	switch src := source.(type) {
	case *ArrayType, *TupleType:
		kind = "array"
	case *LiteralType:
		switch src.Value.Type() {
		case vm.TypeString:
			kind = "string"
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			kind = "number"
		case vm.TypeBoolean:
			kind = "boolean"
		}
	default:
		switch source {
		case String:
			kind = "string"
		case Number:
			kind = "number"
		case Boolean:
			kind = "boolean"
		case BigInt:
			kind = "bigint"
		}
	}
	if kind == "" {
		return false
	}
	members := r.PrimitivePrototypeMembers(kind)
	if members == nil {
		return false
	}
	props := tgt.GetEffectiveProperties()
	shared := false
	for _, name := range SortedPropertyNames(props) {
		want := props[name]
		have, ok := members[name]
		if name == "length" && (kind == "string" || kind == "array") {
			have, ok = Number, true
		}
		if ok {
			shared = true
		}
		if tgt.IsPropertyOptional(name) {
			continue
		}
		if !ok {
			return false
		}
		if _, generic := have.(*GenericType); generic || want == nil {
			continue
		}
		if !r.isAssignable(have, want) {
			return false
		}
	}
	// A weak target (all optional) needs at least one member in common.
	if IsWeakObject(tgt) && !shared {
		return false
	}
	return true
}

// sameGenericWithLooserArgs reports whether source is assignable to target
// without a structural walk because both instantiate the same generic:
// source itself with every target argument any, unknown or identical, or a
// base class of source with every target argument any or unknown (a derived
// class may override members incompatibly, which only the structural check
// sees). ZodString to ZodTypeAny = ZodType<any, any, any> is the case this
// is for: walking zod's members met mapped types this relation cannot
// expand. Anything else is left to the structural check.
func sameGenericWithLooserArgs(source, target *ObjectType, depth int) bool {
	g := target.Instantiates
	if g == nil || depth > 16 || source == nil {
		return false
	}
	if source.Instantiates == g && len(source.GenericArgs) == len(target.GenericArgs) {
		for i, ta := range target.GenericArgs {
			if ta == Any || ta == Unknown {
				continue
			}
			if depth > 0 || (ta != source.GenericArgs[i] && !ta.Equals(source.GenericArgs[i])) {
				return false
			}
		}
		return true
	}
	for _, base := range source.Bases() {
		switch b := base.(type) {
		case *ObjectType:
			if sameGenericWithLooserArgs(b, target, depth+1) {
				return true
			}
		case *InstantiatedType:
			if b.Generic == g && len(b.TypeArguments) == len(target.GenericArgs) && allAnyOrUnknown(target.GenericArgs) {
				return true
			}
			if bo, isObj := b.Substitute().(*ObjectType); isObj && sameGenericWithLooserArgs(bo, target, depth+1) {
				return true
			}
		}
	}
	return false
}

func allAnyOrUnknown(ts []Type) bool {
	for _, t := range ts {
		if t != Any && t != Unknown {
			return false
		}
	}
	return true
}

// simplifyKeyof expands `keyof O` for a concrete object type O (a
// `keyof typeof C` written before C was declared, resolved since) to the
// union of O's property names.
func simplifyKeyof(t Type) Type {
	kt, ok := t.(*KeyofType)
	if !ok {
		return t
	}
	obj, ok := kt.OperandType.(*ObjectType)
	if !ok || len(obj.IndexSigs()) > 0 {
		return t
	}
	names := obj.PropertyNames()
	if len(names) == 0 {
		return Never
	}
	keys := make([]Type, len(names))
	for i, name := range names {
		keys[i] = &LiteralType{Value: vm.String(name)}
	}
	return NewUnionType(keys...)
}

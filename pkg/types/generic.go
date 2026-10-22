package types

import (
	"fmt"
	"strings"
)

// TypeParameter represents a generic type parameter (e.g., T in Array<T> or function<T>)
type TypeParameter struct {
	Name       string // The parameter name (e.g., "T", "U", "K", "V")
	Constraint Type   // Optional constraint (e.g., T extends string), nil if unconstrained
	Default    Type   // Optional default type (e.g., T = string), nil if no default
	Index      int    // Position in the type parameter list (0-based)
	// Canonical marks a fresh copy standing for a generic target
	// signature's own parameter while relating it (tsc's canonical
	// signature): it is related only to itself, never matched by name.
	Canonical bool
}

func (tp *TypeParameter) String() string {
	var result strings.Builder
	result.WriteString(tp.Name)
	if tp.Constraint != nil {
		result.WriteString(" extends ")
		result.WriteString(tp.Constraint.String())
	}
	if tp.Default != nil {
		result.WriteString(" = ")
		result.WriteString(tp.Default.String())
	}
	return result.String()
}

// TypeParameterType represents a reference to a type parameter within a generic type or function
// This is what gets used inside the generic body (e.g., the "T" in "return x: T")
type TypeParameterType struct {
	Parameter *TypeParameter // Reference to the parameter definition
}

func (t *TypeParameterType) String() string {
	return t.Parameter.Name
}

func (t *TypeParameterType) Equals(other Type) bool {
	if o, ok := other.(*TypeParameterType); ok {
		// Two type parameter types are equal if they reference the same parameter
		return t.Parameter == o.Parameter
	}
	return false
}

func (t *TypeParameterType) typeNode() {}

// GenericType represents a generic type definition (before instantiation)
// This is the "template" that gets instantiated with concrete types
type GenericType struct {
	Name           string           // Name of the generic type (e.g., "Array", "Promise")
	TypeParameters []*TypeParameter // The type parameters (e.g., [T] for Array<T>)
	Body           Type             // The body type that may contain TypeParameterType references
	// InstanceType is, for a generic class's constructor, the generic of its
	// instances: what the class name means in a type position. An exported
	// generic class is exported as its constructor.
	InstanceType *GenericType
}

func (g *GenericType) String() string {
	paramNames := make([]string, len(g.TypeParameters))
	for i, param := range g.TypeParameters {
		paramNames[i] = param.String()
	}
	return fmt.Sprintf("%s<%s>", g.Name, strings.Join(paramNames, ", "))
}

func (g *GenericType) Equals(other Type) bool {
	if o, ok := other.(*GenericType); ok {
		if g == o {
			return true
		}
		if g.Name != o.Name || len(g.TypeParameters) != len(o.TypeParameters) {
			return false
		}
		// Anonymous generic function types (`<T>(x: T) => T[]`) all share
		// the name "Function"; they are related structurally instead.
		if g.Name == "Function" {
			return false
		}
		// For now, just compare names and arity
		// Full structural comparison would require checking body types
		return true
	}
	return false
}

func (g *GenericType) typeNode() {}

// InstantiatedType represents a generic type with concrete type arguments
// This is what you get when you write Array<string> - an instantiation of the Array generic
type InstantiatedType struct {
	Generic       *GenericType // The generic type being instantiated
	TypeArguments []Type       // The concrete type arguments (e.g., [string] for Array<string>)
	substituted   Type         // Cached result of substitution (computed lazily)
}

func (i *InstantiatedType) String() string {
	argStrings := make([]string, len(i.TypeArguments))
	for idx, arg := range i.TypeArguments {
		argStrings[idx] = arg.String()
	}
	return fmt.Sprintf("%s<%s>", i.Generic.Name, strings.Join(argStrings, ", "))
}

func (i *InstantiatedType) Equals(other Type) bool {
	return i.equalsIn(other, nil)
}

func (i *InstantiatedType) equalsIn(other Type, s *equalsState) bool {
	if o, ok := other.(*InstantiatedType); ok {
		if !i.Generic.Equals(o.Generic) || len(i.TypeArguments) != len(o.TypeArguments) {
			return false
		}
		for idx, arg := range i.TypeArguments {
			if !equalsIn(arg, o.TypeArguments[idx], s) {
				return false
			}
		}
		return true
	}
	return false
}

func (i *InstantiatedType) typeNode() {}

// Substitute returns the concrete type by replacing type parameters with type arguments
// This is where the "type-level lambda" application happens
func (i *InstantiatedType) Substitute() Type {
	if i.substituted != nil {
		return i.substituted
	}

	if len(i.TypeArguments) != len(i.Generic.TypeParameters) {
		// This should be caught earlier, but return Any as fallback
		i.substituted = Any
		return i.substituted
	}

	// Build substitution map
	substitutions := make(map[*TypeParameter]Type)
	for idx, param := range i.Generic.TypeParameters {
		substitutions[param] = i.TypeArguments[idx]
	}

	// Substitute in the body
	i.substituted = substituteType(i.Generic.Body, substitutions)
	return i.substituted
}

// substituteType performs type parameter substitution in a type
func substituteType(t Type, substitutions map[*TypeParameter]Type) Type {
	return substituteTypeMemo(t, substitutions, make(map[*ObjectType]*ObjectType))
}

// substituteTypeMemo is substituteType with a table of the object types
// already being copied, so a type that refers back to itself (a class whose
// method returns the class) terminates by reusing the copy under construction.
func substituteTypeMemo(t Type, substitutions map[*TypeParameter]Type, memo map[*ObjectType]*ObjectType) Type {
	if t == nil {
		return nil
	}

	// Types that mention none of the substituted parameters are returned as
	// they are, so identity (and class metadata) survives.
	if !mentionsSubstituted(t, substitutions, memo) {
		return t
	}

	switch t := t.(type) {
	case *TypeParameterType:
		// Replace type parameter with concrete type
		if replacement, ok := substitutions[t.Parameter]; ok {
			return replacement
		}
		return t // Not found, return unchanged

	case *ArrayType:
		// Recursively substitute in element type
		newElementType := substituteTypeMemo(t.ElementType, substitutions, memo)
		return &ArrayType{ElementType: newElementType}

	case *ObjectType:
		// Deep copy and substitute in properties
		if done, ok := memo[t]; ok && done != mentionsMarker {
			return done
		}
		if t.Pending {
			// Filled in later, in place: no copy.
			return t
		}
		newObj := NewObjectType()
		newObj.PropertyDocs = t.PropertyDocs
		newObj.Doc = t.Doc
		t.CopyDisplay(newObj, func(a Type) Type { return substituteTypeMemo(a, substitutions, memo) })
		memo[t] = newObj
		for _, name := range t.PropertyNames() {
			propType := t.Props()[name]
			newObj.SetProperty(name, substituteTypeMemo(propType, substitutions, memo))
		}
		// Copy optional properties
		for name, isOptional := range t.OptionalProps() {
			newObj.OptionalProps()[name] = isOptional
		}
		if len(t.ReadOnlyProps()) > 0 {
			newObj.Materialize()
			newObj.ReadOnlyProperties = make(map[string]bool, len(t.ReadOnlyProps()))
			for name, ro := range t.ReadOnlyProps() {
				newObj.ReadOnlyProps()[name] = ro
			}
		}
		if len(t.Bases()) > 0 {
			newObj.Materialize()
			newObj.BaseTypes = make([]Type, len(t.Bases()))
			for i, base := range t.Bases() {
				newObj.Bases()[i] = substituteTypeMemo(base, substitutions, memo)
			}
		}
		newObj.ClassMeta = t.ClassMeta
		newObj.IsInterface = t.IsInterface

		// Handle call signatures
		for _, sig := range t.CallSigs() {
			newSig := substituteMemberSignature(sig, substitutions, memo)
			newObj.Materialize()
			newObj.CallSignatures = append(newObj.CallSigs(), newSig)
		}

		// Handle constructor signatures
		for _, sig := range t.ConstructSigs() {
			newSig := substituteMemberSignature(sig, substitutions, memo)
			newObj.Materialize()
			newObj.ConstructSignatures = append(newObj.ConstructSigs(), newSig)
		}

		// Copy index signatures
		for _, indexSig := range t.IndexSigs() {
			newIndexSig := &IndexSignature{
				KeyType:   substituteTypeMemo(indexSig.KeyType, substitutions, memo),
				ValueType: substituteTypeMemo(indexSig.ValueType, substitutions, memo),
			}
			newObj.Materialize()
			newObj.IndexSignatures = append(newObj.IndexSigs(), newIndexSig)
		}

		return newObj

	case *UnionType:
		// Substitute in all constituent types
		newTypes := make([]Type, len(t.Types))
		for i, constituent := range t.Types {
			newTypes[i] = substituteTypeMemo(constituent, substitutions, memo)
		}
		return NewUnionType(newTypes...)

	case *IntersectionType:
		// Substitute in all constituent types
		newTypes := make([]Type, len(t.Types))
		for i, constituent := range t.Types {
			newTypes[i] = substituteTypeMemo(constituent, substitutions, memo)
		}
		return NewIntersectionType(newTypes...)

	case *InstantiatedType:
		// Recursively substitute in type arguments
		newArgs := make([]Type, len(t.TypeArguments))
		for i, arg := range t.TypeArguments {
			newArgs[i] = substituteTypeMemo(arg, substitutions, memo)
		}
		// Return the InstantiatedType with substituted arguments
		// Don't call Substitute() here to avoid infinite recursion with self-referential types
		return NewInstantiatedType(t.Generic, newArgs)

	case *ReadonlyType:
		// Substitute in the inner type
		newInnerType := substituteTypeMemo(t.InnerType, substitutions, memo)
		return NewReadonlyType(newInnerType)

	// Type operators are substituted structurally; the checker evaluates them
	// where they are used (a call returning zod's objectOutputType<Shape>
	// left Shape inside its mapped and conditional parts).
	case *TupleType:
		elems := make([]Type, len(t.ElementTypes))
		for i, e := range t.ElementTypes {
			elems[i] = substituteTypeMemo(e, substitutions, memo)
		}
		return &TupleType{ElementTypes: elems, OptionalElements: t.OptionalElements, RestElementType: substituteTypeMemo(t.RestElementType, substitutions, memo)}
	case *MappedType:
		cp := *t
		cp.ConstraintType = substituteTypeMemo(t.ConstraintType, substitutions, memo)
		cp.ValueType = substituteTypeMemo(t.ValueType, substitutions, memo)
		return &cp
	case *ConditionalType:
		return &ConditionalType{
			CheckType:   substituteTypeMemo(t.CheckType, substitutions, memo),
			ExtendsType: substituteTypeMemo(t.ExtendsType, substitutions, memo),
			TrueType:    substituteTypeMemo(t.TrueType, substitutions, memo),
			FalseType:   substituteTypeMemo(t.FalseType, substitutions, memo),
		}
	case *IndexedAccessType:
		return &IndexedAccessType{
			ObjectType: substituteTypeMemo(t.ObjectType, substitutions, memo),
			IndexType:  substituteTypeMemo(t.IndexType, substitutions, memo),
		}
	case *KeyofType:
		return &KeyofType{OperandType: substituteTypeMemo(t.OperandType, substitutions, memo)}
	case *GenericTypeAliasForwardReference:
		args := make([]Type, len(t.TypeArguments))
		for i, a := range t.TypeArguments {
			args[i] = substituteTypeMemo(a, substitutions, memo)
		}
		return &GenericTypeAliasForwardReference{AliasName: t.AliasName, TypeArguments: args, Scope: t.Scope}
	case *GenericType:
		// Substitute the outer parameters inside a generic method's body; its
		// own type parameters stay generic.
		return &GenericType{Name: t.Name, TypeParameters: t.TypeParameters, Body: substituteTypeMemo(t.Body, substitutions, memo)}
	case *TemplateLiteralType:
		parts := make([]TemplateLiteralPart, len(t.Parts))
		for i, part := range t.Parts {
			parts[i] = part
			if !part.IsLiteral {
				parts[i].Type = substituteTypeMemo(part.Type, substitutions, memo)
			}
		}
		return &TemplateLiteralType{Parts: parts}

	// For primitive types and other types that don't contain type parameters
	default:
		return t
	}
}

// Helper functions for creating generic types

// NewTypeParameter creates a new type parameter
func NewTypeParameter(name string, index int, constraint Type) *TypeParameter {
	return &TypeParameter{
		Name:       name,
		Constraint: constraint,
		Index:      index,
	}
}

// NewGenericType creates a new generic type definition
func NewGenericType(name string, typeParams []*TypeParameter, body Type) *GenericType {
	return &GenericType{
		Name:           name,
		TypeParameters: typeParams,
		Body:           body,
	}
}

// NewInstantiatedType creates a new instantiated generic type
func NewInstantiatedType(generic *GenericType, typeArgs []Type) *InstantiatedType {
	return &InstantiatedType{
		Generic:       generic,
		TypeArguments: typeArgs,
	}
}

// Built-in generic types (these replace our hardcoded approach)

var (
	// Array<T> generic type
	ArrayGeneric *GenericType

	// Promise<T> generic type
	PromiseGeneric *GenericType

	// PromiseLike<T>, lib.es5's thenable interface
	PromiseLikeGeneric *GenericType

	// Generator<T, TReturn, TNext> generic type
	GeneratorGeneric *GenericType

	// AsyncGenerator<T, TReturn, TNext> generic type
	AsyncGeneratorGeneric *GenericType
)

func init() {
	// Create Array<T> generic type
	arrayT := NewTypeParameter("T", 0, nil)
	arrayBody := &ArrayType{ElementType: &TypeParameterType{Parameter: arrayT}}
	ArrayGeneric = NewGenericType("Array", []*TypeParameter{arrayT}, arrayBody)

	// Create Promise<T> generic type
	promiseT := NewTypeParameter("T", 0, nil)
	promiseTType := &TypeParameterType{Parameter: promiseT}

	// Promise<T>'s methods as lib.es5 declares them. The body names
	// Promise itself, so the generic is made first and filled in after.
	promiseGeneric := &GenericType{Name: "Promise", TypeParameters: []*TypeParameter{promiseT}}
	promiseOf := func(t Type) Type { return NewInstantiatedType(promiseGeneric, []Type{t}) }
	// FIXME: PromiseLike<T> is not modelled; a callback's T | PromiseLike<T>
	// is T | Promise<T>.
	awaitable := func(t Type) Type { return NewUnionType(t, promiseOf(t)) }
	orNullish := func(t Type) Type { return NewUnionType(t, Undefined, Null) }
	promiseBody := NewObjectType()

	// then<TResult1 = T, TResult2 = never>(
	//   onfulfilled?: ((value: T) => TResult1 | PromiseLike<TResult1>) | undefined | null,
	//   onrejected?: ((reason: any) => TResult2 | PromiseLike<TResult2>) | undefined | null,
	// ): Promise<TResult1 | TResult2>
	tResult1 := &TypeParameter{Name: "TResult1", Default: promiseTType, Index: 0}
	tResult2 := &TypeParameter{Name: "TResult2", Default: Never, Index: 1}
	r1, r2 := &TypeParameterType{Parameter: tResult1}, &TypeParameterType{Parameter: tResult2}
	onFulfilled := NewFunctionType(&Signature{ParameterNames: []string{"value"}, ParameterTypes: []Type{promiseTType}, ReturnType: awaitable(r1)})
	onRejected := NewFunctionType(&Signature{ParameterNames: []string{"reason"}, ParameterTypes: []Type{Any}, ReturnType: awaitable(r2)})
	promiseBody.WithProperty("then", &GenericType{
		Name:           "then",
		TypeParameters: []*TypeParameter{tResult1, tResult2},
		Body: NewFunctionType(&Signature{
			ParameterNames: []string{"onfulfilled", "onrejected"},
			ParameterTypes: []Type{orNullish(onFulfilled), orNullish(onRejected)},
			OptionalParams: []bool{true, true},
			ReturnType:     promiseOf(NewUnionType(r1, r2)),
		}),
	})

	// catch<TResult = never>(
	//   onrejected?: ((reason: any) => TResult | PromiseLike<TResult>) | undefined | null,
	// ): Promise<T | TResult>
	tResult := &TypeParameter{Name: "TResult", Default: Never, Index: 0}
	r := &TypeParameterType{Parameter: tResult}
	onCatch := NewFunctionType(&Signature{ParameterNames: []string{"reason"}, ParameterTypes: []Type{Any}, ReturnType: awaitable(r)})
	promiseBody.WithProperty("catch", &GenericType{
		Name:           "catch",
		TypeParameters: []*TypeParameter{tResult},
		Body: NewFunctionType(&Signature{
			ParameterNames: []string{"onrejected"},
			ParameterTypes: []Type{orNullish(onCatch)},
			OptionalParams: []bool{true},
			ReturnType:     promiseOf(NewUnionType(promiseTType, r)),
		}),
	})

	// finally(onfinally?: (() => void) | undefined | null): Promise<T>
	onFinally := NewFunctionType(&Signature{ReturnType: Void})
	promiseBody.WithProperty("finally", NewFunctionType(&Signature{
		ParameterNames: []string{"onfinally"},
		ParameterTypes: []Type{orNullish(onFinally)},
		OptionalParams: []bool{true},
		ReturnType:     promiseOf(promiseTType),
	}))

	// Its instances print, and relate by arguments, as Promise<X>.
	promiseBody.GenericName = "Promise"
	promiseBody.GenericArgs = []Type{promiseTType}
	promiseGeneric.Body = promiseBody
	PromiseGeneric = promiseGeneric

	// interface PromiseLike<T> {
	//   then<TResult1 = T, TResult2 = never>(
	//     onfulfilled?: ((value: T) => TResult1 | PromiseLike<TResult1>) | undefined | null,
	//     onrejected?: ((reason: any) => TResult2 | PromiseLike<TResult2>) | undefined | null,
	//   ): PromiseLike<TResult1 | TResult2>;
	// }
	likeT := NewTypeParameter("T", 0, nil)
	likeTType := &TypeParameterType{Parameter: likeT}
	likeGeneric := &GenericType{Name: "PromiseLike", TypeParameters: []*TypeParameter{likeT}}
	likeOf := func(t Type) Type { return NewInstantiatedType(likeGeneric, []Type{t}) }
	likeResult1 := &TypeParameter{Name: "TResult1", Default: likeTType, Index: 0}
	likeResult2 := &TypeParameter{Name: "TResult2", Default: Never, Index: 1}
	lr1, lr2 := &TypeParameterType{Parameter: likeResult1}, &TypeParameterType{Parameter: likeResult2}
	likeFulfilled := NewFunctionType(&Signature{ParameterNames: []string{"value"}, ParameterTypes: []Type{likeTType}, ReturnType: NewUnionType(lr1, likeOf(lr1))})
	likeRejected := NewFunctionType(&Signature{ParameterNames: []string{"reason"}, ParameterTypes: []Type{Any}, ReturnType: NewUnionType(lr2, likeOf(lr2))})
	likeBody := NewObjectType()
	likeBody.IsInterface = true
	likeBody.WithProperty("then", &GenericType{
		Name:           "then",
		TypeParameters: []*TypeParameter{likeResult1, likeResult2},
		Body: NewFunctionType(&Signature{
			ParameterNames: []string{"onfulfilled", "onrejected"},
			ParameterTypes: []Type{orNullish(likeFulfilled), orNullish(likeRejected)},
			OptionalParams: []bool{true, true},
			ReturnType:     likeOf(NewUnionType(lr1, lr2)),
		}),
	})
	likeBody.GenericName = "PromiseLike"
	likeBody.GenericArgs = []Type{likeTType}
	likeGeneric.Body = likeBody
	// FIXME: an object literal implementing then by hand (`{ then(ok) {...} }`)
	// does not relate to the generic then, which tsc accepts.
	PromiseLikeGeneric = likeGeneric

	// Create Generator<T, TReturn, TNext> generic type
	t := NewTypeParameter("T", 0, nil)
	tReturn := NewTypeParameter("TReturn", 1, nil)
	tNext := NewTypeParameter("TNext", 2, nil)

	// Create types for the parameters
	tType := &TypeParameterType{Parameter: t}
	tReturnType := &TypeParameterType{Parameter: tReturn}
	tNextType := &TypeParameterType{Parameter: tNext}

	// Create IteratorResult<T, TReturn> type
	iteratorResultType := NewObjectType().
		WithProperty("value", NewUnionType(tType, tReturnType)).
		WithProperty("done", Boolean)

	// Create generator body with iterator protocol methods
	generatorBody := NewObjectType().
		// next(value?: TNext): IteratorResult<T, TReturn>
		WithProperty("next", NewOptionalFunction(
			[]Type{tNextType},
			iteratorResultType,
			[]bool{true})).
		// return(value?: any): IteratorResult<T, TReturn>
		// Note: .return() accepts any value, not just TReturn
		WithProperty("return", NewOptionalFunction(
			[]Type{Any},
			iteratorResultType,
			[]bool{true})).
		// throw(exception?: any): IteratorResult<T, TReturn>
		WithProperty("throw", NewOptionalFunction(
			[]Type{Any},
			iteratorResultType,
			[]bool{true}))

	// lib.es2015.generator: interface Generator<T = unknown, TReturn = any, TNext = any>
	t.Default, tReturn.Default, tNext.Default = Unknown, Any, Any
	generatorBody.GenericName = "Generator"
	generatorBody.GenericArgs = []Type{tType, tReturnType, tNextType}
	GeneratorGeneric = NewGenericType("Generator", []*TypeParameter{t, tReturn, tNext}, generatorBody)

	// Create AsyncGenerator<T, TReturn, TNext> generic type
	tAsync := NewTypeParameter("T", 0, nil)
	tReturnAsync := NewTypeParameter("TReturn", 1, nil)
	tNextAsync := NewTypeParameter("TNext", 2, nil)

	tTypeAsync := &TypeParameterType{Parameter: tAsync}
	tReturnTypeAsync := &TypeParameterType{Parameter: tReturnAsync}
	tNextTypeAsync := &TypeParameterType{Parameter: tNextAsync}

	// Create IteratorResult<T, TReturn> type for async generator
	iteratorResultTypeAsync := NewObjectType().
		WithProperty("value", NewUnionType(tTypeAsync, tReturnTypeAsync)).
		WithProperty("done", Boolean)

	// For AsyncGenerator, methods return Promise<IteratorResult<T, TReturn>>
	// We instantiate the Promise generic with IteratorResult
	promiseIteratorResult := NewInstantiatedType(PromiseGeneric, []Type{iteratorResultTypeAsync})

	// Create async generator body with iterator protocol methods that return promises
	asyncGeneratorBody := NewObjectType().
		// next(value?: TNext): Promise<IteratorResult<T, TReturn>>
		WithProperty("next", NewOptionalFunction(
			[]Type{tNextTypeAsync},
			promiseIteratorResult,
			[]bool{true})).
		// return(value?: any): Promise<IteratorResult<T, TReturn>>
		// Note: .return() accepts any value, not just TReturn
		WithProperty("return", NewOptionalFunction(
			[]Type{Any},
			promiseIteratorResult,
			[]bool{true})).
		// throw(exception?: any): Promise<IteratorResult<T, TReturn>>
		WithProperty("throw", NewOptionalFunction(
			[]Type{Any},
			promiseIteratorResult,
			[]bool{true}))

	AsyncGeneratorGeneric = NewGenericType("AsyncGenerator", []*TypeParameter{tAsync, tReturnAsync, tNextAsync}, asyncGeneratorBody)
}

// substituteSignature performs type parameter substitution in a signature
func substituteSignature(sig *Signature, substitutions map[*TypeParameter]Type) *Signature {
	return substituteSignatureMemo(sig, substitutions, make(map[*ObjectType]*ObjectType))
}

// substituteMemberSignature substitutes in a signature that is a member of an
// object type. The signature's own type parameters shadow any substituted
// parameter of the same identity, so they are left alone.
func substituteMemberSignature(sig *Signature, substitutions map[*TypeParameter]Type, memo map[*ObjectType]*ObjectType) *Signature {
	if sig != nil && len(sig.TypeParameters) > 0 {
		shadowed := false
		for _, tp := range sig.TypeParameters {
			if _, ok := substitutions[tp]; ok {
				shadowed = true
				break
			}
		}
		if shadowed {
			inner := make(map[*TypeParameter]Type, len(substitutions))
			for k, v := range substitutions {
				inner[k] = v
			}
			for _, tp := range sig.TypeParameters {
				delete(inner, tp)
			}
			return substituteSignatureMemo(sig, inner, make(map[*ObjectType]*ObjectType))
		}
	}
	// FIXME: the member's own type parameters are dropped here, so the
	// method `pipeThrough<T>` of an instantiated `ReadableStream<number>`
	// loses T. Keeping them (out.TypeParameters = sig.TypeParameters) makes
	// relating types with overloaded generic methods (tRPC's Observable.pipe)
	// blow up exponentially: each erasure copies the instantiations afresh,
	// so no related pair repeats. Needs canonical signatures (tsc relates
	// generic signatures without erasing) or interned instantiations first.
	return substituteSignatureMemo(sig, substitutions, memo)
}

func substituteSignatureMemo(sig *Signature, substitutions map[*TypeParameter]Type, memo map[*ObjectType]*ObjectType) *Signature {
	if sig == nil {
		return nil
	}

	// Substitute parameter types
	newParamTypes := make([]Type, len(sig.ParameterTypes))
	for i, paramType := range sig.ParameterTypes {
		newParamTypes[i] = substituteTypeMemo(paramType, substitutions, memo)
	}

	// Substitute return type
	newReturnType := substituteTypeMemo(sig.ReturnType, substitutions, memo)

	// Substitute rest parameter type if present
	var newRestParamType Type
	if sig.RestParameterType != nil {
		newRestParamType = substituteTypeMemo(sig.RestParameterType, substitutions, memo)
	}

	return &Signature{
		ParameterTypes:    newParamTypes,
		ReturnType:        newReturnType,
		OptionalParams:    sig.OptionalParams, // Copy as-is
		IsVariadic:        sig.IsVariadic,
		StrictVariance:    sig.StrictVariance,
		RestParameterType: newRestParamType,
	}
}

// mentionsMarker is memo's entry for an object type a substitution has found
// to mention one of its parameters but has not copied yet.
var mentionsMarker = &ObjectType{}

// mentionsSubstituted is mentionsTypeParameter for one substitution, with
// every object type's answer remembered in memo: an object the substitution
// leaves as it is maps to itself, one it will copy to mentionsMarker (or to
// its copy). Objects on a cycle are settled together, as Tarjan's algorithm
// settles a strongly connected component, so substituting into each member of
// a large graph does not walk the graph again.
func mentionsSubstituted(t Type, substitutions map[*TypeParameter]Type, memo map[*ObjectType]*ObjectType) bool {
	if o, ok := t.(*ObjectType); ok {
		if done, ok := memo[o]; ok {
			return done != o
		}
	}
	w := &mentionWalker{params: substitutions, memo: memo, index: map[*ObjectType]int{}, onStack: map[*ObjectType]bool{}}
	if w.mentions(t) {
		// What is left on the stack reaches an object on the path to the
		// mention, so it mentions a parameter too.
		for _, o := range w.stack {
			if !o.Pending {
				if _, ok := memo[o]; !ok {
					memo[o] = mentionsMarker
				}
			}
		}
		return true
	}
	return false
}

// mentionsTypeParameter reports whether t refers to any of the given type
// parameters (visited guards against self-referential types).
func mentionsTypeParameter(t Type, params map[*TypeParameter]Type, visited, known map[Type]bool, target Type) bool {
	w := &mentionWalker{params: params, visited: visited, known: known, target: target}
	return w.mentions(t)
}

// mentionWalker walks a type for mentions of params (or of target). With
// memo set it is mentionsSubstituted's walk, and object types are tracked
// with Tarjan's index, low link and stack; otherwise visited (and known)
// guard against cycles.
type mentionWalker struct {
	params         map[*TypeParameter]Type
	target         Type
	visited, known map[Type]bool

	memo    map[*ObjectType]*ObjectType
	index   map[*ObjectType]int
	onStack map[*ObjectType]bool
	stack   []*ObjectType
	low     int
}

func (w *mentionWalker) mentions(t Type) bool {
	if t == nil {
		return false
	}
	if w.target != nil && t == w.target {
		return true
	}
	switch tt := t.(type) {
	case *TypeParameterType:
		_, ok := w.params[tt.Parameter]
		return ok
	case *ArrayType:
		return w.mentions(tt.ElementType)
	case *ReadonlyType:
		return w.mentions(tt.InnerType)
	case *UnionType:
		for _, m := range tt.Types {
			if w.mentions(m) {
				return true
			}
		}
	case *IntersectionType:
		for _, m := range tt.Types {
			if w.mentions(m) {
				return true
			}
		}
	case *InstantiatedType:
		for _, a := range tt.TypeArguments {
			if w.mentions(a) {
				return true
			}
		}
	case *ObjectType:
		if w.memo != nil {
			return w.objectInComponent(tt)
		}
		if w.visited[t] || w.known[t] {
			return false
		}
		w.visited[t] = true
		return w.objectMembers(tt)
	case *TupleType:
		for _, e := range tt.ElementTypes {
			if w.mentions(e) {
				return true
			}
		}
		return w.mentions(tt.RestElementType)
	case *MappedType:
		return w.mentions(tt.ConstraintType) || w.mentions(tt.ValueType)
	case *ConditionalType:
		return w.mentions(tt.CheckType) || w.mentions(tt.ExtendsType) || w.mentions(tt.TrueType) || w.mentions(tt.FalseType)
	case *IndexedAccessType:
		return w.mentions(tt.ObjectType) || w.mentions(tt.IndexType)
	case *KeyofType:
		return w.mentions(tt.OperandType)
	case *GenericType:
		// A generic method of a generic interface (Iterator<T>.map<U>)
		// mentions the outer parameters in its body.
		return w.mentions(tt.Body)
	case *GenericTypeAliasForwardReference:
		for _, a := range tt.TypeArguments {
			if w.mentions(a) {
				return true
			}
		}
	case *TemplateLiteralType:
		for _, part := range tt.Parts {
			if !part.IsLiteral && w.mentions(part.Type) {
				return true
			}
		}
	default:
		// Other types are conservatively treated as possibly mentioning the
		// parameters.
		switch t.(type) {
		case *Primitive, *LiteralType, *EnumMemberType, *EnumType:
			return false
		}
		return true
	}
	return false
}

// objectInComponent is the object step of mentionsSubstituted's walk. An
// object whose walk finds no mention and reaches nothing still open below its
// own index closes a component: all of it is recorded in memo as mentioning
// nothing. A mention ends the whole walk, and mentionsSubstituted records what
// is still on the stack.
func (w *mentionWalker) objectInComponent(o *ObjectType) bool {
	if done, ok := w.memo[o]; ok {
		return done != o
	}
	if i, ok := w.index[o]; ok {
		if w.onStack[o] && i < w.low {
			w.low = i
		}
		return false
	}
	i := len(w.index)
	w.index[o] = i
	w.onStack[o] = true
	w.stack = append(w.stack, o)
	outer := w.low
	w.low = i
	if w.objectMembers(o) {
		return true
	}
	low := w.low
	if low < outer {
		w.low = low
	} else {
		w.low = outer
	}
	if low == i {
		for {
			top := w.stack[len(w.stack)-1]
			w.stack = w.stack[:len(w.stack)-1]
			w.onStack[top] = false
			if !top.Pending {
				w.memo[top] = top
			}
			if top == o {
				break
			}
		}
	}
	return false
}

func (w *mentionWalker) objectMembers(tt *ObjectType) bool {
	if tt.ClosedInstance {
		for _, a := range tt.GenericArgs {
			if w.mentions(a) {
				return true
			}
		}
		return false
	}
	for _, p := range tt.Props() {
		if w.mentions(p) {
			return true
		}
	}
	for _, sigs := range [][]*Signature{tt.CallSigs(), tt.ConstructSigs()} {
		for _, sig := range sigs {
			for _, p := range sig.ParameterTypes {
				if w.mentions(p) {
					return true
				}
			}
			if w.mentions(sig.ReturnType) || w.mentions(sig.RestParameterType) || w.mentions(sig.ThisType) {
				return true
			}
			for _, tp := range sig.TypeParameters {
				if tp != nil && (w.mentions(tp.Constraint) || w.mentions(tp.Default)) {
					return true
				}
			}
		}
	}
	for _, idx := range tt.IndexSigs() {
		if w.mentions(idx.KeyType) || w.mentions(idx.ValueType) || w.mentions(idx.ConstraintType) {
			return true
		}
	}
	for _, base := range tt.Bases() {
		if w.mentions(base) {
			return true
		}
	}
	for _, a := range tt.GenericArgs {
		if w.mentions(a) {
			return true
		}
	}
	return false
}

// PolymorphicThis is the type parameter `this` stands for in a class member's
// declared type (`optional(): ZodOptional<this>`): the type of the receiver
// the member is read from, substituted at the read.
var PolymorphicThis = &TypeParameter{Name: "this"}

// MentionsTypeParameter reports whether t may refer to p (conservatively:
// type operators it does not walk count as a mention).
func MentionsTypeParameter(t Type, p *TypeParameter) bool {
	return mentionsTypeParameter(t, map[*TypeParameter]Type{p: nil}, map[Type]bool{}, nil, nil)
}

// MentionsAnyTypeParameter reports whether t may refer to any parameter in
// params (conservatively, as mentionsTypeParameter).
func MentionsAnyTypeParameter(t Type, params map[*TypeParameter]Type) bool {
	return mentionsTypeParameter(t, params, map[Type]bool{}, nil, nil)
}

// Reaches reports whether target may be reachable from t (conservatively, as
// mentionsTypeParameter: type operators it does not walk count as reaching).
func Reaches(t, target Type) bool {
	return mentionsTypeParameter(t, nil, map[Type]bool{}, nil, target)
}

// Reacher answers Reaches for one target over many starting points. A search
// that fails has walked only objects that cannot reach the target, so they
// are remembered and the next search does not walk them again: rebinding
// every inherited member of an interface asks about the same library graphs
// over and over.
type Reacher struct {
	target     Type
	unreaching map[Type]bool
}

// NewReacher returns a Reacher for target.
func NewReacher(target Type) *Reacher {
	return &Reacher{target: target, unreaching: map[Type]bool{}}
}

// Reaches reports whether r's target may be reachable from t, as Reaches.
func (r *Reacher) Reaches(t Type) bool {
	if r.unreaching[t] {
		return false
	}
	seen := map[Type]bool{}
	if mentionsTypeParameter(t, nil, seen, r.unreaching, r.target) {
		return true
	}
	for o := range seen {
		r.unreaching[o] = true
	}
	return false
}

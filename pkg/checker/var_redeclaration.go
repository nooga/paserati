package checker

import (
	"fmt"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// varDeclKey identifies a function-scoped `var` binding: the function (or
// global) scope that owns it plus its name.
type varDeclKey struct {
	scope *Environment
	name  string
}

// varDeclRecord is what we remember about the first declaration of a var.
type varDeclRecord struct {
	typ types.Type
	// reliable is true when typ came from an annotation or from an initializer
	// whose type does not depend on narrowing/inference we only approximate.
	reliable bool
	// annotated is true when typ was written out as a type annotation, or
	// comes from an initializer whose type we infer exactly.
	annotated bool
	// explicitAny is true when that annotation is the `any` keyword itself,
	// not a type the parser lowered to any.
	explicitAny bool
}

// firstVarDecl returns the record for the first declaration of the `var`
// binding name in scope, and whether one has been recorded.
func (c *Checker) firstVarDecl(scope *Environment, name string) (varDeclRecord, bool) {
	if c.varFirstDecl == nil {
		return varDeclRecord{}, false
	}
	r, ok := c.varFirstDecl[varDeclKey{scope, name}]
	return r, ok
}

func (c *Checker) recordFirstVarDecl(scope *Environment, name string, t types.Type, reliable bool, annotation, initializer parser.Expression) {
	annotated := annotation != nil || c.exactInitializer(initializer)
	if c.varFirstDecl == nil {
		c.varFirstDecl = make(map[varDeclKey]varDeclRecord)
	}
	c.varFirstDecl[varDeclKey{scope, name}] = varDeclRecord{typ: t, reliable: reliable, annotated: annotated, explicitAny: isAnyKeyword(annotation)}
}

// varDeclTypeIsReliable reports whether the type of a declaration can be
// trusted for the TS2403 identity comparison: an annotation always can; an
// initializer only when its type does not hinge on flow narrowing or call
// resolution, which the checker models only approximately.
func (c *Checker) varDeclTypeIsReliable(typeAnnotation, initializer parser.Expression) bool {
	if typeAnnotation != nil {
		return true
	}
	switch e := initializer.(type) {
	case nil:
		return false
	case *parser.NumberLiteral, *parser.StringLiteral, *parser.BooleanLiteral, *parser.BigIntLiteral,
		*parser.NullLiteral, *parser.UndefinedLiteral, *parser.RegexLiteral, *parser.TemplateLiteral,
		*parser.ObjectLiteral,
		*parser.FunctionLiteral, *parser.ArrowFunctionLiteral, *parser.ClassExpression,
		*parser.TypeAssertionExpression:
		return true
	case *parser.NewExpression:
		// User-defined classes only: builtin generics (Array, Map, ...) have
		// evolving or inferred type arguments we do not model exactly.
		return c.exactInitializer(e)
	case *parser.ArrayLiteral, *parser.Identifier, *parser.MemberExpression:
		return c.exactInitializer(e)
	case *parser.PrefixExpression:
		return e.Operator == "-" || e.Operator == "!" || e.Operator == "typeof"
	}
	return false
}

// exactInitializer reports whether an initializer's type is inferred exactly
// enough for the identity relation: a user class instance, or an array
// literal of those or of primitive literals.
func (c *Checker) exactInitializer(init parser.Expression) bool {
	switch e := init.(type) {
	case *parser.NewExpression:
		id, ok := e.Constructor.(*parser.Identifier)
		if ok && id.Value == "Array" && len(e.TypeArguments) == 1 {
			return true // new Array<T>() is T[]
		}
		return ok && !c.preexistingGlobals[id.Value]
	case *parser.Identifier, *parser.MemberExpression:
		if id, isIdent := init.(*parser.Identifier); isIdent {
			// A function or namespace declaration's value has its exact type.
			switch c.env.ResolveBindingKind(id.Value) {
			case BindingFunction, BindingNamespace:
				return true
			}
		}
		// A class or namespace value: `typeof X`.
		obj, ok := init.GetComputedType().(*types.ObjectType)
		return ok && strings.HasPrefix(obj.DisplayName, "typeof ")
	case *parser.ArrayLiteral:
		if len(e.Elements) == 0 {
			return false
		}
		for _, el := range e.Elements {
			switch el.(type) {
			case *parser.NumberLiteral, *parser.StringLiteral, *parser.BooleanLiteral:
			default:
				if !c.exactInitializer(el) {
					return false
				}
			}
		}
		return true
	}
	return false
}

// reportSubsequentVarDeclaration reports TS2403 when a later `var` declaration
// of an already-declared variable has a type that is not identical to the
// type of the first declaration.
//
// tsc compares with isTypeIdenticalTo. Our types are less precise than tsc's,
// so we only report when both types are reliable (see varDeclTypeIsReliable)
// and clearly different, rather than risk a false positive on a type we merely
// failed to model.
func (c *Checker) reportSubsequentVarDeclaration(name *parser.Identifier, first varDeclRecord, laterType types.Type, laterReliable bool, laterAnnotation, laterInitializer parser.Expression) {
	if name == nil || !first.reliable || !laterReliable {
		return
	}
	laterAnnotated := laterAnnotation != nil || c.exactInitializer(laterInitializer)
	if first.explicitAny || isAnyKeyword(laterAnnotation) {
		// An any written out is any: anything else differs from it.
		if (first.typ == types.Any) == (laterType == types.Any) {
			return
		}
	} else if !c.varTypesClearlyDiffer(first.typ, laterType, first.annotated && laterAnnotated) {
		return
	} else if lit, isLit := laterInitializer.(*parser.ObjectLiteral); isLit && c.flattenedSpreads[lit] {
		return // its type merges a union spread tsc would distribute
	}
	c.addErrorWithCode(name, tsSubsequentVarDecl, fmt.Sprintf(
		"Subsequent variable declarations must have the same type.  Variable '%s' must be of type '%s', but here has type '%s'.",
		name.Value, first.typ.String(), laterType.String()))
}

func (c *Checker) varTypesClearlyDiffer(a, b types.Type, bothAnnotated bool) bool {
	if a == nil || b == nil || a == b {
		return false
	}
	if isUnmodeledType(a) || isUnmodeledType(b) {
		return false
	}
	if a.String() == b.String() {
		return false
	}
	if bothAnnotated && isIdentityComparable(a, 0) && isIdentityComparable(b, 0) &&
		!containsAny(a, 0) && !containsAny(b, 0) {
		// Both sides are built from kinds the relation models (including
		// function and object literal types): use the real identity relation.
		return !c.rel.IsIdenticalType(a, b)
	}
	if bothAnnotated && !containsAny(a, 0) && !containsAny(b, 0) {
		// Identical types are assignable both ways, so failing either way
		// proves a difference even where identity is beyond our model.
		if _, isObj := a.(*types.ObjectType); isObj {
			if _, isObj := b.(*types.ObjectType); isObj {
				return !c.rel.IsAssignable(a, b) || !c.rel.IsAssignable(b, a)
			}
		}
	}
	if fa, fb := plainFunctionSignature(a), plainFunctionSignature(b); fa != nil && fb != nil {
		// Function types differ when their arity, a parameter or the return
		// type clearly does.
		if len(fa.ParameterTypes) != len(fb.ParameterTypes) || fa.IsVariadic != fb.IsVariadic {
			return true
		}
		for i := range fa.ParameterTypes {
			if c.varTypesClearlyDiffer(fa.ParameterTypes[i], fb.ParameterTypes[i], true) {
				return true
			}
		}
		// A function expression's inferred return type widens.
		return c.varTypesClearlyDiffer(c.rel.GetWidenedType(fa.ReturnType), c.rel.GetWidenedType(fb.ReturnType), true)
	}
	if objectUnionVersusObject(a, b) || objectUnionVersusObject(b, a) {
		// Identity never reduces a union: `A | B` is not `A`, even when B is
		// a subtype of A.
		return true
	}
	ka, kb := identityKind(a), identityKind(b)
	if ka == "" || kb == "" {
		return false // object-like types: structural identity is beyond what we model
	}
	if ka == "any" || kb == "any" {
		// The parser lowers annotations it does not understand (a parenthesized
		// type, for one) to `any`, so a mismatch against any proves nothing.
		return false
	}
	if ka != kb {
		return true
	}
	switch ka {
	case "primitive", "literal":
		return true // same kind, different text: string vs number, 1 vs 2
	case "array":
		return c.varTypesClearlyDiffer(a.(*types.ArrayType).ElementType, b.(*types.ArrayType).ElementType, bothAnnotated)
	case "union":
		if strings.Contains(a.String(), "any") || strings.Contains(b.String(), "any") {
			return false // see the any note above
		}
		return !c.isAssignableWithExpansion(a, b) || !c.isAssignableWithExpansion(b, a)
	}
	return false
}

// objectUnionVersusObject reports whether u is a union of several object
// types and o a single object type.
func objectUnionVersusObject(u, o types.Type) bool {
	union, ok := u.(*types.UnionType)
	if !ok || len(union.Types) < 2 {
		return false
	}
	for _, m := range union.Types {
		if _, isObj := m.(*types.ObjectType); !isObj {
			return false
		}
	}
	_, isObj := o.(*types.ObjectType)
	return isObj
}

// identityKind classifies a type for the identity comparison of a redeclared
// var. It is "" for types we do not compare (objects, functions, classes, ...):
// two structurally identical object types can print differently and mutual
// assignability is not identity.
func identityKind(t types.Type) string {
	switch tt := t.(type) {
	case *types.Primitive:
		if tt == types.Any {
			return "any"
		}
		return "primitive"
	case *types.LiteralType:
		return "literal"
	case *types.UnionType:
		return "union"
	case *types.ArrayType:
		if strings.Contains(t.String(), "any") {
			return "" // may be a lowered annotation; see varTypesClearlyDiffer
		}
		return "array"
	}
	return ""
}

// isUnmodeledType reports types the checker cannot meaningfully compare for
// identity (type parameters, still-unresolved references).
func isUnmodeledType(t types.Type) bool {
	switch t.(type) {
	case *types.TypeParameterType, *types.ForwardReferenceType:
		return true
	}
	return t == types.Unknown || t == types.Never
}

// checkSecondaryTopLevelVar checks a top-level `var` declarator that
// re-declares a variable already declared in the same scope: its initializer
// must be assignable to its own declared type, and that type must be identical
// to the first declaration's (TS2403). The variable keeps the first
// declaration's type.
func (c *Checker) checkSecondaryTopLevelVar(name *parser.Identifier, typeAnnotation, initializer parser.Expression, first varDeclRecord) {
	var declType types.Type
	if typeAnnotation != nil {
		declType = c.resolveTypeAnnotation(typeAnnotation)
	}
	var initType types.Type
	if initializer != nil {
		if declType != nil {
			c.visitWithContext(initializer, &ContextualType{ExpectedType: declType, IsContextual: true})
		} else {
			c.visit(initializer)
		}
		initType = initializer.GetComputedType()
	}
	if declType == nil {
		switch {
		case initType == nil:
			declType = types.Any
		case isFreshLiteralExpression(initializer) || c.isWideningLiteral(initType):
			declType = c.rel.GetWidenedType(initType)
		default:
			declType = c.rel.DeeplyWidenType(initType)
		}
	} else if initType != nil && !c.assignableToFresh(initializer, initType, declType) {
		c.reportNotAssignable(name, initializer, initType, declType, headAssign)
	}
	name.SetComputedType(declType)
	c.reportSubsequentVarDeclaration(name, first, declType, c.varDeclTypeIsReliable(typeAnnotation, initializer), typeAnnotation, initializer)
}

// checkTopLevelVarLikeDeclarators runs the Pass 5 initializer check for every
// declarator of a top-level let/const/var clause (not just the first), and the
// TS2403 redeclaration check for later `var` declarations of the same name.
func (c *Checker) checkTopLevelVarLikeDeclarators(declarations []*parser.VarDeclarator, isVar bool, globalEnv *Environment, flow *flowNarrowState) {
	for _, d := range declarations {
		if d == nil || d.Name == nil {
			continue
		}
		c.settlePendingInit(d.Name.Value)
		// An unannotated function literal initializer was fully handled by
		// Pass 2/3.
		value := d.Value
		if _, isFn := value.(*parser.FunctionLiteral); isFn && d.TypeAnnotation == nil {
			value = nil
		}
		if isVar {
			if first, seen := c.firstVarDecl(globalEnv, d.Name.Value); seen {
				if value != nil || d.Value == nil {
					c.checkSecondaryTopLevelVar(d.Name, d.TypeAnnotation, value, first)
				}
				continue
			}
		}
		c.checkVarLikeInitializerAndRefine(d.Name, d.TypeAnnotation, value, globalEnv, flow)
		if isVar {
			if t, _, ok := globalEnv.Resolve(d.Name.Value); ok {
				c.recordFirstVarDecl(globalEnv, d.Name.Value, t, c.varDeclTypeIsReliable(d.TypeAnnotation, d.Value), d.TypeAnnotation, d.Value)
			}
		}
	}
}

// isConstVarLikeName reports whether name is a top-level `const` (whose
// inferred type keeps enum member literals).
func isConstVarLikeName(name *parser.Identifier, env *Environment) bool {
	if name == nil {
		return false
	}
	if info, ok := env.symbols[name.Value]; ok {
		return info.IsConst
	}
	return false
}

// reportBuiltinRedeclaration reports TS2403 when a program's `var` redeclares a
// lib global (`var Symbol: any`) with a clearly different type. Only structural
// annotations are compared (a primitive keyword or an object type literal): a
// named type such as SymbolConstructor may be the lib's own interface, merged
// with the program's, which we cannot tell from here.
func (c *Checker) reportBuiltinRedeclaration(name *parser.Identifier, annotation parser.Expression, builtinType, declared types.Type) {
	if name == nil || annotation == nil || declared == nil || builtinType == nil || builtinType == types.Any {
		return
	}
	switch a := annotation.(type) {
	case *parser.ObjectTypeExpression:
	case *parser.Identifier:
		switch a.Value {
		case "any", "number", "string", "boolean", "symbol", "object", "bigint":
		default:
			return
		}
	default:
		return
	}
	if declared == builtinType || declared.String() == builtinType.String() {
		return
	}
	c.addErrorWithCode(name, tsSubsequentVarDecl, fmt.Sprintf(
		"Subsequent variable declarations must have the same type.  Variable '%s' must be of type '%s', but here has type '%s'.",
		name.Value, builtinType.String(), declared.String()))
}

// isIdentityComparable reports whether a type is built only from kinds whose
// structure the relation understands (no mapped, conditional, indexed-access,
// typeof or generic placeholders), so an identity comparison is trustworthy.
func isIdentityComparable(t types.Type, depth int) bool {
	if t == nil || depth > 5 {
		return depth > 5
	}
	switch tt := t.(type) {
	case *types.Primitive, *types.LiteralType, *types.EnumMemberType:
		return true
	case *types.ArrayType:
		return isIdentityComparable(tt.ElementType, depth+1)
	case *types.TupleType:
		for _, e := range tt.ElementTypes {
			if !isIdentityComparable(e, depth+1) {
				return false
			}
		}
		return tt.RestElementType == nil || isIdentityComparable(tt.RestElementType, depth+1)
	case *types.UnionType:
		for _, m := range tt.Types {
			if !isIdentityComparable(m, depth+1) {
				return false
			}
		}
		return true
	case *types.ObjectType:
		if tt.IsInterface || tt.ClassMeta != nil {
			return false // declaration merging is not modelled precisely enough
		}
		for _, p := range tt.Props() {
			if !isIdentityComparable(p, depth+1) {
				return false
			}
		}
		for _, sigs := range [][]*types.Signature{tt.CallSigs(), tt.ConstructSigs()} {
			for _, sig := range sigs {
				for _, p := range sig.ParameterTypes {
					if !isIdentityComparable(p, depth+1) {
						return false
					}
				}
				if sig.ReturnType != nil && !isIdentityComparable(sig.ReturnType, depth+1) {
					return false
				}
			}
		}
		return true
	}
	return false
}

// containsAny reports whether `any` occurs anywhere in an identity-comparable
// type. The parser lowers annotations it does not understand to any, so an
// identity mismatch involving any proves nothing.
func containsAny(t types.Type, depth int) bool {
	if t == nil || depth > 5 {
		return false
	}
	switch tt := t.(type) {
	case *types.Primitive:
		return tt == types.Any
	case *types.ArrayType:
		return containsAny(tt.ElementType, depth+1)
	case *types.TupleType:
		for _, e := range tt.ElementTypes {
			if containsAny(e, depth+1) {
				return true
			}
		}
		return containsAny(tt.RestElementType, depth+1)
	case *types.UnionType:
		for _, m := range tt.Types {
			if containsAny(m, depth+1) {
				return true
			}
		}
	case *types.ObjectType:
		for _, p := range tt.Props() {
			if containsAny(p, depth+1) {
				return true
			}
		}
		for _, sigs := range [][]*types.Signature{tt.CallSigs(), tt.ConstructSigs()} {
			for _, sig := range sigs {
				for _, p := range sig.ParameterTypes {
					if containsAny(p, depth+1) {
						return true
					}
				}
				if containsAny(sig.ReturnType, depth+1) {
					return true
				}
			}
		}
	}
	return false
}

// isAnyKeyword reports whether a type annotation is the `any` keyword.
func isAnyKeyword(annotation parser.Expression) bool {
	id, ok := annotation.(*parser.Identifier)
	return ok && id.Value == "any"
}

// plainFunctionSignature is the one non-generic call signature of a function
// type with no other members, or nil.
func plainFunctionSignature(t types.Type) *types.Signature {
	obj, ok := t.(*types.ObjectType)
	if !ok || len(obj.CallSigs()) != 1 || len(obj.ConstructSigs()) > 0 || len(obj.Props()) > 0 || len(obj.IndexSigs()) > 0 {
		return nil
	}
	sig := obj.CallSigs()[0]
	if len(sig.TypeParameters) > 0 || sig.ThisType != nil {
		return nil
	}
	return sig
}

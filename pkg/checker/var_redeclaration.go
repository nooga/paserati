package checker

import (
	"fmt"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
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

func (c *Checker) recordFirstVarDecl(scope *Environment, name string, t types.Type, reliable bool) {
	if c.varFirstDecl == nil {
		c.varFirstDecl = make(map[varDeclKey]varDeclRecord)
	}
	c.varFirstDecl[varDeclKey{scope, name}] = varDeclRecord{typ: t, reliable: reliable}
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
		id, ok := e.Constructor.(*parser.Identifier)
		return ok && !c.preexistingGlobals[id.Value]
	case *parser.PrefixExpression:
		return e.Operator == "-" || e.Operator == "!" || e.Operator == "typeof"
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
func (c *Checker) reportSubsequentVarDeclaration(name *parser.Identifier, first varDeclRecord, laterType types.Type, laterReliable bool) {
	if name == nil || !first.reliable || !laterReliable || !c.varTypesClearlyDiffer(first.typ, laterType) {
		return
	}
	c.addErrorWithCode(name, tsSubsequentVarDecl, fmt.Sprintf(
		"Subsequent variable declarations must have the same type.  Variable '%s' must be of type '%s', but here has type '%s'.",
		name.Value, first.typ.String(), laterType.String()))
}

func (c *Checker) varTypesClearlyDiffer(a, b types.Type) bool {
	if a == nil || b == nil || a == b {
		return false
	}
	if isUnmodeledType(a) || isUnmodeledType(b) {
		return false
	}
	if a.String() == b.String() {
		return false
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
		return c.varTypesClearlyDiffer(a.(*types.ArrayType).ElementType, b.(*types.ArrayType).ElementType)
	case "union":
		if strings.Contains(a.String(), "any") || strings.Contains(b.String(), "any") {
			return false // see the any note above
		}
		return !c.isAssignableWithExpansion(a, b) || !c.isAssignableWithExpansion(b, a)
	}
	return false
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
		case isFreshLiteralExpression(initializer):
			declType = types.GetWidenedType(initType)
		default:
			declType = types.DeeplyWidenType(initType)
		}
	} else if initType != nil && !c.isAssignableWithExpansion(initType, declType) {
		sourceTypeStr, targetTypeStr := c.getAssignmentErrorTypes(initType, declType)
		c.addErrorWithCode(initializer, errors.TS2322, fmt.Sprintf("Type '%s' is not assignable to type '%s'.", sourceTypeStr, targetTypeStr))
	}
	name.SetComputedType(declType)
	c.reportSubsequentVarDeclaration(name, first, declType, c.varDeclTypeIsReliable(typeAnnotation, initializer))
}

// checkTopLevelVarLikeDeclarators runs the Pass 5 initializer check for every
// declarator of a top-level let/const/var clause (not just the first), and the
// TS2403 redeclaration check for later `var` declarations of the same name.
func (c *Checker) checkTopLevelVarLikeDeclarators(declarations []*parser.VarDeclarator, isVar bool, globalEnv *Environment, flow *flowNarrowState) {
	for _, d := range declarations {
		if d == nil || d.Name == nil {
			continue
		}
		// A function literal initializer was fully handled by Pass 2/3.
		value := d.Value
		if _, isFn := value.(*parser.FunctionLiteral); isFn {
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
				c.recordFirstVarDecl(globalEnv, d.Name.Value, t, c.varDeclTypeIsReliable(d.TypeAnnotation, d.Value))
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

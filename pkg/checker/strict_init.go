package checker

import (
	"fmt"
	"reflect"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// checkStrictPropertyInit emits TS2564 for class properties that have a type
// annotation but no initializer, no definite-assignment assertion, are not
// optional, and are never assigned via `this.<name> = ...` inside the
// constructor body.
//
// Skipped wholesale for ambient (`declare class`) and abstract classes.
// Per-property abstract is not tracked by the parser today, so abstract classes
// are treated permissively rather than risk false positives.
func (c *Checker) checkStrictPropertyInit(node *parser.ClassDeclaration) {
	if c.skipStrictPropertyInit {
		return
	}
	if node == nil || node.Body == nil {
		return
	}
	// A class in an ambient namespace or module, or in a declaration file,
	// is ambient too (@types/node's `declare module "assert" { class
	// AssertionError { actual: unknown; } }`).
	if node.Declare || node.IsAbstract || c.inAmbientNamespace || c.inDeclarationFile() {
		return
	}

	assigned := collectStrictInitAssignments(node.Body)
	// A `this` property type is the polymorphic this, which needs an
	// initializer like any other non-undefined type.
	savedThis := c.currentThisType
	if savedThis == nil {
		c.currentThisType = &types.TypeParameterType{Parameter: types.PolymorphicThis}
	}
	defer func() { c.currentThisType = savedThis }()

	for _, prop := range node.Body.Properties {
		if !shouldEmitStrictInit(prop) {
			continue
		}
		name, ok := strictInitPropertyName(prop.Key)
		if !ok {
			continue
		}
		if assigned[name] {
			continue
		}
		propType := c.resolveTypeAnnotation(prop.TypeAnnotation)
		if propType == nil || strictInitTypePermits(propType) {
			continue
		}
		var at parser.Node = prop.Key
		if _, computed := prop.Key.(*parser.ComputedPropertyName); computed && prop.Token != nil && prop.Token.Literal == "[" {
			at = &parser.Identifier{Token: prop.Token, Value: "["}
		}
		c.addErrorWithCode(at, errors.TS2564, fmt.Sprintf(
			"Property '%s' has no initializer and is not definitely assigned in the constructor.",
			name,
		))
	}
}

func shouldEmitStrictInit(prop *parser.PropertyDefinition) bool {
	if prop == nil {
		return false
	}
	if prop.Value != nil {
		return false
	}
	if prop.Optional || prop.IsStatic || prop.DefiniteAssignment || prop.IsDeclare {
		return false
	}
	if prop.TypeAnnotation == nil {
		return false
	}
	return true
}

// strictInitPropertyName names the properties tsc checks: identifiers and
// computed names, not string or numeric literal names.
func strictInitPropertyName(key parser.Expression) (string, bool) {
	switch k := key.(type) {
	case *parser.Identifier:
		if k.Token != nil && (k.Token.Type == lexer.NUMBER || k.Token.Type == lexer.STRING || k.Token.Type == lexer.BIGINT) {
			return "", false
		}
		return k.Value, true
	case *parser.ComputedPropertyName:
		if k.Expr != nil {
			text := entityNameText(k.Expr)
			if text == "" {
				text = k.Expr.String()
			}
			return "[" + text + "]", true
		}
	}
	return "", false
}

// strictInitTypePermits returns true when the property's declared type is one
// that doesn't require an explicit initializer. Matches TS semantics: `any`
// (and unions containing it) plus types that explicitly include `undefined`.
// `unknown` and `null` still require initialization.
func strictInitTypePermits(t types.Type) bool {
	if t == nil {
		return true
	}
	if t == types.Any || t == types.Undefined || t == types.Void {
		return true
	}
	if u, ok := t.(*types.UnionType); ok {
		for _, m := range u.Types {
			if strictInitTypePermits(m) {
				return true
			}
		}
	}
	return false
}

// collectStrictInitAssignments returns the set of instance-property names that
// are unambiguously initialized by the constructor — either via `this.<name> =`
// somewhere in the body, or via a parameter-property modifier (which the
// runtime auto-assigns).
func collectStrictInitAssignments(body *parser.ClassBody) map[string]bool {
	found := map[string]bool{}
	if body == nil {
		return found
	}
	for _, method := range body.Methods {
		if method == nil || method.Kind != "constructor" || method.Value == nil {
			continue
		}
		for _, param := range method.Value.Parameters {
			if param == nil || param.Name == nil {
				continue
			}
			if param.IsPublic || param.IsPrivate || param.IsProtected || param.IsReadonly {
				found[param.Name.Value] = true
			}
		}
		if method.Value.Body != nil {
			walkForThisAssignments(method.Value.Body, found)
		}
	}
	return found
}

// walkForThisAssignments traverses a node looking for `this.<name> = ...`
// assignments (including compound forms like `+=`). It covers the common
// statement / expression containers but isn't exhaustive — anything it doesn't
// recognize is skipped. TS2564 is an "any-path" approximation, so a missed
// shape produces a false positive that the developer can suppress with `!`.
func walkForThisAssignments(node parser.Node, found map[string]bool) {
	if node == nil {
		return
	}
	// Guard against typed-nil interface values — fields declared as Statement/Expression
	// can carry a (*ConcreteType)(nil), which is non-nil at the interface level but
	// panics on field access.
	if v := reflect.ValueOf(node); v.Kind() == reflect.Ptr && v.IsNil() {
		return
	}
	switch n := node.(type) {
	case *parser.BlockStatement:
		for _, s := range n.Statements {
			walkForThisAssignments(s, found)
		}
	case *parser.ExpressionStatement:
		walkForThisAssignments(n.Expression, found)
	case *parser.IfStatement:
		walkForThisAssignments(n.Condition, found)
		walkForThisAssignments(n.Consequence, found)
		walkForThisAssignments(n.Alternative, found)
	case *parser.WhileStatement:
		walkForThisAssignments(n.Condition, found)
		walkForThisAssignments(n.Body, found)
	case *parser.DoWhileStatement:
		walkForThisAssignments(n.Condition, found)
		walkForThisAssignments(n.Body, found)
	case *parser.ForStatement:
		walkForThisAssignments(n.Initializer, found)
		walkForThisAssignments(n.Condition, found)
		walkForThisAssignments(n.Update, found)
		walkForThisAssignments(n.Body, found)
	case *parser.ForOfStatement:
		walkForThisAssignments(n.Variable, found)
		walkForThisAssignments(n.Iterable, found)
		walkForThisAssignments(n.Body, found)
	case *parser.ForInStatement:
		walkForThisAssignments(n.Variable, found)
		walkForThisAssignments(n.Object, found)
		walkForThisAssignments(n.Body, found)
	case *parser.TryStatement:
		walkForThisAssignments(n.Body, found)
		if n.CatchClause != nil {
			walkForThisAssignments(n.CatchClause.Body, found)
		}
		walkForThisAssignments(n.FinallyBlock, found)
	case *parser.SwitchStatement:
		walkForThisAssignments(n.Expression, found)
		for _, sc := range n.Cases {
			if sc == nil {
				continue
			}
			walkForThisAssignments(sc.Condition, found)
			walkForThisAssignments(sc.Body, found)
		}
	case *parser.ReturnStatement:
		walkForThisAssignments(n.ReturnValue, found)
	case *parser.ThrowStatement:
		walkForThisAssignments(n.Value, found)
	case *parser.LabeledStatement:
		walkForThisAssignments(n.Statement, found)
	case *parser.AssignmentExpression:
		if me, ok := n.Left.(*parser.MemberExpression); ok {
			if _, isThis := me.Object.(*parser.ThisExpression); isThis {
				if id, ok := me.Property.(*parser.Identifier); ok {
					found[id.Value] = true
				}
			}
		}
		walkForThisAssignments(n.Left, found)
		walkForThisAssignments(n.Value, found)
	}
}

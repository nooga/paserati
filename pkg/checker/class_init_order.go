package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// TS2729 — "Property 'x' is used before its initialization."
//
// tsc's checkPropertyNotUsedBeforeDeclaration: inside a property initializer
// or a static block (not inside a nested function, nor an arrow function), a
// `this.x` or `C.x` read of a field of the same class and staticness is an
// error when that field is initialized later in source order, or is the
// field being initialized. A field declared earlier without an initializer
// (nor `!`) still counts as uninitialized when read through `this`.
// Constructor parameter properties are initialized after all fields when
// fields are emitted as standard ES2022 fields, and a field redeclared from a
// base class only counts as uninitialized under useDefineForClassFields.
// Auto-accessors (`accessor x`) are not property declarations.

type fieldDecl struct {
	pos         int
	hasInit     bool
	definite    bool
	optional    bool
	paramProp   bool
	declaration *parser.PropertyDefinition
}

func (c *Checker) checkClassInitializationOrder(className string, body *parser.ClassBody, instanceType *types.ObjectType) {
	if body == nil {
		return
	}
	// synthesizeParameterProperties appends parameter properties to
	// body.Properties; those are handled below instead.
	paramTokens := map[any]bool{}
	for _, m := range body.Methods {
		if m != nil && m.Kind == "constructor" && m.Value != nil {
			for _, prm := range m.Value.Parameters {
				if prm != nil && prm.Token != nil {
					paramTokens[prm.Token] = true
				}
			}
		}
	}
	instanceFields := map[string]fieldDecl{}
	staticFields := map[string]fieldDecl{}
	for _, p := range body.Properties {
		id, ok := p.Key.(*parser.Identifier)
		if !ok || p.Token == nil || p.IsDeclare || p.IsAccessor || paramTokens[p.Token] {
			continue
		}
		d := fieldDecl{pos: p.Token.StartPos, hasInit: p.Value != nil, definite: p.DefiniteAssignment, optional: p.Optional, declaration: p}
		if p.IsStatic {
			staticFields[id.Value] = d
		} else {
			instanceFields[id.Value] = d
		}
	}
	if !c.noStandardClassFields {
		for _, m := range body.Methods {
			if m == nil || m.Kind != "constructor" || m.Value == nil {
				continue
			}
			for _, prm := range m.Value.Parameters {
				if prm != nil && prm.Name != nil && (prm.IsPublic || prm.IsPrivate || prm.IsProtected || prm.IsReadonly) {
					if _, dup := instanceFields[prm.Name.Value]; !dup {
						instanceFields[prm.Name.Value] = fieldDecl{pos: 1 << 62, paramProp: true}
					}
				}
			}
		}
	}

	check := func(init parser.Node, static bool, owner *parser.PropertyDefinition) {
		forEachInitializerAccess(init, func(m *parser.MemberExpression) {
			prop, ok := m.Property.(*parser.Identifier)
			if !ok || prop.Token == nil {
				return
			}
			var fields map[string]fieldDecl
			switch obj := m.Object.(type) {
			case *parser.ThisExpression:
				fields = instanceFields
				if static {
					fields = staticFields
				}
			case *parser.Identifier:
				if className == "" || obj.Value != className {
					return
				}
				fields = staticFields
			default:
				return
			}
			d, ok := fields[prop.Value]
			if !ok || d.optional {
				return
			}
			usage := prop.Token.StartPos
			_, viaThis := m.Object.(*parser.ThisExpression)
			uninitialized := viaThis && !d.hasInit && !d.definite && !d.paramProp
			declaredBefore := d.pos <= usage && !uninitialized
			if d.pos <= usage && uninitialized {
				// A static block runs after the fields declared before it; a
				// static initializer may rely on a static block in between
				// having assigned the field (isPropertyInitializedInStaticBlocks).
				declaredBefore = owner == nil ||
					(static && assignedInStaticBlocks(body.StaticInitializers, prop.Value, d.pos, owner.Token.StartPos))
			}
			if declaredBefore && d.declaration != owner {
				return
			}
			if c.noDefineForClassFields && !static && instanceType != nil && declaredInBase(instanceType, prop.Value) {
				return
			}
			c.addErrorWithCode(prop, errors.TS2729, fmt.Sprintf("Property '%s' is used before its initialization.", prop.Value))
		})
	}
	for _, p := range body.Properties {
		if p != nil && p.Value != nil {
			check(p.Value, p.IsStatic, p)
		}
	}
	for _, b := range body.StaticInitializers {
		check(b, true, nil)
	}
}

// assignedInStaticBlocks reports whether a static block between from and to
// assigns this.name at its top level.
func assignedInStaticBlocks(blocks []*parser.BlockStatement, name string, from, to int) bool {
	for _, b := range blocks {
		if b == nil || b.Token == nil || b.Token.StartPos < from || b.Token.StartPos > to {
			continue
		}
		for _, st := range b.Statements {
			es, ok := st.(*parser.ExpressionStatement)
			if !ok {
				continue
			}
			as, ok := es.Expression.(*parser.AssignmentExpression)
			if !ok || as.Operator != "=" {
				continue
			}
			if m, ok := as.Left.(*parser.MemberExpression); ok {
				if _, isThis := m.Object.(*parser.ThisExpression); isThis {
					if id, ok := m.Property.(*parser.Identifier); ok && id.Value == name {
						return true
					}
				}
			}
		}
	}
	return false
}

// declaredInBase reports whether a base class of t declares the property.
func declaredInBase(t *types.ObjectType, name string) bool {
	for _, base := range t.Bases() {
		if bo, ok := base.(*types.ObjectType); ok {
			if _, has := bo.GetEffectiveProperties()[name]; has {
				return true
			}
		}
	}
	return false
}

// forEachInitializerAccess calls fn for each member access in an initializer
// that runs as part of it: nested functions and arrow functions are skipped
// (isInPropertyInitializerOrClassStaticBlock quits there), as are classes.
// For `a.b.c` only the innermost access whose object is not itself an access
// is considered.
func forEachInitializerAccess(node parser.Node, fn func(*parser.MemberExpression)) {
	if isNilNode(node) {
		return
	}
	switch n := node.(type) {
	case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral, *parser.ShorthandMethod,
		*parser.ClassExpression, *parser.ClassDeclaration:
		return
	case *parser.MemberExpression:
		switch n.Object.(type) {
		case *parser.MemberExpression, *parser.IndexExpression:
		default:
			fn(n)
		}
	}
	forEachChildInScope(node, func(child parser.Node) {
		forEachInitializerAccess(child, fn)
	})
}

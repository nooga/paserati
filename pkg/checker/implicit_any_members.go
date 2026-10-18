package checker

import (
	"fmt"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// Under noImplicitAny a property declared with neither a type nor an
// initializer is any, which tsc reports as TS7008 on its name. A private
// member of an ambient class is exempt (its type is not part of the
// declaration's surface).

func (c *Checker) reportImplicitAnyMember(nameNode parser.Node, name string) {
	if c.implicitAnyMembers == nil {
		c.implicitAnyMembers = make(map[parser.Node]bool)
	}
	if c.implicitAnyMembers[nameNode] {
		return
	}
	c.implicitAnyMembers[nameNode] = true
	c.addErrorWithCode(nameNode, errors.TS7008, fmt.Sprintf("Member '%s' implicitly has an 'any' type.", name))
}

// reportImplicitAnyReturn reports TS7010 for a declaration with no body and
// no return type annotation: its return type is any.
func (c *Checker) reportImplicitAnyReturn(nameNode *parser.Identifier, name string) {
	if !c.noImplicitAny || c.inDeclarationFile() || nameNode == nil {
		return
	}
	if c.implicitAnyMembers == nil {
		c.implicitAnyMembers = make(map[parser.Node]bool)
	}
	if c.implicitAnyMembers[nameNode] {
		return
	}
	c.implicitAnyMembers[nameNode] = true
	c.addErrorWithCode(nameNode, errors.TS7010, fmt.Sprintf("'%s', which lacks return-type annotation, implicitly has an 'any' return type.", name))
}

func (c *Checker) checkImplicitAnyClassMembers(node *parser.ClassDeclaration) {
	if !c.noImplicitAny || node == nil || node.Body == nil || c.inDeclarationFile() {
		return
	}
	c.checkCircularClassFields(node.Body)
	ambient := node.Declare || c.inAmbientNamespace
	// Parameter properties are typed by their parameter.
	paramTokens := map[*lexer.Token]bool{}
	for _, m := range node.Body.Methods {
		if m != nil && m.Kind == "constructor" && m.Value != nil {
			for _, p := range m.Value.Parameters {
				if p != nil && p.Name != nil {
					paramTokens[p.Name.Token] = true
				}
			}
		}
	}
	// A property assigned in the constructor (or, if static, in a static
	// block) is typed by those assignments.
	assignedInstance := collectStrictInitAssignments(node.Body)
	assignedStatic := map[string]bool{}
	for _, block := range node.Body.StaticInitializers {
		walkForThisAssignments(block, assignedStatic)
	}
	for _, m := range node.Body.MethodSigs {
		if m == nil || m.ReturnTypeAnnotation != nil || m.Kind != "method" || m.HasBody {
			continue
		}
		if id, ok := m.Key.(*parser.Identifier); ok && !(ambient && (m.IsPrivate || strings.HasPrefix(id.Value, "#"))) {
			c.reportImplicitAnyReturn(id, id.Value)
		}
	}
	for _, prop := range node.Body.Properties {
		if prop == nil || prop.TypeAnnotation != nil || prop.Value != nil || prop.IsAccessor {
			continue
		}
		id, ok := prop.Key.(*parser.Identifier)
		if !ok || paramTokens[id.Token] {
			continue
		}
		if (prop.IsStatic && assignedStatic[id.Value]) || (!prop.IsStatic && assignedInstance[id.Value]) {
			continue
		}
		if id.Token != nil && (id.Token.Type == lexer.NUMBER || id.Token.Type == lexer.STRING || id.Token.Type == lexer.BIGINT) {
			continue
		}
		if ambient && (prop.IsPrivate || strings.HasPrefix(id.Value, "#")) {
			continue
		}
		if !prop.IsStatic && c.baseClassHasProperty(node, id.Value) {
			continue // typed like the base class property it redeclares
		}
		c.reportImplicitAnyMember(id, id.Value)
	}
}

// baseClassHasProperty reports whether the instance type of node's base
// class has a property name.
func (c *Checker) baseClassHasProperty(node *parser.ClassDeclaration, name string) bool {
	if node.SuperClass == nil || node.Name == nil {
		return false
	}
	t, ok := c.env.ResolveType(node.Name.Value)
	if !ok {
		return true // unknown: do not guess
	}
	obj, isObj := t.(*types.ObjectType)
	if !isObj {
		return true
	}
	for _, base := range obj.Bases() {
		if b, ok := base.(*types.ObjectType); ok {
			if _, has := b.EffectiveProperty(name); has {
				return true
			}
		} else {
			return true
		}
	}
	return false
}

func (c *Checker) checkImplicitAnyInterfaceMembers(props []*parser.InterfaceProperty) {
	if !c.noImplicitAny || c.inDeclarationFile() {
		return
	}
	accessorMarker := false // the previous member was a split-off get/set
	for _, p := range props {
		afterMarker := accessorMarker
		accessorMarker = p != nil && p.Type == nil && !p.IsMethod && p.Name != nil && p.Name.Token != nil &&
			(p.Name.Token.Type == lexer.GET || p.Name.Token.Type == lexer.SET)
		if p != nil && p.IsMethod && p.Name != nil {
			if fn, ok := p.Type.(*parser.FunctionTypeExpression); ok && fn.ReturnType == nil && !afterMarker {
				c.reportImplicitAnyReturn(p.Name, p.Name.Value)
			}
			continue
		}
		if p == nil || p.Type != nil || p.IsMethod || p.IsIndexSignature || p.IsConstructorSignature || p.IsComputedProperty || p.Name == nil {
			continue
		}
		if t := p.Name.Token; t != nil && (t.Type == lexer.GET || t.Type == lexer.SET) {
			continue // an accessor signature the parser splits up
		}
		c.reportImplicitAnyMember(p.Name, p.Name.Value)
	}
}

func (c *Checker) checkImplicitAnyTypeLiteralMembers(props []*parser.ObjectTypeProperty) {
	if !c.noImplicitAny || c.inDeclarationFile() {
		return
	}
	for _, p := range props {
		if p == nil || p.Type != nil || p.IsCallSignature || p.IsConstructSignature || p.IsIndexSignature || p.Name == nil {
			continue
		}
		c.reportImplicitAnyMember(p.Name, p.Name.Value)
	}
}

package checker

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// JSX type checking. An element is checked as tsc resolves it: as a call of
// its component with one argument, the attributes object (the attributes,
// plus `children` from the element's body). The checker builds that call once
// per element (JSXElement.Checked) and checks it like any other, so
// attribute values are contextually typed, generic components are inferred
// and overloads resolve as for calls. The types come from the JSX namespace:
// React.JSX (the namespace of the default factory React.createElement) or the
// global JSX.

// jsxNamespace is the JSX namespace's declarations, looked up once per
// checker (per file).
type jsxNamespace struct {
	element             types.Type // JSX.Element
	intrinsicElements   types.Type
	intrinsicAttributes types.Type
	intrinsicClassAttrs *types.GenericType
	propsName           string // ElementAttributesProperty's member ("props")
	childrenName        string // ElementChildrenAttribute's member ("children")
}

func (c *Checker) jsxNS() *jsxNamespace {
	if c.jsx != nil {
		return c.jsx
	}
	ns := &jsxNamespace{childrenName: "children"}
	root := []string{"React", "JSX"}
	if c.jsxLookup(root, "Element") == nil {
		root = []string{"JSX"}
	}
	ns.element = c.jsxLookup(root, "Element")
	ns.intrinsicElements = c.jsxLookup(root, "IntrinsicElements")
	ns.intrinsicAttributes = c.jsxLookup(root, "IntrinsicAttributes")
	if g, ok := c.jsxLookupRaw(root, "IntrinsicClassAttributes").(*types.GenericType); ok {
		ns.intrinsicClassAttrs = g
	}
	if name, ok := soleMemberName(c.jsxLookup(root, "ElementAttributesProperty")); ok {
		ns.propsName = name
	}
	if t := c.jsxLookup(root, "ElementChildrenAttribute"); t != nil {
		if name, ok := soleMemberName(t); ok {
			ns.childrenName = name
		}
	}
	c.jsx = ns
	return ns
}

// jsxLookup resolves the type path.name (React.JSX.Element) without reporting
// anything; nil if it does not exist.
func (c *Checker) jsxLookup(path []string, name string) types.Type {
	t := c.jsxLookupRaw(path, name)
	if g, ok := t.(*types.GenericType); ok {
		return c.instantiateGenericType(g, nil, nil)
	}
	return t
}

func (c *Checker) jsxLookupRaw(path []string, name string) types.Type {
	tok := &lexer.Token{Type: lexer.IDENT, StartPos: -1, EndPos: -1}
	var expr parser.Expression = &parser.Identifier{Token: tok, Value: path[0]}
	for _, part := range append(path[1:], name) {
		expr = &parser.MemberExpression{Token: tok, Object: expr, Property: &parser.Identifier{Token: tok, Value: part}}
	}
	nErrs := len(c.errors)
	t := c.resolveEnumMemberTypeExpression(expr.(*parser.MemberExpression))
	if len(c.errors) > nErrs {
		c.errors = c.errors[:nErrs]
		return nil
	}
	return t
}

// soleMemberName is the name of the only property of t (ElementAttributesProperty
// and ElementChildrenAttribute name the props and children members so).
func soleMemberName(t types.Type) (string, bool) {
	obj, ok := types.GetEffectiveType(t).(*types.ObjectType)
	if !ok {
		return "", false
	}
	names := obj.PropertyNames()
	if len(names) != 1 {
		return "", false
	}
	return names[0], true
}

// isIntrinsicJSXTag reports whether a tag names an intrinsic element (`div`,
// `my-element`, `svg:rect`) rather than a component.
func isIntrinsicJSXTag(name parser.Expression) bool {
	switch n := name.(type) {
	case *parser.JSXNamespacedName:
		return true
	case *parser.Identifier:
		if strings.Contains(n.Value, "-") {
			return true
		}
		for _, r := range n.Value {
			return unicode.IsLower(r)
		}
	}
	return false
}

func (c *Checker) checkJSXElement(node *parser.JSXElement) {
	ns := c.jsxNS()
	result := ns.element
	if result == nil {
		result = types.Any
	}
	if node.IsFragment() {
		for _, child := range node.Children {
			if e := jsxChildExpression(child); e != nil {
				c.visit(e)
			}
		}
		node.SetComputedType(result)
		return
	}
	if node.Checked == nil {
		node.Checked = c.jsxCall(node, ns)
	}
	if node.Checked == nil {
		// The tag is `any` or has no signatures: check the attributes alone.
		for _, attr := range node.Attributes {
			if attr.Spread != nil {
				c.visit(attr.Spread)
			} else if v := jsxAttributeValue(attr); v != nil {
				c.visit(v)
			}
		}
		for _, child := range node.Children {
			if e := jsxChildExpression(child); e != nil {
				c.visit(e)
			}
		}
		node.SetComputedType(result)
		return
	}
	c.visit(node.Checked)
	for _, attr := range node.Attributes {
		if attr.Spread == nil && strings.Contains(attr.Name, "-") {
			if v := jsxAttributeValue(attr); v != nil {
				c.visit(v)
			}
		}
	}
	node.SetComputedType(result)
}

// jsxCall builds the call node is checked as, or returns nil when there is
// nothing to call (an `any` tag, a tag without signatures: reported here).
func (c *Checker) jsxCall(node *parser.JSXElement, ns *jsxNamespace) parser.Expression {
	ret := ns.element
	if ret == nil {
		ret = types.Any
	}
	var sigs []*types.Signature
	if isIntrinsicJSXTag(node.TagName) {
		if ns.intrinsicElements == nil {
			return nil
		}
		name := parserTagText(node.TagName)
		props := c.memberType(ns.intrinsicElements, name)
		if props == nil {
			c.addErrorWithCode(node.TagName, "TS2339", fmt.Sprintf("Property '%s' does not exist on type 'JSX.IntrinsicElements'.", name))
			return nil
		}
		if props == types.Any {
			return nil
		}
		sigs = []*types.Signature{jsxSignature(nil, c.jsxWithIntrinsicAttributes(props, ns), ret)}
	} else {
		c.visit(node.TagName)
		tagType := node.TagName.GetComputedType()
		if tagType == nil || tagType == types.Any {
			return nil
		}
		sigs = c.jsxSignatures(tagType, ns, ret)
		if len(sigs) == 0 {
			c.addErrorWithCode(node.TagName, "TS2604", fmt.Sprintf("JSX element type '%s' does not have any construct or call signatures.", parserTagText(node.TagName)))
			return nil
		}
	}
	callee := types.NewObjectType()
	for _, sig := range sigs {
		callee.AddCallSignature(sig)
	}
	tag := &parser.JSXTagType{Token: node.Token}
	tag.SetComputedType(callee)
	attrs := c.jsxAttributesObject(node, ns)
	if c.jsxAttributes == nil {
		c.jsxAttributes = map[parser.Expression]parser.Node{}
	}
	c.jsxAttributes[attrs] = node.TagName
	return &parser.CallExpression{
		Token:         node.Token,
		Function:      tag,
		TypeArguments: node.TypeArguments,
		Arguments:     []parser.Expression{attrs},
	}
}

// jsxSignature is the signature `<typeParams>(props: P) => ret`.
func jsxSignature(typeParams []*types.TypeParameter, props, ret types.Type) *types.Signature {
	return &types.Signature{
		TypeParameters: typeParams,
		ParameterNames: []string{"props"},
		ParameterTypes: []types.Type{props},
		OptionalParams: []bool{false},
		ReturnType:     ret,
		StrictVariance: true,
	}
}

func (c *Checker) jsxWithIntrinsicAttributes(props types.Type, ns *jsxNamespace) types.Type {
	if ns.intrinsicAttributes == nil {
		return props
	}
	return types.NewIntersectionType(props, ns.intrinsicAttributes)
}

// jsxSignatures are the signatures a component of type t is called with:
// a function component's, taking its props (and the intrinsic attributes),
// and a class component's, taking its instance's props member.
func (c *Checker) jsxSignatures(t types.Type, ns *jsxNamespace, ret types.Type) []*types.Signature {
	t = types.GetEffectiveType(t)
	if u, ok := t.(*types.UnionType); ok {
		// A union of components (React.ComponentType<P>) takes props every
		// member accepts.
		var props []types.Type
		for _, m := range u.Types {
			ms := c.jsxSignatures(m, ns, ret)
			if len(ms) == 0 {
				return nil
			}
			var alts []types.Type
			for _, s := range ms {
				alts = append(alts, s.ParameterTypes[0])
			}
			props = append(props, types.NewUnionType(alts...))
		}
		return []*types.Signature{jsxSignature(nil, types.NewIntersectionType(props...), ret)}
	}
	call, construct := c.componentSignatures(t)
	var out []*types.Signature
	for _, sig := range call {
		var props types.Type = types.NewObjectType()
		if len(sig.ParameterTypes) > 0 {
			props = sig.ParameterTypes[0]
		} else if sig.RestParameterType != nil {
			props = types.Any
		}
		out = append(out, jsxSignature(sig.TypeParameters, c.jsxWithIntrinsicAttributes(props, ns), ret))
	}
	if len(out) > 0 {
		return out
	}
	for _, sig := range construct {
		instance := sig.ReturnType
		var props types.Type = types.Any
		if ns.propsName != "" {
			props = c.memberType(instance, ns.propsName)
			if props == nil {
				props = types.NewObjectType()
			}
		} else if len(sig.ParameterTypes) > 0 {
			props = sig.ParameterTypes[0]
		}
		props = c.jsxWithIntrinsicAttributes(props, ns)
		if ns.intrinsicClassAttrs != nil {
			props = types.NewIntersectionType(props, c.instantiateGenericType(ns.intrinsicClassAttrs, []types.Type{instance}, nil))
		}
		out = append(out, jsxSignature(sig.TypeParameters, props, ret))
	}
	return out
}

// memberType is the type of t's member name, inherited ones included; nil
// if it has none.
func (c *Checker) memberType(t types.Type, name string) types.Type {
	if obj, ok := types.GetEffectiveType(t).(*types.ObjectType); ok {
		if mt, found := c.resolveObjectMemberForDestructuring(obj, name); found {
			return mt
		}
		return nil
	}
	if mt := c.getPropertyTypeFromType(t, name, false); mt != nil && mt != types.Never {
		return mt
	}
	return nil
}

// componentSignatures are the call and construct signatures of a component
// type.
func (c *Checker) componentSignatures(t types.Type) (call, construct []*types.Signature) {
	switch t := t.(type) {
	case *types.ObjectType:
		return t.CallSigs(), t.ConstructSigs()
	case *types.GenericType:
		return c.extractCallSignaturesFromType(t), c.extractConstructSignaturesFromType(t)
	case *types.IntersectionType:
		for _, m := range t.Types {
			mc, mk := c.componentSignatures(types.GetEffectiveType(m))
			call = append(call, mc...)
			construct = append(construct, mk...)
		}
	}
	return call, construct
}

// jsxAttributesObject builds the attributes object literal of an element:
// its attributes in order and, when it has children, the children property.
func (c *Checker) jsxAttributesObject(node *parser.JSXElement, ns *jsxNamespace) *parser.ObjectLiteral {
	obj := &parser.ObjectLiteral{Token: node.Token}
	for _, attr := range node.Attributes {
		if attr.Spread != nil {
			obj.Properties = append(obj.Properties, &parser.ObjectProperty{Key: &parser.SpreadElement{Token: attr.Token, Argument: attr.Spread}})
			continue
		}
		v := jsxAttributeValue(attr)
		if v == nil || strings.Contains(attr.Name, "-") {
			// tsc does not check hyphenated names (data-*, aria-*) against
			// the props type; their values are checked on their own.
			continue
		}
		obj.Properties = append(obj.Properties, &parser.ObjectProperty{Key: jsxPropertyKey(attr.Token, attr.Name), Value: v})
	}
	var children []parser.Expression
	for _, child := range node.Children {
		if container, ok := child.(*parser.JSXExpressionContainer); ok && container.Spread && container.Expression != nil {
			children = append(children, &parser.SpreadElement{Token: container.Token, Argument: container.Expression})
			continue
		}
		if e := jsxChildExpression(child); e != nil {
			children = append(children, e)
		}
	}
	if len(children) > 0 && ns.childrenName != "" {
		var value parser.Expression
		if len(children) == 1 {
			if _, spread := children[0].(*parser.SpreadElement); !spread {
				value = children[0]
			}
		}
		if value == nil {
			value = &parser.ArrayLiteral{Token: node.Token, Elements: children}
		}
		obj.Properties = append(obj.Properties, &parser.ObjectProperty{Key: jsxPropertyKey(node.Token, ns.childrenName), Value: value})
	}
	return obj
}

// jsxAttributeValue is the expression an attribute's value is: `true` for a
// bare name, nil for an empty `{}`.
func jsxAttributeValue(attr *parser.JSXAttribute) parser.Expression {
	switch v := attr.Value.(type) {
	case nil:
		return &parser.BooleanLiteral{Token: attr.Token, Value: true}
	case *parser.JSXExpressionContainer:
		return v.Expression
	}
	return attr.Value
}

// jsxChildExpression is the expression a child contributes to `children`, nil
// for whitespace-only text spanning lines and empty containers.
func jsxChildExpression(child parser.Expression) parser.Expression {
	switch ch := child.(type) {
	case *parser.JSXText:
		text := jsxTrimText(ch.Value)
		if text == "" {
			return nil
		}
		return &parser.StringLiteral{Token: ch.Token, Value: text}
	case *parser.JSXExpressionContainer:
		return ch.Expression
	}
	return child
}

// jsxTrimText trims JSX text as React does: lines are trimmed and joined
// with spaces, and lines left empty dropped.
func jsxTrimText(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	var parts []string
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' }) {
		if line = strings.TrimSpace(line); line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, " ")
}

func jsxPropertyKey(tok *lexer.Token, name string) parser.Expression {
	if isPlainIdentifier(name) {
		return &parser.Identifier{Token: tok, Value: name}
	}
	return &parser.StringLiteral{Token: tok, Value: name}
}

func isPlainIdentifier(name string) bool {
	for i, r := range name {
		if r == '_' || r == '$' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r)) {
			continue
		}
		return false
	}
	return name != ""
}

// parserTagText is a tag name as written.
func parserTagText(n parser.Expression) string {
	switch n := n.(type) {
	case *parser.Identifier:
		return n.Value
	case *parser.ThisExpression:
		return "this"
	case *parser.JSXNamespacedName:
		return n.Namespace + ":" + n.Name
	case *parser.MemberExpression:
		if prop, ok := n.Property.(*parser.Identifier); ok {
			return parserTagText(n.Object) + "." + prop.Value
		}
	}
	return ""
}

package checker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// This file reports assignability failures the way TypeScript does
// (checkTypeAssignableToAndOptionallyElaborate / elaborateError): an object
// literal, array literal or arrow function that is not assignable is
// reported on the offending property, element or return expression; a fresh
// object literal with a property the target lacks is TS2353; a target with
// required properties the source lacks is TS2741/TS2739/TS2740; a weak target
// (all optional properties) with no property in common is TS2559.

// relHead identifies the head diagnostic of an assignability failure.
type relHead int

const (
	headAssign    relHead = iota // Type 'X' is not assignable to type 'Y' (TS2322)
	headArgument                 // Argument of type 'X' is not assignable to parameter of type 'Y' (TS2345)
	headSatisfies                // Type 'X' does not satisfy the expected type 'Y' (TS1360)
)

// leftmostToken returns the token a node's source text starts with.
func leftmostToken(node parser.Node) *lexer.Token {
	switch n := node.(type) {
	case *parser.MemberExpression:
		if n.Object != nil {
			return leftmostToken(n.Object)
		}
	case *parser.OptionalChainingExpression:
		if n.Object != nil {
			return leftmostToken(n.Object)
		}
	case *parser.IndexExpression:
		if n.Left != nil {
			return leftmostToken(n.Left)
		}
	case *parser.CallExpression:
		if n.Function != nil {
			return leftmostToken(n.Function)
		}
	case *parser.InfixExpression:
		if n.Left != nil {
			return leftmostToken(n.Left)
		}
	case *parser.TernaryExpression:
		if n.Condition != nil {
			return leftmostToken(n.Condition)
		}
	case *parser.AssignmentExpression:
		if n.Left != nil {
			return leftmostToken(n.Left)
		}
	case *parser.TypeAssertionExpression:
		if n.Expression != nil {
			return leftmostToken(n.Expression)
		}
	case *parser.NonNullExpression:
		if n.Expression != nil {
			return leftmostToken(n.Expression)
		}
	case *parser.UpdateExpression:
		if !n.Prefix && n.Argument != nil {
			return leftmostToken(n.Argument)
		}
	case *parser.ArrowFunctionLiteral:
		if len(n.Parameters) > 0 && n.Parameters[0] != nil && n.Parameters[0].Token != nil {
			return n.Parameters[0].Token
		}
	case *parser.ExpressionStatement:
		if n.Expression != nil {
			return leftmostToken(n.Expression)
		}
	}
	return parser.GetTokenFromNode(node)
}

// addErrorAtStart records a diagnostic positioned at the start of a node.
func (c *Checker) addErrorAtStart(node parser.Node, code string, message string) {
	token := leftmostToken(node)
	if token == nil {
		token = parser.GetTokenFromNode(node)
	}
	// Declarations and expressions can be checked in more than one pass; a
	// diagnostic is reported once per position, code and message.
	key := fmt.Sprintf("%d:%s:%s", token.StartPos, code, message)
	if c.reportedRelationDiagnostics == nil {
		c.reportedRelationDiagnostics = make(map[string]bool)
	}
	if c.reportedRelationDiagnostics[key] {
		return
	}
	c.reportedRelationDiagnostics[key] = true
	c.errors = append(c.errors, &errors.TypeError{
		Position: errors.Position{
			Line:     token.Line,
			Column:   token.Column,
			StartPos: token.StartPos,
			EndPos:   token.EndPos,
			Source:   c.source,
		},
		Msg:       message,
		ErrorCode: code,
	})
}

// resolveStructural unwraps instantiated generics and mapped types so object
// structure can be inspected.
func (c *Checker) resolveStructural(t types.Type) types.Type {
	for i := 0; i < 8 && t != nil; i++ {
		switch tt := t.(type) {
		case *types.InstantiatedType:
			sub := tt.Substitute()
			if sub == nil || sub == t {
				return t
			}
			t = sub
			continue
		case *types.MappedType:
			expanded := c.expandIfMappedType(t)
			if expanded == nil || expanded == t {
				return t
			}
			t = expanded
			continue
		}
		break
	}
	return t
}

// targetObjects lists the object types a literal is being related to: the
// target itself, or the object members of a union/intersection target. It
// returns nil when the target has no inspectable object structure (any,
// primitives, type parameters, ...), in which case no per-property
// diagnostics are produced.
func (c *Checker) targetObjects(target types.Type) []*types.ObjectType {
	target = c.resolveStructural(target)
	switch t := target.(type) {
	case *types.ObjectType:
		return []*types.ObjectType{t}
	case *types.UnionType:
		var out []*types.ObjectType
		for _, m := range t.Types {
			m = c.resolveStructural(m)
			switch mm := m.(type) {
			case *types.ObjectType:
				out = append(out, mm)
			case *types.ArrayType, *types.TupleType, *types.TypeParameterType:
				return nil
			default:
				if m == types.Any || m == types.Unknown || m == types.NonPrimitive {
					return nil
				}
			}
		}
		return out
	case *types.IntersectionType:
		var out []*types.ObjectType
		for _, m := range t.Types {
			m = c.resolveStructural(m)
			mm, ok := m.(*types.ObjectType)
			if !ok {
				return nil
			}
			out = append(out, mm)
		}
		return out
	}
	return nil
}

// objectKnowsProperty reports whether a property name is declared by the
// object type or admitted by one of its index signatures (isKnownProperty).
func objectKnowsProperty(obj *types.ObjectType, name string) bool {
	if _, ok := obj.GetEffectiveProperties()[name]; ok {
		return true
	}
	for _, idx := range obj.IndexSignatures {
		if idx == nil {
			continue
		}
		if idx.KeyType == types.String || idx.KeyType == types.Any {
			return true
		}
		if idx.KeyType == types.Number && isNumericPropertyName(name) {
			return true
		}
		if idx.IsMapped {
			return true
		}
	}
	return false
}

// objectPropertyType returns a target object's declared type for a property
// (with undefined added when optional), falling back to an applicable index
// signature.
func objectPropertyType(obj *types.ObjectType, name string) (types.Type, bool) {
	if pt, ok := obj.GetEffectiveProperties()[name]; ok {
		if obj.IsPropertyOptional(name) && types.StrictNullChecks {
			return types.NewUnionType(pt, types.Undefined), true
		}
		return pt, true
	}
	for _, idx := range obj.IndexSignatures {
		if idx == nil {
			continue
		}
		if idx.KeyType == types.String || idx.KeyType == types.Any ||
			(idx.KeyType == types.Number && isNumericPropertyName(name)) {
			return idx.ValueType, true
		}
	}
	return nil, false
}

// targetPropertyType is the type a property takes across all target objects
// (the union of the declarations when several members declare it).
func targetPropertyType(objs []*types.ObjectType, name string) (types.Type, bool) {
	var found []types.Type
	for _, o := range objs {
		if pt, ok := objectPropertyType(o, name); ok {
			found = append(found, pt)
		}
	}
	switch len(found) {
	case 0:
		return nil, false
	case 1:
		return found[0], true
	}
	return types.NewUnionType(found...), true
}

// staticPropertyName returns the statically known name of an object literal key.
func staticPropertyName(key parser.Expression) (string, bool) {
	switch k := key.(type) {
	case *parser.Identifier:
		return k.Value, true
	case *parser.StringLiteral:
		return k.Value, true
	case *parser.NumberLiteral:
		return fmt.Sprintf("%v", k.Value), true
	}
	return "", false
}

// excessProperty describes the first property of a fresh object literal that
// its target does not declare.
type excessProperty struct {
	node   parser.Node
	name   string
	target types.Type
}

// findExcessProperty implements hasExcessProperties on the literal syntax:
// freshness is a property of the expression, so only literals written in
// place (possibly nested in array literals, parentheses or conditionals) are
// checked.
func (c *Checker) findExcessProperty(expr parser.Expression, target types.Type) *excessProperty {
	switch e := expr.(type) {
	case *parser.ObjectLiteral:
		for _, p := range e.Properties {
			if _, spread := p.Key.(*parser.SpreadElement); spread {
				return nil // spreads make the literal non-fresh
			}
		}
		objs := c.targetObjects(target)
		if len(objs) == 0 {
			return nil
		}
		for _, o := range objs {
			if len(o.GetEffectiveProperties()) == 0 && len(o.IndexSignatures) == 0 &&
				len(o.CallSignatures) == 0 && len(o.ConstructSignatures) == 0 {
				return nil // {} accepts anything
			}
			if len(o.GetEffectiveProperties()) == 0 && len(o.IndexSignatures) == 0 {
				return nil // function-like targets: be conservative
			}
		}
		for _, p := range e.Properties {
			name, ok := staticPropertyName(p.Key)
			if !ok {
				continue
			}
			known := false
			for _, o := range objs {
				if objectKnowsProperty(o, name) {
					known = true
					break
				}
			}
			if !known {
				return &excessProperty{node: p.Key, name: name, target: target}
			}
		}
		for _, p := range e.Properties {
			name, ok := staticPropertyName(p.Key)
			if !ok || p.Value == nil {
				continue
			}
			if pt, ok := targetPropertyType(objs, name); ok {
				if ex := c.findExcessProperty(p.Value, pt); ex != nil {
					return ex
				}
			}
		}
	case *parser.ArrayLiteral:
		for i, el := range e.Elements {
			if _, spread := el.(*parser.SpreadElement); spread {
				continue
			}
			if et := elementTargetType(c.resolveStructural(target), i); et != nil {
				if ex := c.findExcessProperty(el, et); ex != nil {
					return ex
				}
			}
		}
	case *parser.TernaryExpression:
		if ex := c.findExcessProperty(e.Consequence, target); ex != nil {
			return ex
		}
		return c.findExcessProperty(e.Alternative, target)
	}
	return nil
}

// elementTargetType is the type the i-th element of an array literal is
// related to for an array or tuple target.
func elementTargetType(target types.Type, i int) types.Type {
	switch t := target.(type) {
	case *types.ArrayType:
		return t.ElementType
	case *types.TupleType:
		if i < len(t.ElementTypes) {
			return t.ElementTypes[i]
		}
		return t.RestElementType
	case *types.UnionType:
		var found []types.Type
		for _, m := range t.Types {
			if et := elementTargetType(m, i); et != nil {
				found = append(found, et)
			}
		}
		if len(found) > 0 {
			return types.NewUnionType(found...)
		}
	}
	return nil
}

// assignableToFresh is the assignability relation including the freshness
// rules of object literals written in place.
func (c *Checker) assignableToFresh(expr parser.Expression, source, target types.Type) bool {
	if expr != nil && c.findExcessProperty(expr, target) != nil {
		return false
	}
	return c.isAssignableWithExpansion(source, target)
}

// missingProperties lists the required properties of the target that the
// source lacks (getUnmatchedProperties). ok is false when the target or
// source is not a plain object relation.
func (c *Checker) missingProperties(source, target types.Type) ([]string, bool) {
	srcObj, ok := c.resolveStructural(source).(*types.ObjectType)
	if !ok {
		return nil, false
	}
	tgtObj, ok := c.resolveStructural(target).(*types.ObjectType)
	if !ok {
		return nil, false
	}
	// shouldReportUnmatchedPropertyError: a callable source without
	// properties only focuses on the property when the target is callable too.
	srcProps := srcObj.GetEffectiveProperties()
	if (len(srcObj.CallSignatures) > 0 || len(srcObj.ConstructSignatures) > 0) && len(srcProps) == 0 {
		if !((len(tgtObj.CallSignatures) > 0 && len(srcObj.CallSignatures) > 0) ||
			(len(tgtObj.ConstructSignatures) > 0 && len(srcObj.ConstructSignatures) > 0)) {
			return nil, false
		}
	}
	var missing []string
	for name := range tgtObj.GetEffectiveProperties() {
		if tgtObj.IsPropertyOptional(name) {
			continue
		}
		if _, has := srcProps[name]; !has {
			if strings.HasPrefix(name, "__COMPUTED_PROPERTY__") || strings.HasPrefix(name, "@@symbol:") {
				return nil, false // synthetic names of computed keys: no reliable report
			}
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing, len(missing) > 0
}

// reportLeaf reports an assignability failure at a node with the code TypeScript
// would choose for it.
func (c *Checker) reportLeaf(node parser.Node, expr parser.Expression, source, target types.Type, head relHead) {
	srcStr, tgtStr := c.getAssignmentErrorTypes(c.generalizedSource(source, target), target)

	if expr != nil {
		if ex := c.findExcessProperty(expr, target); ex != nil {
			c.addErrorAtStart(ex.node, errors.TS2353, fmt.Sprintf(
				"Object literal may only specify known properties, and '%s' does not exist in type '%s'.", ex.name, tgtStr))
			return
		}
	}

	if head == headAssign && types.IsReadonlyArrayLike(source) {
		switch target.(type) {
		case *types.ArrayType, *types.TupleType:
			c.addErrorAtStart(node, errors.TS4104, fmt.Sprintf(
				"The type '%s' is 'readonly' and cannot be assigned to the mutable type '%s'.", srcStr, tgtStr))
			return
		}
	}

	if names, ok := c.missingProperties(source, target); ok {
		if head == headAssign {
			switch {
			case len(names) == 1:
				c.addErrorAtStart(node, errors.TS2741, fmt.Sprintf(
					"Property '%s' is missing in type '%s' but required in type '%s'.", names[0], srcStr, tgtStr))
			case len(names) > 5:
				c.addErrorAtStart(node, errors.TS2740, fmt.Sprintf(
					"Type '%s' is missing the following properties from type '%s': %s, and %d more.",
					srcStr, tgtStr, strings.Join(names[:4], ", "), len(names)-4))
			default:
				c.addErrorAtStart(node, errors.TS2739, fmt.Sprintf(
					"Type '%s' is missing the following properties from type '%s': %s",
					srcStr, tgtStr, strings.Join(names, ", ")))
			}
			return
		}
	}

	if head == headAssign {
		if srcObj, ok := c.resolveStructural(source).(*types.ObjectType); ok {
			if tgtObj, ok := c.resolveStructural(target).(*types.ObjectType); ok &&
				types.IsWeakObject(tgtObj) && types.ObjectHasMembers(srcObj) && !types.ObjectsShareProperty(srcObj, tgtObj) {
				c.addErrorAtStart(node, errors.TS2559, fmt.Sprintf(
					"Type '%s' has no properties in common with type '%s'.", srcStr, tgtStr))
				return
			}
		}
	}

	switch head {
	case headArgument:
		c.addErrorAtStart(node, errors.TS2345, fmt.Sprintf(
			"Argument of type '%s' is not assignable to parameter of type '%s'.", srcStr, tgtStr))
	case headSatisfies:
		c.addErrorAtStart(node, errors.TS1360, fmt.Sprintf(
			"Type '%s' does not satisfy the expected type '%s'.", srcStr, tgtStr))
	default:
		c.addErrorAtStart(node, errors.TS2322, fmt.Sprintf(
			"Type '%s' is not assignable to type '%s'.", srcStr, tgtStr))
	}
}

// elaborate descends into an expression that is not assignable and reports
// the failure on the offending property, element or return expression
// (elaborateError). It reports whether any diagnostic was produced.
func (c *Checker) elaborate(expr parser.Expression, source, target types.Type, head relHead) bool {
	if c.didYouMeanToCall(source, target) {
		// elaborateDidYouMeanToCallOrConstruct: calling the source would
		// have worked, so the head error is reported on the expression.
		c.reportLeaf(expr, expr, source, target, head)
		return true
	}
	switch e := expr.(type) {
	case *parser.ObjectLiteral:
		objs := c.targetObjects(target)
		if len(objs) == 0 {
			return false
		}
		reported := false
		for _, p := range e.Properties {
			if p.Value == nil {
				continue
			}
			name, ok := staticPropertyName(p.Key)
			if !ok {
				continue
			}
			tp, ok := targetPropertyType(objs, name)
			if !ok {
				continue
			}
			sp := p.Value.GetComputedType()
			if sp == nil || tp == nil {
				continue
			}
			if c.assignableToFresh(p.Value, sp, tp) {
				continue
			}
			reported = true
			if !c.elaborate(p.Value, sp, tp, headAssign) {
				c.reportLeaf(p.Key, p.Value, sp, tp, headAssign)
			}
		}
		return reported
	case *parser.ArrayLiteral:
		target = c.resolveStructural(target)
		reported := false
		for i, el := range e.Elements {
			if _, spread := el.(*parser.SpreadElement); spread || el == nil {
				continue
			}
			et := elementTargetType(target, i)
			sp := el.GetComputedType()
			if et == nil || sp == nil {
				continue
			}
			if c.assignableToFresh(el, sp, et) {
				continue
			}
			reported = true
			if !c.elaborate(el, sp, et, headAssign) {
				c.reportLeaf(el, el, sp, et, headAssign)
			}
		}
		return reported
	case *parser.ArrowFunctionLiteral:
		if _, isBlock := e.Body.(*parser.BlockStatement); isBlock {
			return false
		}
		for _, p := range e.Parameters {
			if p != nil && p.TypeAnnotation != nil {
				return false
			}
		}
		body, ok := e.Body.(parser.Expression)
		if !ok {
			return false
		}
		srcObj, ok := c.resolveStructural(source).(*types.ObjectType)
		if !ok || len(srcObj.CallSignatures) != 1 {
			return false
		}
		tgtObj, ok := c.resolveStructural(target).(*types.ObjectType)
		if !ok || len(tgtObj.CallSignatures) == 0 {
			return false
		}
		var tgtReturns []types.Type
		for _, s := range tgtObj.CallSignatures {
			if s.ReturnType != nil {
				tgtReturns = append(tgtReturns, s.ReturnType)
			}
		}
		if len(tgtReturns) == 0 || srcObj.CallSignatures[0].ReturnType == nil {
			return false
		}
		srcReturn := srcObj.CallSignatures[0].ReturnType
		tgtReturn := types.NewUnionType(tgtReturns...)
		if tgtReturn == types.Void || tgtReturn == types.Any || c.assignableToFresh(body, srcReturn, tgtReturn) {
			return false
		}
		if !c.elaborate(body, srcReturn, tgtReturn, headAssign) {
			c.reportLeaf(body, body, srcReturn, tgtReturn, headAssign)
		}
		return true
	case *parser.AssignmentExpression:
		if e.Operator == "=" && e.Value != nil {
			if vt := e.Value.GetComputedType(); vt != nil {
				return c.elaborate(e.Value, vt, target, head)
			}
		}
	}
	return false
}

// reportNotAssignable reports that source is not assignable to target for an
// assignment-like check whose error node is errNode and whose source
// expression is expr (nil when the source is not an expression).
func (c *Checker) reportNotAssignable(errNode parser.Node, expr parser.Expression, source, target types.Type, head relHead) {
	// A `(typeof X)[K]` resolved before X had a type is reported as what it
	// is by now.
	source = c.resolveDeferredTypeofIndex(source, false)
	target = c.resolveDeferredTypeofIndex(target, false)
	if expr != nil && c.elaborate(expr, source, target, head) {
		return
	}
	c.reportLeaf(errNode, expr, source, target, head)
}

// generalizedSource mirrors reportRelationError: a literal source is shown as
// its base type unless the target could itself hold a literal ("1" is not
// assignable to number is reported as 'number', not '1').
func (c *Checker) generalizedSource(source, target types.Type) types.Type {
	if _, ok := source.(*types.LiteralType); !ok {
		return source
	}
	switch c.resolveStructural(target).(type) {
	case *types.ObjectType, *types.ArrayType, *types.TupleType:
		return types.GetWidenedType(source)
	}
	switch target {
	case types.String, types.Number, types.BigInt, types.Symbol:
		return types.GetWidenedType(source)
	}
	return source
}

// didYouMeanToCall reports whether the source is a callable whose call result
// (or construct result) would be assignable to the target even though the
// source itself is not.
func (c *Checker) didYouMeanToCall(source, target types.Type) bool {
	srcObj, ok := c.resolveStructural(source).(*types.ObjectType)
	if !ok || (len(srcObj.CallSignatures) == 0 && len(srcObj.ConstructSignatures) == 0) {
		return false
	}
	if c.isAssignableWithExpansion(source, target) {
		return false
	}
	for _, sigs := range [][]*types.Signature{srcObj.ConstructSignatures, srcObj.CallSignatures} {
		for _, sig := range sigs {
			if sig.ReturnType == nil || sig.ReturnType == types.Any || sig.ReturnType == types.Never {
				continue
			}
			if c.isAssignableWithExpansion(sig.ReturnType, target) {
				return true
			}
		}
	}
	return false
}

// checkDestructuringExcess reports TS2353 for a property of a fresh object
// literal that the object binding/assignment pattern it is destructured into
// does not mention (the pattern's implied type is the target of the literal).
func (c *Checker) checkDestructuringExcess(props []*parser.DestructuringProperty, hasRest bool, value parser.Expression) {
	lit, ok := value.(*parser.ObjectLiteral)
	if !ok || hasRest || len(props) == 0 {
		return
	}
	known := make(map[string]bool, len(props))
	var parts []string
	for _, p := range props {
		if p == nil {
			continue
		}
		name, ok := staticPropertyName(p.Key)
		if !ok {
			return // computed pattern keys: the implied type is not closed
		}
		known[name] = true
		parts = append(parts, name+": any")
	}
	for _, lp := range lit.Properties {
		if _, spread := lp.Key.(*parser.SpreadElement); spread {
			return
		}
	}
	for _, lp := range lit.Properties {
		name, ok := staticPropertyName(lp.Key)
		if !ok || known[name] {
			continue
		}
		c.addErrorAtStart(lp.Key, errors.TS2353, fmt.Sprintf(
			"Object literal may only specify known properties, and '%s' does not exist in type '{ %s; }'.",
			name, strings.Join(parts, "; ")))
		return
	}
}

package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// isConstAssertionTarget reports whether a type assertion's target is the
// `const` of `expr as const` / `<const>expr`.
func isConstAssertionTarget(target parser.Expression) bool {
	ident, ok := target.(*parser.Identifier)
	return ok && ident.Value == "const"
}

// isConstAssertion reports whether e is `expr as const` / `<const>expr`.
func isConstAssertion(e parser.Expression) bool {
	ta, ok := e.(*parser.TypeAssertionExpression)
	return ok && isConstAssertionTarget(ta.TargetType)
}

// constAssertionType is the type `expr as const` gives an already-checked
// expression: literals keep their literal types, array literals become
// readonly tuples and object literals get readonly properties, recursively
// (#638). Anything else keeps its computed type.
func constAssertionType(expr parser.Expression) types.Type {
	computed := expr.GetComputedType()
	if computed == nil {
		computed = types.Any
	}
	switch e := expr.(type) {
	case *parser.StringLiteral:
		return &types.LiteralType{Value: vm.String(e.Value)}
	case *parser.NumberLiteral:
		return &types.LiteralType{Value: vm.Number(e.Value)}
	case *parser.BooleanLiteral:
		return &types.LiteralType{Value: vm.BooleanValue(e.Value)}
	case *parser.PrefixExpression:
		if num, ok := e.Right.(*parser.NumberLiteral); ok && e.Operator == "-" {
			return &types.LiteralType{Value: vm.Number(-num.Value)}
		}
	case *parser.ArrayLiteral:
		elems := make([]types.Type, 0, len(e.Elements))
		for _, el := range e.Elements {
			if _, isSpread := el.(*parser.SpreadElement); isSpread || el == nil {
				return computed
			}
			elems = append(elems, constAssertionType(el))
		}
		return types.NewReadonlyType(&types.TupleType{ElementTypes: elems})
	case *parser.ObjectLiteral:
		obj, ok := computed.(*types.ObjectType)
		if !ok {
			return computed
		}
		out := types.NewObjectType()
		for _, name := range obj.PropertyNames() {
			t := obj.Properties[name]
			out.SetProperty(name, t)
		}
		for name, opt := range obj.OptionalProperties {
			if out.OptionalProperties == nil {
				out.OptionalProperties = map[string]bool{}
			}
			out.OptionalProperties[name] = opt
		}
		out.IndexSignatures = obj.IndexSignatures
		out.CallSignatures = obj.CallSignatures
		for _, prop := range e.Properties {
			var name string
			switch k := prop.Key.(type) {
			case *parser.Identifier:
				name = k.Value
			case *parser.StringLiteral:
				name = k.Value
			default:
				continue
			}
			if _, isFn := prop.Value.(*parser.FunctionLiteral); isFn {
				continue
			}
			if _, isMethod := prop.Value.(*parser.MethodDefinition); isMethod {
				continue
			}
			if _, exists := out.Properties[name]; exists && prop.Value != nil {
				out.SetProperty(name, constAssertionType(prop.Value))
			}
		}
		if out.ReadOnlyProperties == nil {
			out.ReadOnlyProperties = map[string]bool{}
		}
		for name := range out.Properties {
			out.ReadOnlyProperties[name] = true
		}
		return out
	}
	return computed
}

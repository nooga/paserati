package checker

import (
	"fmt"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// Under --experimentalDecorators tsc resolves each decorator as a call with
// the arguments the runtime passes (resolveDecorator): the class constructor
// for a class decorator; target and key for a property; target, key and
// descriptor for a method or accessor; target, key and index for a
// parameter. A decorator no signature accepts is reported with a head
// message naming the decorator's kind.

type legacyDecoratorKind int

const (
	legacyClassDecorator legacyDecoratorKind = iota
	legacyParameterDecorator
	legacyPropertyDecorator
	legacyMethodDecorator
)

func (k legacyDecoratorKind) diagnostic() (string, string) {
	switch k {
	case legacyClassDecorator:
		return "TS1238", "Unable to resolve signature of class decorator when called as an expression."
	case legacyParameterDecorator:
		return "TS1239", "Unable to resolve signature of parameter decorator when called as an expression."
	case legacyPropertyDecorator:
		return "TS1240", "Unable to resolve signature of property decorator when called as an expression."
	}
	return "TS1241", "Unable to resolve signature of method decorator when called as an expression."
}

// checkLegacyDecorators resolves every decorator of a class against the
// arguments the legacy decorator runtime calls it with.
func (c *Checker) checkLegacyDecorators(node *parser.ClassDeclaration) {
	if node.Name == nil || node.Body == nil {
		return
	}
	ctorType, _, ok := c.env.Resolve(node.Name.Value)
	if !ok || ctorType == nil {
		return
	}
	var instanceType types.Type = types.Any
	if ctor, isObj := ctorType.(*types.ObjectType); isObj && len(ctor.ConstructSigs()) > 0 && ctor.ConstructSigs()[0].ReturnType != nil {
		instanceType = ctor.ConstructSigs()[0].ReturnType
	}
	target := func(static bool) types.Type {
		if static {
			return ctorType
		}
		return instanceType
	}

	for _, dec := range node.Decorators {
		c.resolveLegacyDecorator(dec, legacyClassDecorator, []types.Type{ctorType})
	}
	// Member decorators are inside the class body: they see its private names.
	saved := c.currentClassContext
	c.setClassContext(node.Name.Value, types.AccessContextInstanceMethod)
	defer func() { c.currentClassContext = saved }()
	for _, m := range node.Body.Methods {
		if m == nil {
			continue
		}
		key := memberKeyType(m.Key)
		if m.Kind == "constructor" {
			if m.Value != nil {
				c.checkLegacyParameterDecorators(m.Value.Parameters, ctorType, types.Undefined)
			}
			continue
		}
		for _, dec := range m.Decorators {
			c.resolveLegacyDecorator(dec, legacyMethodDecorator, []types.Type{target(m.IsStatic), key, types.Any})
		}
		if m.Value != nil {
			c.checkLegacyParameterDecorators(m.Value.Parameters, target(m.IsStatic), key)
		}
	}
	for _, p := range node.Body.Properties {
		if p == nil {
			continue
		}
		if id, ok := p.Key.(*parser.Identifier); ok && strings.HasPrefix(id.Value, "#") {
			continue // private names are not modelled as keys
		}
		args := []types.Type{target(p.IsStatic), memberKeyType(p.Key)}
		if p.IsAccessor {
			args = append(args, types.Any)
		}
		for _, dec := range p.Decorators {
			c.resolveLegacyDecorator(dec, legacyPropertyDecorator, args)
		}
	}
}

func (c *Checker) checkLegacyParameterDecorators(params []*parser.Parameter, target, key types.Type) {
	for i, p := range params {
		if p == nil {
			continue
		}
		for _, dec := range p.Decorators {
			c.resolveLegacyDecorator(dec, legacyParameterDecorator, []types.Type{target, key, &types.LiteralType{Value: vm.Number(float64(i))}})
		}
	}
}

// memberKeyType is the type of the key a decorator receives for a member:
// the literal name when it is static, else string.
func memberKeyType(key parser.Expression) types.Type {
	if ck, ok := key.(*parser.ComputedPropertyName); ok {
		key = ck.Expr
	}
	switch k := key.(type) {
	case *parser.Identifier:
		return &types.LiteralType{Value: vm.String(k.Value)}
	case *parser.StringLiteral:
		return &types.LiteralType{Value: vm.String(k.Value)}
	}
	return types.String
}

// resolveLegacyDecorator reports a decorator that no call signature accepts
// with args (the full argument list; a method decorator whose signature has
// at most two parameters gets only the first two).
func (c *Checker) resolveLegacyDecorator(dec *parser.Decorator, kind legacyDecoratorKind, args []types.Type) {
	if dec == nil || dec.Expression == nil {
		return
	}
	switch dec.Expression.(type) {
	case *parser.Identifier, *parser.MemberExpression, *parser.CallExpression:
	case *parser.ArrowFunctionLiteral, *parser.FunctionLiteral:
		// A function written in place takes its parameter types from the
		// decorator call's arguments.
		// FIXME: its signature is not resolved against those arguments.
		ctx := types.NewFunctionType(&types.Signature{ParameterTypes: args, ReturnType: types.Any})
		c.visitWithContext(dec.Expression, &ContextualType{ExpectedType: ctx, IsContextual: true})
		return
	default:
		return
	}
	c.visit(dec.Expression)
	t := dec.Expression.GetComputedType()
	if t == nil || t == types.Any || c.isUntypedCallTarget(t) {
		return
	}
	var sigs []*types.Signature
	switch tt := t.(type) {
	case *types.ObjectType:
		sigs = tt.CallSigs()
	default:
		return // unions, generics, type parameters: not modelled
	}
	code, msg := kind.diagnostic()
	if len(sigs) == 0 {
		c.addErrorWithCode(dec, code, msg)
		return
	}
	argCount := func(sig *types.Signature) int {
		if kind == legacyMethodDecorator && len(sig.ParameterTypes) <= 2 {
			return 2
		}
		return len(args)
	}
	uncalled := true
	for _, sig := range sigs {
		if minArgCount(sig) != 0 || sig.RestParameterType != nil || len(sig.ParameterTypes) >= argCount(sig) {
			uncalled = false
			break
		}
	}
	if uncalled {
		name := dec.Expression.String()
		c.addErrorWithCode(dec, "TS1329", fmt.Sprintf("'%s' accepts too few arguments to be used as a decorator here. Did you mean to call it first and write '@%s()'?", name, name))
		return
	}
	for _, sig := range sigs {
		if len(sig.TypeParameters) > 0 {
			return // inference over decorator arguments is not modelled
		}
		if c.decoratorSignatureAccepts(sig, args[:argCount(sig)]) {
			return
		}
	}
	c.addErrorWithCode(dec, code, msg)
}

func minArgCount(sig *types.Signature) int {
	n := 0
	for i := range sig.ParameterTypes {
		if i < len(sig.OptionalParams) && sig.OptionalParams[i] {
			break
		}
		n = i + 1
	}
	return n
}

// decoratorSignatureAccepts reports whether sig accepts args by arity and
// by assignability of each argument to its parameter.
func (c *Checker) decoratorSignatureAccepts(sig *types.Signature, args []types.Type) bool {
	if len(args) < minArgCount(sig) {
		return false
	}
	if len(args) > len(sig.ParameterTypes) && sig.RestParameterType == nil {
		return false
	}
	for i, a := range args {
		var param types.Type
		if i < len(sig.ParameterTypes) {
			param = sig.ParameterTypes[i]
		} else if arr, ok := sig.RestParameterType.(*types.ArrayType); ok {
			param = arr.ElementType
		}
		if param == nil || a == types.Any {
			continue
		}
		if !c.rel.IsAssignable(a, param) {
			return false
		}
	}
	return true
}

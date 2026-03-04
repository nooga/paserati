package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// checkAllDecorators iterates all program statements and validates decorators.
// This runs after all declarations are processed so identifiers are resolved.
func (c *Checker) checkAllDecorators(program *parser.Program) {
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *parser.ClassDeclaration:
			c.checkDecorators(s)
		case *parser.ExportNamedDeclaration:
			if classDecl, ok := s.Declaration.(*parser.ClassDeclaration); ok {
				c.checkDecorators(classDecl)
			}
		case *parser.ExpressionStatement:
			if classExpr, ok := s.Expression.(*parser.ClassExpression); ok {
				// Convert for checking
				c.checkDecorators(&parser.ClassDeclaration{
					Name:       classExpr.Name,
					Decorators: classExpr.Decorators,
					Body:       classExpr.Body,
				})
			}
		}
	}
}

// checkDecorators validates all decorators on a class declaration and its members.
// Per TC39 spec, each decorator must resolve to a callable value.
func (c *Checker) checkDecorators(node *parser.ClassDeclaration) {
	// Check class-level decorators
	for _, dec := range node.Decorators {
		c.checkDecoratorExpression(dec)
	}

	// Check method decorators
	for _, method := range node.Body.Methods {
		if method.Kind == "constructor" {
			continue
		}
		for _, dec := range method.Decorators {
			c.checkDecoratorExpression(dec)
		}
	}

	// Check property decorators
	for _, prop := range node.Body.Properties {
		for _, dec := range prop.Decorators {
			c.checkDecoratorExpression(dec)
		}
	}
}

// checkDecoratorExpression validates that a decorator expression resolves to a callable type.
func (c *Checker) checkDecoratorExpression(dec *parser.Decorator) {
	// Resolve the decorator expression type manually from the environment,
	// avoiding the visit() path which may trigger false "undefined variable" errors
	// when the class is being checked in a scope context.
	decType := c.resolveDecoratorExprType(dec.Expression)
	if decType == nil {
		return // Type couldn't be determined, skip validation
	}

	// Check if the type is callable
	if !c.isDecoratorCallable(decType) {
		c.addError(dec, fmt.Sprintf("decorator must be a callable expression, but has type '%s'", decType.String()))
	}
}

// resolveDecoratorExprType resolves the type of a decorator expression by looking up
// identifiers in the environment directly, without triggering visit() side effects.
func (c *Checker) resolveDecoratorExprType(expr parser.Expression) types.Type {
	switch e := expr.(type) {
	case *parser.Identifier:
		// Look up identifier in the environment
		typ, _, found := c.env.Resolve(e.Value)
		if found {
			return typ
		}
		// Not found - might be a forward reference, return nil (skip checking)
		return nil

	case *parser.CallExpression:
		// For decorator call expressions like @prefix("hello"),
		// resolve the callee's return type
		calleeType := c.resolveDecoratorExprType(e.Function)
		if calleeType == nil {
			return nil
		}
		// The call returns a decorator function - for type checking purposes,
		// we assume the return is callable (the call expression itself validates this)
		return types.Any

	case *parser.MemberExpression:
		// For @a.b.c style, resolve the full member expression type
		// For now, return any to avoid false positives
		return types.Any

	default:
		return types.Any
	}
}

// isDecoratorCallable checks if a type is callable as a decorator.
// This includes function types, any, and types with call signatures.
func (c *Checker) isDecoratorCallable(t types.Type) bool {
	// Unwrap type aliases
	t = types.GetEffectiveType(t)

	// any is always callable
	if t == types.Any {
		return true
	}

	switch tt := t.(type) {
	case *types.ObjectType:
		return tt.IsCallable() || tt.IsConstructable()
	case *types.UnionType:
		// All members must be callable
		for _, m := range tt.Types {
			if !c.isDecoratorCallable(m) {
				return false
			}
		}
		return len(tt.Types) > 0
	case *types.IntersectionType:
		// At least one member must be callable
		for _, m := range tt.Types {
			if c.isDecoratorCallable(m) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

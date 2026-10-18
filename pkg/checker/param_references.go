package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// Parameter initializers run before the parameters after them are bound and
// before the body's declarations exist, so tsc rejects references to them:
// TS2372 for a parameter's own name, TS2373 for a later parameter or a name
// the body declares. References inside nested functions run later and are
// fine.

// checkParameterReferences walks program and checks the parameter
// initializers of every function in it.
func (c *Checker) checkParameterReferences(program *parser.Program) {
	var walk func(n parser.Node)
	walk = func(n parser.Node) {
		switch fn := n.(type) {
		case *parser.FunctionLiteral:
			params, body := fn.Parameters, fn.Body
			stmts := blockStatements(body)
			if fn.LoweredParams != nil {
				params = fn.LoweredParams
				if fn.LoweredStmts <= len(stmts) {
					stmts = stmts[fn.LoweredStmts:]
				}
			}
			c.checkParamInitializers(params, stmts)
		case *parser.ArrowFunctionLiteral:
			params := fn.Parameters
			stmts := blockStatements(fn.Body)
			if fn.LoweredParams != nil {
				params = fn.LoweredParams
				if fn.LoweredStmts <= len(stmts) {
					stmts = stmts[fn.LoweredStmts:]
				}
			}
			c.checkParamInitializers(params, stmts)
		case *parser.ShorthandMethod:
			c.checkParamInitializers(fn.Parameters, blockStatements(fn.Body))
		}
		forEachChildInScope(n, walk)
	}
	walk(program)
}

func blockStatements(body parser.Node) []parser.Statement {
	if b, ok := body.(*parser.BlockStatement); ok && b != nil {
		return b.Statements
	}
	return nil
}

func (c *Checker) checkParamInitializers(params []*parser.Parameter, body []parser.Statement) {
	if len(params) == 0 {
		return
	}
	// From ES2015 on, initializers see the enclosing scope rather than the
	// body's declarations, unless lowering the parameters moves them into
	// the body (tsc's useOuterVariableScopeInParameter).
	bodyNames := map[string]bool{}
	if c.target() >= 2015 && !c.paramsRequireScopeChange(params) {
		body = nil
	}
	for _, st := range body {
		for _, name := range parser.DeclaredNames(st) {
			bodyNames[name] = true
		}
		if es, ok := st.(*parser.ExpressionStatement); ok {
			if fl, ok := es.Expression.(*parser.FunctionLiteral); ok && fl.Name != nil {
				bodyNames[fl.Name.Value] = true
			}
		}
	}
	paramNames := make([][]string, len(params))
	for i, p := range params {
		if p == nil {
			continue
		}
		if p.Name != nil {
			paramNames[i] = []string{p.Name.Value}
		} else if p.Pattern != nil {
			parser.CollectPatternNames(p.Pattern, map[string]bool{}, &paramNames[i])
		}
	}
	for i, p := range params {
		if p == nil || p.IsThis {
			continue
		}
		later := map[string]bool{}
		for j := i + 1; j < len(params); j++ {
			for _, n := range paramNames[j] {
				later[n] = true
			}
		}
		own := map[string]bool{}
		for _, n := range paramNames[i] {
			own[n] = true
		}
		label := ""
		if p.Name != nil {
			label = p.Name.Value
		} else if p.Pattern != nil {
			label = p.Pattern.String()
		}
		report := func(id *parser.Identifier) {
			switch {
			case p.Name != nil && id.Value == p.Name.Value:
				c.addErrorWithCode(id, errors.TS2372, fmt.Sprintf("Parameter '%s' cannot reference itself.", id.Value))
			case later[id.Value] || (bodyNames[id.Value] && !own[id.Value] && !earlierParam(paramNames[:i], id.Value)):
				c.addErrorWithCode(id, errors.TS2373, fmt.Sprintf("Parameter '%s' cannot reference identifier '%s' declared after it.", label, id.Value))
			}
		}
		if p.DefaultValue != nil {
			forEachInitializerReference(p.DefaultValue, report)
		}
		if p.Pattern != nil {
			forEachPatternInitializerReference(p.Pattern, report)
		}
	}
}

// paramsRequireScopeChange reports whether emitting params introduces
// temporaries in the body: static fields of a class expression without
// standard class fields, `?.` and `??` before ES2020, and object rest
// before ES2017.
func (c *Checker) paramsRequireScopeChange(params []*parser.Parameter) bool {
	found := false
	var walk func(n parser.Node)
	walk = func(n parser.Node) {
		if found || isNilNode(n) {
			return
		}
		switch x := n.(type) {
		case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral, *parser.ShorthandMethod:
			return
		case *parser.ClassExpression:
			if c.noStandardClassFields && x.Body != nil {
				for _, prop := range x.Body.Properties {
					if prop != nil && prop.IsStatic {
						found = true
						return
					}
				}
			}
		case *parser.OptionalChainingExpression, *parser.OptionalIndexExpression, *parser.OptionalCallExpression:
			if c.target() < 2020 {
				found = true
				return
			}
		case *parser.InfixExpression:
			if x.Operator == "??" && c.target() < 2020 {
				found = true
				return
			}
		case *parser.ObjectParameterPattern:
			if x.RestProperty != nil && c.target() < 2017 {
				found = true
				return
			}
			for _, prop := range x.Properties {
				if prop != nil {
					walk(prop.Key)
					walk(prop.Target)
					walk(prop.Default)
				}
			}
			return
		case *parser.ArrayParameterPattern:
			for _, el := range x.Elements {
				if el != nil {
					walk(el.Target)
					walk(el.Default)
				}
			}
			return
		}
		forEachChildInScope(n, walk)
	}
	for _, p := range params {
		if p == nil {
			continue
		}
		if p.Pattern != nil {
			walk(p.Pattern)
		}
		if p.DefaultValue != nil {
			walk(p.DefaultValue)
		}
	}
	return found
}

func earlierParam(names [][]string, name string) bool {
	for _, ns := range names {
		for _, n := range ns {
			if n == name {
				return true
			}
		}
	}
	return false
}

// forEachPatternInitializerReference visits the computed keys and defaults
// of a destructuring parameter pattern.
func forEachPatternInitializerReference(pattern parser.Expression, fn func(*parser.Identifier)) {
	switch pt := pattern.(type) {
	case *parser.ObjectParameterPattern:
		for _, prop := range pt.Properties {
			if prop == nil {
				continue
			}
			if ck, ok := prop.Key.(*parser.ComputedPropertyName); ok {
				forEachInitializerReference(ck.Expr, fn)
			}
			if prop.Default != nil {
				forEachInitializerReference(prop.Default, fn)
			}
			forEachPatternInitializerReference(prop.Target, fn)
		}
	case *parser.ArrayParameterPattern:
		for _, el := range pt.Elements {
			if el == nil {
				continue
			}
			if el.Default != nil {
				forEachInitializerReference(el.Default, fn)
			}
			forEachPatternInitializerReference(el.Target, fn)
		}
	}
}

// forEachInitializerReference calls fn for each identifier an expression
// reads as it is evaluated: not inside nested functions, and not property
// names.
func forEachInitializerReference(node parser.Node, fn func(*parser.Identifier)) {
	if isNilNode(node) {
		return
	}
	switch n := node.(type) {
	case *parser.Identifier:
		fn(n)
		return
	case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral, *parser.ShorthandMethod:
		return
	case *parser.MemberExpression:
		forEachInitializerReference(n.Object, fn)
		return
	case *parser.ObjectLiteral:
		for _, p := range n.Properties {
			if p == nil {
				continue
			}
			if ck, ok := p.Key.(*parser.ComputedPropertyName); ok {
				forEachInitializerReference(ck.Expr, fn)
			}
			forEachInitializerReference(p.Value, fn)
		}
		return
	case *parser.ClassExpression:
		forEachInitializerReference(n.SuperClass, fn)
		return
	}
	forEachChildInScope(node, func(child parser.Node) {
		forEachInitializerReference(child, fn)
	})
}

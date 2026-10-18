package checker

import (
	"fmt"
	"reflect"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

var nodeInterface = reflect.TypeOf((*parser.Node)(nil)).Elem()

// forEachEagerNode calls fn on node and its descendants outside nested
// function and class bodies (code that runs later); fn returning false
// stops the walk below that node.
func forEachEagerNode(node parser.Node, fn func(parser.Node) bool) {
	if node == nil {
		return
	}
	v := reflect.ValueOf(node)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return
	}
	switch node.(type) {
	case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral, *parser.ClassExpression, *parser.ShorthandMethod:
		return
	}
	if !fn(node) {
		return
	}
	walkNodeFields(v, fn)
}

func walkNodeFields(v reflect.Value, fn func(parser.Node) bool) {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanInterface() {
			continue
		}
		switch f.Kind() {
		case reflect.Ptr, reflect.Interface:
			if f.IsNil() {
				continue
			}
			if n, ok := f.Interface().(parser.Node); ok {
				forEachEagerNode(n, fn)
			} else if f.Kind() == reflect.Ptr && f.Elem().Kind() == reflect.Struct && f.Type().Elem().PkgPath() == nodeInterface.PkgPath() {
				walkNodeFields(f, fn)
			}
		case reflect.Slice:
			for j := 0; j < f.Len(); j++ {
				e := f.Index(j)
				if (e.Kind() != reflect.Ptr && e.Kind() != reflect.Interface) || e.IsNil() {
					continue
				}
				if n, ok := e.Interface().(parser.Node); ok {
					forEachEagerNode(n, fn)
				}
			}
		}
	}
}

// mentionsNameEagerly reports whether node reads the identifier name outside
// any nested function or class body.
func mentionsNameEagerly(node parser.Node, name string) bool {
	found := false
	forEachEagerNode(node, func(n parser.Node) bool {
		if id, ok := n.(*parser.Identifier); ok && id.Value == name {
			found = true
		}
		return !found
	})
	return found
}

// thisPropertyReads lists the property names node reads off this
// (`this.x`, `this.#x`, `this["x"]`) outside nested functions.
func thisPropertyReads(node parser.Node) []string {
	var names []string
	forEachEagerNode(node, func(n parser.Node) bool {
		switch e := n.(type) {
		case *parser.MemberExpression:
			if _, isThis := e.Object.(*parser.ThisExpression); isThis {
				if id, ok := e.Property.(*parser.Identifier); ok {
					names = append(names, id.Value)
				}
			}
		case *parser.IndexExpression:
			if _, isThis := e.Left.(*parser.ThisExpression); isThis {
				if lit, ok := e.Index.(*parser.StringLiteral); ok {
					names = append(names, "[\""+lit.Value+"\"]")
				}
			}
		}
		return true
	})
	return names
}

// checkCircularClassFields reports TS7022 for unannotated fields whose
// initializers read each other off this in a cycle.
func (c *Checker) checkCircularClassFields(body *parser.ClassBody) {
	if !c.noImplicitAny || body == nil {
		return
	}
	for _, static := range []bool{false, true} {
		deps := map[string][]string{}
		keys := map[string]parser.Node{}
		var order []string
		for _, prop := range body.Properties {
			if prop == nil || prop.IsStatic != static || prop.TypeAnnotation != nil || prop.Value == nil {
				continue
			}
			var name string
			switch k := prop.Key.(type) {
			case *parser.Identifier:
				name = k.Value
			case *parser.ComputedPropertyName:
				if lit, ok := k.Expr.(*parser.StringLiteral); ok {
					name = "[\"" + lit.Value + "\"]"
				}
			}
			if name == "" {
				continue
			}
			if _, dup := keys[name]; dup {
				continue
			}
			keys[name] = prop.Key
			if _, computed := prop.Key.(*parser.ComputedPropertyName); computed && prop.Token != nil && prop.Token.Literal == "[" {
				keys[name] = &parser.Identifier{Token: prop.Token, Value: "["}
			}
			order = append(order, name)
			deps[name] = thisPropertyReads(prop.Value)
		}
		for _, name := range order {
			if !reachesItself(name, deps) {
				continue
			}
			c.addErrorAtStart(keys[name], errors.TS7022, fmt.Sprintf("'%s' implicitly has type 'any' because it does not have a type annotation and is referenced directly or indirectly in its own initializer.", name))
		}
	}
}

func reachesItself(start string, deps map[string][]string) bool {
	seen := map[string]bool{}
	stack := append([]string(nil), deps[start]...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == start {
			return true
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		stack = append(stack, deps[n]...)
	}
	return false
}

// checkForOfSelfReference reports TS7022 for `for (let v of <expr using v>)`:
// the variable's type comes from the iterable, which needs that type.
func (c *Checker) checkForOfSelfReference(node *parser.ForOfStatement) {
	if !c.noImplicitAny || node.Iterable == nil {
		return
	}
	decls, _, _ := varLikeDeclarationParts(node.Variable)
	if len(decls) != 1 || decls[0] == nil || decls[0].Name == nil || decls[0].TypeAnnotation != nil {
		return
	}
	name := decls[0].Name
	if !mentionsNameEagerly(node.Iterable, name.Value) {
		return
	}
	c.addErrorWithCode(name, errors.TS7022, fmt.Sprintf("'%s' implicitly has type 'any' because it does not have a type annotation and is referenced directly or indirectly in its own initializer.", name.Value))
}

package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// In the constructor of a derived class, `this` and `super.x` may only be
// used once `super(...)` has run on every path reaching them (tsc's
// checkThisBeforeSuper, TS17009 and TS17011). The walk below tracks that one
// fact through the constructor's own flow: parameter defaults first, then the
// body in evaluation order. Arrow functions and nested functions are flow
// containers of their own and are not entered, matching tsc, which only
// checks a `this` whose flow is the constructor's.
//
// A path that cannot continue (return, throw, break, continue) counts as
// having called super, so it never holds a merge point back.

func (c *Checker) checkSuperBeforeThis(superClass parser.Expression, body *parser.ClassBody) {
	if isNilNode(superClass) || body == nil {
		return
	}
	if _, extendsNull := superClass.(*parser.NullLiteral); extendsNull {
		return
	}
	for _, m := range body.Methods {
		if m == nil || m.Kind != "constructor" || m.Value == nil || m.Value.Body == nil {
			continue
		}
		w := &superFlow{c: c}
		called := false
		for _, p := range m.Value.Parameters {
			if p != nil {
				called = w.walk(p.DefaultValue, called)
			}
		}
		w.walk(m.Value.Body, called)
	}
}

type superFlow struct {
	c *Checker
	// breaks collects the states that leave the innermost breakable
	// statement through `break`.
	breaks []bool
}

// walk visits node with super-called state `called` and returns the state
// after it.
func (w *superFlow) walk(node parser.Node, called bool) bool {
	if isNilNode(node) {
		return called
	}
	switch n := node.(type) {
	case *parser.ThisExpression:
		if !called {
			w.c.addErrorWithCode(n, errors.TS17009, "'super' must be called before accessing 'this' in the constructor of a derived class.")
		}
		return called
	case *parser.MemberExpression:
		if _, isSuper := n.Object.(*parser.SuperExpression); isSuper {
			if !called {
				w.c.addErrorWithCode(n.Object, errors.TS17011, "'super' must be called before accessing a property of 'super' in the constructor of a derived class.")
			}
			return called
		}
		called = w.walk(n.Object, called)
		if _, isIdent := n.Property.(*parser.Identifier); !isIdent {
			called = w.walk(n.Property, called)
		}
		return called
	case *parser.IndexExpression:
		if _, isSuper := n.Left.(*parser.SuperExpression); isSuper && !called {
			w.c.addErrorWithCode(n.Left, errors.TS17011, "'super' must be called before accessing a property of 'super' in the constructor of a derived class.")
			return w.walk(n.Index, called)
		}
	case *parser.CallExpression:
		if _, isSuper := n.Function.(*parser.SuperExpression); isSuper {
			for _, arg := range n.Arguments {
				called = w.walk(arg, called)
			}
			return true
		}

	case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral, *parser.ClassDeclaration,
		*parser.ClassExpression:
		return called

	case *parser.ReturnStatement:
		w.walk(n.ReturnValue, called)
		return true
	case *parser.ThrowStatement:
		w.walk(n.Value, called)
		return true
	case *parser.BreakStatement:
		if n.Label == nil && len(w.breaks) > 0 {
			w.breaks[len(w.breaks)-1] = w.breaks[len(w.breaks)-1] && called
		}
		return true
	case *parser.ContinueStatement:
		return true

	case *parser.IfStatement:
		called = w.walk(n.Condition, called)
		thenState := w.walk(n.Consequence, called)
		elseState := w.walk(n.Alternative, called)
		return thenState && elseState
	case *parser.TernaryExpression:
		called = w.walk(n.Condition, called)
		return w.walk(n.Consequence, called) && w.walk(n.Alternative, called)
	case *parser.InfixExpression:
		switch n.Operator {
		case "&&", "||", "??":
			called = w.walk(n.Left, called)
			w.walk(n.Right, called)
			return called
		}

	case *parser.WhileStatement:
		called = w.walk(n.Condition, called)
		w.loopBody(n.Body, called)
		return called
	case *parser.ForStatement:
		called = w.walk(n.Initializer, called)
		called = w.walk(n.Condition, called)
		w.walk(n.Update, w.loopBody(n.Body, called))
		return called
	case *parser.ForOfStatement:
		called = w.walk(n.Iterable, called)
		w.loopBody(n.Body, called)
		return called
	case *parser.ForInStatement:
		called = w.walk(n.Object, called)
		w.loopBody(n.Body, called)
		return called
	case *parser.DoWhileStatement:
		w.breaks = append(w.breaks, true)
		end := w.walk(n.Body, called)
		end = w.walk(n.Condition, end)
		brk := w.breaks[len(w.breaks)-1]
		w.breaks = w.breaks[:len(w.breaks)-1]
		return end && brk
	case *parser.SwitchStatement:
		called = w.walk(n.Expression, called)
		w.breaks = append(w.breaks, true)
		state, hasDefault := true, false
		for _, sc := range n.Cases {
			if sc == nil {
				continue
			}
			if sc.Condition == nil {
				hasDefault = true
			}
			// A case is entered by matching (with the discriminant's state)
			// or by falling through from the case before it.
			entry := called && state
			entry = w.walk(sc.Condition, entry)
			state = w.walk(sc.Body, entry)
		}
		brk := w.breaks[len(w.breaks)-1]
		w.breaks = w.breaks[:len(w.breaks)-1]
		after := state && brk
		if !hasDefault {
			after = after && called
		}
		return after
	case *parser.TryStatement:
		tryEnd := w.walk(n.Body, called)
		end := tryEnd
		if n.CatchClause != nil {
			// The catch may be entered before anything in the try ran.
			end = tryEnd && w.walk(n.CatchClause.Body, called)
		}
		if !isNilNode(n.FinallyBlock) {
			w.walk(n.FinallyBlock, called)
		}
		return end
	case *parser.LabeledStatement:
		return w.walk(n.Statement, called)
	}
	forEachChildInScope(node, func(child parser.Node) {
		called = w.walk(child, called)
	})
	return called
}

// loopBody walks a loop body that may run zero times; the returned state is
// the body's end state, for the loop's update clause.
func (w *superFlow) loopBody(body parser.Node, called bool) bool {
	w.breaks = append(w.breaks, true)
	end := w.walk(body, called)
	w.breaks = w.breaks[:len(w.breaks)-1]
	return end
}

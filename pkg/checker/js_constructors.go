package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// jsCtor is a JavaScript constructor function in tsc's sense: a function
// whose body assigns `this.x = ...` or whose `F.prototype` is assigned
// members. Its instances have those members (typed any here), and `this`
// inside the function and its prototype methods is the instance.
type jsCtor struct {
	inst *types.ObjectType
}

// collectJSConstructors finds the constructor functions of a JavaScript
// program (see jsCtor). It also records the function literals whose `this` is
// an instance: the constructor itself and `F.prototype.m = function () {}`.
func (c *Checker) collectJSConstructors(program *parser.Program) {
	c.jsCtors = nil
	c.jsFuncThis = nil
	fns := map[string]*parser.FunctionLiteral{}
	props := map[string][]string{}
	protoFns := map[string][]*parser.FunctionLiteral{}
	has := map[string]bool{}
	add := func(name, prop string) {
		key := name + "\x00" + prop
		if !has[key] {
			has[key] = true
			props[name] = append(props[name], prop)
		}
	}
	var walk func(n parser.Node)
	walk = func(n parser.Node) {
		switch d := n.(type) {
		case *parser.ExpressionStatement:
			if fn, ok := d.Expression.(*parser.FunctionLiteral); ok && fn.Name != nil && !fn.Parenthesized {
				fns[fn.Name.Value] = fn
			}
			if m, ok := d.Expression.(*parser.MemberExpression); ok {
				// `F.prototype.k;` (with a JSDoc type) declares k.
				if f, p, ok := protoMember(m); ok {
					add(f, p)
				}
			}
			if as, ok := d.Expression.(*parser.AssignmentExpression); ok && as.Operator == "=" {
				if m, ok := as.Left.(*parser.MemberExpression); ok {
					if f, p, ok := protoMember(m); ok {
						add(f, p)
						if fl, ok := as.Value.(*parser.FunctionLiteral); ok {
							protoFns[f] = append(protoFns[f], fl)
						}
					} else if o, ok := m.Object.(*parser.Identifier); ok {
						if id, ok := m.Property.(*parser.Identifier); ok && id.Value == "prototype" {
							if lit, ok := as.Value.(*parser.ObjectLiteral); ok {
								for _, pr := range lit.Properties {
									if k, ok := pr.Key.(*parser.Identifier); ok {
										add(o.Value, k.Value)
										if fl, ok := pr.Value.(*parser.FunctionLiteral); ok {
											protoFns[o.Value] = append(protoFns[o.Value], fl)
										}
									}
								}
							}
						}
					}
				}
			}
		case *parser.VarStatement:
			c.jsCtorDecls(d.Declarations, fns)
		case *parser.LetStatement:
			c.jsCtorDecls(d.Declarations, fns)
		case *parser.ConstStatement:
			c.jsCtorDecls(d.Declarations, fns)
		}
		forEachChildInScope(n, walk)
	}
	walk(program)
	for name, fn := range fns {
		var walkThis func(n parser.Node)
		walkThis = func(n parser.Node) {
			switch d := n.(type) {
			case *parser.FunctionLiteral, *parser.ClassDeclaration, *parser.ClassExpression:
				return
			case *parser.AssignmentExpression:
				if m, ok := d.Left.(*parser.MemberExpression); ok && d.Operator == "=" {
					if _, ok := m.Object.(*parser.ThisExpression); ok {
						if id, ok := m.Property.(*parser.Identifier); ok {
							add(name, id.Value)
						}
					}
				}
			}
			forEachChildInScope(n, walkThis)
		}
		for _, f := range append([]*parser.FunctionLiteral{fn}, protoFns[name]...) {
			if f.Body != nil {
				walkThis(f.Body)
			}
		}
	}
	for name, fn := range fns {
		if len(props[name]) == 0 {
			continue
		}
		inst := types.NewObjectType()
		for _, p := range props[name] {
			inst.WithProperty(p, types.Any)
		}
		if c.jsCtors == nil {
			c.jsCtors = map[string]*jsCtor{}
			c.jsFuncThis = map[*parser.FunctionLiteral]types.Type{}
		}
		c.jsCtors[name] = &jsCtor{inst: inst}
		c.jsFuncThis[fn] = inst
		for _, fl := range protoFns[name] {
			c.jsFuncThis[fl] = inst
		}
	}
}

func (c *Checker) jsCtorDecls(decls []*parser.VarDeclarator, fns map[string]*parser.FunctionLiteral) {
	for _, d := range decls {
		if fn, ok := d.Value.(*parser.FunctionLiteral); ok && d.Name != nil {
			fns[d.Name.Value] = fn
		}
	}
}

// protoMember splits `F.prototype.m` into F and m.
func protoMember(m *parser.MemberExpression) (fn, prop string, ok bool) {
	inner, ok := m.Object.(*parser.MemberExpression)
	if !ok {
		return "", "", false
	}
	o, ok := inner.Object.(*parser.Identifier)
	if !ok {
		return "", "", false
	}
	if ip, ok := inner.Property.(*parser.Identifier); !ok || ip.Value != "prototype" {
		return "", "", false
	}
	p, ok := m.Property.(*parser.Identifier)
	if !ok {
		return "", "", false
	}
	return o.Value, p.Value, true
}

// jsCtorFor returns the JavaScript constructor function a `new` callee names.
func (c *Checker) jsCtorFor(callee parser.Expression) *jsCtor {
	if id, ok := callee.(*parser.Identifier); ok {
		return c.jsCtors[id.Value]
	}
	return nil
}

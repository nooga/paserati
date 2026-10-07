package checker

import (
	"fmt"
	"math"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// enumMemberConsts records, per enum type, the constant value of each member
// (nil when the member is computed). Merged declarations of one enum share the
// entry, so a later declaration can refer to members of an earlier one.
type enumMemberConsts map[string]*parser.EnumConst

// checkEnumDeclaration handles type checking for enum declarations, following
// tsc's computeEnumMemberValues: members are evaluated in order, a member may
// refer to earlier members of the same enum (bare, or as E.A / E["A"]), and an
// enum may be declared several times (declaration merging).
func (c *Checker) checkEnumDeclaration(node *parser.EnumDeclaration) {
	if node == nil || node.Name == nil {
		return
	}
	enumName := node.Name.Value
	debugPrintf("// [Checker Enum] Checking enum declaration '%s' (const=%v)\n", enumName, node.IsConst)

	// 1. A second declaration of an enum in the same scope merges into the
	// first. Conflicts with other kinds of declarations are reported by the
	// declaration binder (Program.BindErrors).
	var enumType *types.EnumType
	merged := false
	if info, exists := c.env.symbols[enumName]; exists {
		existing, ok := info.Type.(*types.EnumType)
		if !ok || existing.IsConst != node.IsConst {
			c.redeclarationReportedByBinder()
			return
		}
		enumType = existing
		merged = true
	} else {
		enumType = &types.EnumType{
			Name:      enumName,
			Members:   make(map[string]*types.EnumMemberType),
			IsConst:   node.IsConst,
			IsNumeric: true, // until a string member is found
		}
	}
	if c.enumConsts == nil {
		c.enumConsts = make(map[*types.EnumType]enumMemberConsts)
	}
	consts := c.enumConsts[enumType]
	if consts == nil {
		consts = make(enumMemberConsts)
		c.enumConsts[enumType] = consts
	}

	// The enum is visible by name inside its own initializers (`E.A`).
	if !merged {
		c.env.Define(enumName, enumType, false)
	}

	// Every member name of this declaration is in scope while initializers are
	// checked; referring to one declared later is reported (TS2651) instead of
	// being an unresolved name.
	memberEnv := NewEnclosedEnvironment(c.env)
	declared := make(map[string]int, len(node.Members))
	for i, m := range node.Members {
		if m != nil && m.Name != nil {
			if _, dup := declared[m.Name.Value]; !dup {
				declared[m.Name.Value] = i
			}
		}
	}
	for name := range declared {
		if existing, ok := enumType.Members[name]; ok {
			memberEnv.Define(name, existing, true)
		} else {
			memberEnv.Define(name, types.Any, true)
		}
	}

	nextValue := 0.0
	autoValid := true // false after a computed or string member

	for idx, member := range node.Members {
		if member == nil || member.Name == nil {
			continue
		}
		memberName := member.Name.Value

		// A duplicate member name is the binder's TS2300; keep the first.
		if _, dup := enumType.Members[memberName]; dup {
			c.redeclarationReportedByBinder()
			continue
		}

		current := member
		currentIdx := idx
		resolve := func(qualifier, name string) (parser.EnumConst, bool) {
			return c.resolveEnumConstReference(enumType, consts, enumName, qualifier, name, current, declared, currentIdx)
		}

		var constVal *parser.EnumConst
		var value interface{}

		if member.Value != nil {
			result, ok := parser.EvalEnumConstShadow(member.Value, resolve, c.isUserBinding)
			if ok {
				constVal = &result
				if node.IsConst && !result.IsString && (math.IsNaN(result.Num) || math.IsInf(result.Num, 0)) {
					if math.IsNaN(result.Num) {
						c.addErrorWithCode(member.Value, "TS2478", "'const' enum member initializer was evaluated to disallowed value 'NaN'.")
					} else {
						c.addErrorWithCode(member.Value, "TS2477", "'const' enum member initializer was evaluated to a non-finite value.")
					}
				}
			} else {
				// Not a constant expression: still a checked expression. Its
				// names may be declared by later passes, so while the top-level
				// passes run it waits for Pass 2.5.
				initializer, isConstEnum := member.Value, node.IsConst
				check := func() {
					savedEnv := c.env
					c.env = memberEnv
					c.visit(initializer)
					c.env = savedEnv
					if isConstEnum {
						c.addErrorWithCode(initializer, "TS2474", "const enum member initializers must be constant expressions.")
					} else if t := initializer.GetComputedType(); t != nil && !c.isAssignableWithExpansion(t, types.Number) {
						c.addErrorWithCode(initializer, "TS18033", fmt.Sprintf("Type '%s' is not assignable to type 'number' as required for computed enum member values.", t.String()))
					}
				}
				if c.deferMethodBodies && c.blockDepth == 0 && c.functionNestingDepth == 0 {
					c.deferredMethodBodies = append(c.deferredMethodBodies, deferredMethodBodyCheck{env: c.env, run: check})
				} else {
					check()
				}
			}
		} else {
			if !autoValid {
				c.addErrorWithCode(member.Name, errors.TS1061, "Enum member must have initializer.")
			} else {
				constVal = &parser.EnumConst{Num: nextValue}
			}
		}

		switch {
		case constVal == nil:
			value = 0 // computed member: no compile-time value
			autoValid = false
		case constVal.IsString:
			value = constVal.Str
			autoValid = false
			enumType.IsNumeric = false
		default:
			value = enumMemberNumber(constVal.Num)
			nextValue = constVal.Num + 1
			autoValid = true
		}

		memberType := &types.EnumMemberType{
			EnumName:   enumName,
			MemberName: memberName,
			Value:      value,
			Parent:     enumType,
		}
		enumType.Members[memberName] = memberType
		enumType.MemberOrder = append(enumType.MemberOrder, memberName)
		consts[memberName] = constVal
		memberEnv.Update(memberName, memberType)
		member.Name.SetComputedType(memberType)
	}

	// 4. The enum as a type is the union of its members.
	var memberTypes []types.Type
	for _, memberName := range enumType.OrderedMemberNames() {
		memberTypes = append(memberTypes, enumType.Members[memberName])
	}
	unionType := &types.UnionType{Types: memberTypes}
	if merged {
		c.env.typeAliases[enumName] = unionType
	} else if !c.env.DefineTypeAlias(enumName, unionType) {
		c.addError(node.Name, fmt.Sprintf("failed to define enum type '%s'", enumName))
		return
	}

	node.SetComputedType(enumType)
	debugPrintf("// [Checker Enum] Successfully defined enum '%s' with %d members\n", enumName, len(enumType.Members))
}

// enumMemberNumber is the runtime value type EnumMemberType carries for numeric
// members: an int when the value is integral, else the float.
func enumMemberNumber(v float64) interface{} {
	if v == math.Trunc(v) && math.Abs(v) < 1<<53 {
		return int(v)
	}
	return v
}

// resolveEnumConstReference resolves a name inside an enum initializer for the
// constant evaluator: a member of this enum (earlier declarations included), a
// member of another enum, or a constant variable.
func (c *Checker) resolveEnumConstReference(enumType *types.EnumType, consts enumMemberConsts, enumName, qualifier, name string, current *parser.EnumMember, declared map[string]int, currentIdx int) (parser.EnumConst, bool) {
	lookup := func(mc enumMemberConsts, member string) (parser.EnumConst, bool) {
		v, ok := mc[member]
		if !ok || v == nil {
			return parser.EnumConst{}, false
		}
		return *v, true
	}
	if qualifier == "" || qualifier == enumName {
		if idx, inThisDecl := declared[name]; inThisDecl && idx >= currentIdx {
			if idx > currentIdx {
				c.addErrorWithCode(current.Value, "TS2651", "A member initializer in a enum declaration cannot reference members declared after it, including members defined in other enums.")
			}
			return parser.EnumConst{}, false
		}
		if v, ok := lookup(consts, name); ok {
			return v, true
		}
		if qualifier != "" {
			return parser.EnumConst{}, false
		}
		// A constant variable (`const x = 1`, `const y = x * 2`) is a constant
		// too, as long as it is declared before the member.
		return c.evalConstVariable(name, current.Value, 0)
	}
	// A member of another enum.
	if t, _, found := c.env.Resolve(qualifier); found {
		if other, ok := t.(*types.EnumType); ok {
			if mc := c.enumConsts[other]; mc != nil {
				return lookup(mc, name)
			}
		}
	}
	return parser.EnumConst{}, false
}

// evalConstVariable evaluates the initializer of a `const` the enum member at
// useSite refers to by name, when it is declared before the use and is itself a
// constant expression.
func (c *Checker) evalConstVariable(name string, useSite parser.Expression, depth int) (parser.EnumConst, bool) {
	if depth > 16 {
		return parser.EnumConst{}, false
	}
	info := c.blockScopedInfoFor(name)
	if info == nil || info.constInit == nil || info.kind != bsVariable {
		return parser.EnumConst{}, false
	}
	if tok := parser.GetTokenFromNode(useSite); tok != nil && tok.StartPos != 0 && info.pos > tok.StartPos {
		return parser.EnumConst{}, false
	}
	resolve := func(qualifier, ident string) (parser.EnumConst, bool) {
		if qualifier != "" {
			return parser.EnumConst{}, false
		}
		return c.evalConstVariable(ident, useSite, depth+1)
	}
	return parser.EvalEnumConstShadow(info.constInit, resolve, c.isUserBinding)
}

// isUserBinding reports whether name resolves to a binding declared by the
// program (as opposed to a builtin global such as NaN or Infinity).
func (c *Checker) isUserBinding(name string) bool {
	if c.blockScopedInfoFor(name) != nil {
		return true
	}
	for e := c.env; e != nil; e = e.outer {
		if _, ok := e.symbols[name]; ok {
			return e.outer != nil && !c.isGlobalBuiltinScope(e)
		}
	}
	return false
}

// isGlobalBuiltinScope reports whether e is the builtin global environment.
func (c *Checker) isGlobalBuiltinScope(e *Environment) bool {
	return e.outer == nil
}

package parser

import (
	"math"
	"strconv"
)

// EnumConst is the compile-time value of a constant enum member initializer.
type EnumConst struct {
	IsString bool
	Num      float64
	Str      string
}

// EnumConstResolver looks up what a name inside an enum initializer denotes.
// qualifier is "" for a bare identifier (`A`) and the object name for `E.A` or
// `E["A"]`. found is false for anything that is not a known constant member.
type EnumConstResolver func(qualifier, name string) (value EnumConst, found bool)

// EvalEnumConst evaluates an enum member initializer the way tsc's constant
// evaluator does (checker `evaluate`): literals, parenthesized and unary/binary
// numeric operators, string concatenation, substitution-free templates, and
// references to other constant members. ok is false when the expression is not
// a constant expression (the member is "computed").
func EvalEnumConst(expr Expression, resolve EnumConstResolver) (EnumConst, bool) {
	return EvalEnumConstShadow(expr, resolve, nil)
}

// EvalEnumConstShadow is EvalEnumConst with a hook telling whether a name such
// as NaN or Infinity is shadowed by a user binding (and so not the global).
func EvalEnumConstShadow(expr Expression, resolve EnumConstResolver, shadowed func(name string) bool) (EnumConst, bool) {
	switch e := expr.(type) {
	case *NumberLiteral:
		return EnumConst{Num: e.Value}, true
	case *StringLiteral:
		return EnumConst{IsString: true, Str: e.Value}, true
	case *TemplateLiteral:
		out := ""
		for _, part := range e.Parts {
			switch p := part.(type) {
			case *TemplateStringPart:
				out += p.Value
			case Expression:
				v, ok := EvalEnumConstShadow(p, resolve, shadowed)
				if !ok {
					return EnumConst{}, false
				}
				out += enumConstText(v)
			default:
				return EnumConst{}, false
			}
		}
		return EnumConst{IsString: true, Str: out}, true
	case *PrefixExpression:
		v, ok := EvalEnumConstShadow(e.Right, resolve, shadowed)
		if !ok || v.IsString {
			return EnumConst{}, false
		}
		switch e.Operator {
		case "+":
			return v, true
		case "-":
			return EnumConst{Num: -v.Num}, true
		case "~":
			return EnumConst{Num: float64(^toInt32(v.Num))}, true
		}
	case *InfixExpression:
		l, lok := EvalEnumConstShadow(e.Left, resolve, shadowed)
		r, rok := EvalEnumConstShadow(e.Right, resolve, shadowed)
		if !lok || !rok {
			return EnumConst{}, false
		}
		if !l.IsString && !r.IsString {
			switch e.Operator {
			case "|":
				return EnumConst{Num: float64(toInt32(l.Num) | toInt32(r.Num))}, true
			case "&":
				return EnumConst{Num: float64(toInt32(l.Num) & toInt32(r.Num))}, true
			case "^":
				return EnumConst{Num: float64(toInt32(l.Num) ^ toInt32(r.Num))}, true
			case "<<":
				return EnumConst{Num: float64(toInt32(l.Num) << (uint32(toInt32(r.Num)) & 31))}, true
			case ">>":
				return EnumConst{Num: float64(toInt32(l.Num) >> (uint32(toInt32(r.Num)) & 31))}, true
			case ">>>":
				return EnumConst{Num: float64(uint32(toInt32(l.Num)) >> (uint32(toInt32(r.Num)) & 31))}, true
			case "+":
				return EnumConst{Num: l.Num + r.Num}, true
			case "-":
				return EnumConst{Num: l.Num - r.Num}, true
			case "*":
				return EnumConst{Num: l.Num * r.Num}, true
			case "/":
				return EnumConst{Num: l.Num / r.Num}, true
			case "%":
				return EnumConst{Num: math.Mod(l.Num, r.Num)}, true
			case "**":
				return EnumConst{Num: math.Pow(l.Num, r.Num)}, true
			}
			return EnumConst{}, false
		}
		if e.Operator == "+" {
			return EnumConst{IsString: true, Str: enumConstText(l) + enumConstText(r)}, true
		}
	case *Identifier:
		if resolve != nil {
			if v, ok := resolve("", e.Value); ok {
				return v, true
			}
		}
		if shadowed == nil || !shadowed(e.Value) {
			switch e.Value {
			case "NaN":
				return EnumConst{Num: math.NaN()}, true
			case "Infinity":
				return EnumConst{Num: math.Inf(1)}, true
			}
		}
	case *MemberExpression:
		if obj, ok := e.Object.(*Identifier); ok {
			if prop, ok := e.Property.(*Identifier); ok && resolve != nil {
				return resolve(obj.Value, prop.Value)
			}
		}
	case *IndexExpression:
		if obj, ok := e.Left.(*Identifier); ok {
			if idx, ok := e.Index.(*StringLiteral); ok && resolve != nil {
				return resolve(obj.Value, idx.Value)
			}
		}
	}
	return EnumConst{}, false
}

func enumConstText(v EnumConst) string {
	if v.IsString {
		return v.Str
	}
	if math.IsNaN(v.Num) {
		return "NaN"
	}
	if math.IsInf(v.Num, 1) {
		return "Infinity"
	}
	if math.IsInf(v.Num, -1) {
		return "-Infinity"
	}
	return strconv.FormatFloat(v.Num, 'f', -1, 64)
}

// toInt32 is ECMAScript's ToInt32 for a float64.
func toInt32(f float64) int32 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	f = math.Trunc(f)
	m := math.Mod(f, 4294967296)
	if m < 0 {
		m += 4294967296
	}
	return int32(uint32(m))
}

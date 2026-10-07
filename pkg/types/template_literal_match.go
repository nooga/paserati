package types

import (
	"strconv"

	"github.com/nooga/paserati/pkg/vm"
)

// assignableToTemplateLiteral decides assignability to a template literal
// type: a string literal must match its pattern, a union must match member by
// member, an identical template matches. decided is false for source kinds it
// has no rule for, which fall through to the general checks.
func assignableToTemplateLiteral(source Type, target *TemplateLiteralType) (result bool, decided bool) {
	switch s := source.(type) {
	case *LiteralType:
		if s.Value.Type() != vm.TypeString {
			return false, true
		}
		return matchTemplateParts(target.Parts, s.Value.ToString()), true
	case *UnionType:
		for _, member := range s.Types {
			if !IsAssignable(member, target) {
				return false, true
			}
		}
		return true, true
	case *TemplateLiteralType:
		return s.Equals(target), true
	}
	if source == String {
		return false, true
	}
	return false, false
}

// matchTemplateParts reports whether text matches the parts in order: literal
// parts exactly, each interpolation by its type (see matchesTemplateHole).
func matchTemplateParts(parts []TemplateLiteralPart, text string) bool {
	if len(parts) == 0 {
		return text == ""
	}
	p := parts[0]
	if p.IsLiteral {
		if len(text) < len(p.Literal) || text[:len(p.Literal)] != p.Literal {
			return false
		}
		return matchTemplateParts(parts[1:], text[len(p.Literal):])
	}
	for n := 0; n <= len(text); n++ {
		if matchesTemplateHole(p.Type, text[:n]) && matchTemplateParts(parts[1:], text[n:]) {
			return true
		}
	}
	return false
}

// matchesTemplateHole reports whether s is a string an interpolated type
// `${T}` can produce.
func matchesTemplateHole(t Type, s string) bool {
	switch tt := t.(type) {
	case *LiteralType:
		return tt.Value.ToString() == s
	case *UnionType:
		for _, m := range tt.Types {
			if matchesTemplateHole(m, s) {
				return true
			}
		}
		return false
	case *TemplateLiteralType:
		return matchTemplateParts(tt.Parts, s)
	}
	switch t {
	case String, Any:
		return true
	case Number:
		return isNumericTemplateText(s)
	case BigInt:
		if s == "" {
			return false
		}
		start := 0
		if s[0] == '-' {
			start = 1
		}
		if start == len(s) {
			return false
		}
		for i := start; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		return true
	case Boolean:
		return s == "true" || s == "false"
	case Null:
		return s == "null"
	case Undefined:
		return s == "undefined"
	case Symbol, Void, Never, Unknown:
		return false
	}
	// A type the matcher doesn't model (a type parameter, an intrinsic like
	// Capitalize<...>): accept rather than report a false error.
	return true
}

// isNumericTemplateText reports whether s is numeric text `${number}`
// accepts: a finite number literal, optionally signed.
func isNumericTemplateText(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	if !(c >= '0' && c <= '9') && c != '.' && c != '-' && c != '+' {
		return false
	}
	if len(s) > 2 && s[0] == '0' {
		switch s[1] {
		case 'x', 'X', 'o', 'O', 'b', 'B':
			_, err := strconv.ParseInt(s[2:], map[byte]int{'x': 16, 'X': 16, 'o': 8, 'O': 8, 'b': 2, 'B': 2}[s[1]], 64)
			return err == nil
		}
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '_' {
			return false
		}
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

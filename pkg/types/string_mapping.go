package types

import (
	"strings"
	"unicode/utf8"

	"github.com/nooga/paserati/pkg/vm"
)

// StringMappingType is an intrinsic string mapping (Uppercase, Lowercase,
// Capitalize, Uncapitalize) that cannot be applied yet: over string, over a
// type parameter, or over a pattern placeholder such as `${number}`. Mappings
// of literals, unions and template literals are applied when constructed
// (see NewStringMappingType).
type StringMappingType struct {
	Kind string // Uppercase, Lowercase, Capitalize or Uncapitalize
	Type Type
}

func (s *StringMappingType) String() string { return s.Kind + "<" + s.Type.String() + ">" }

func (s *StringMappingType) Equals(other Type) bool {
	o, ok := other.(*StringMappingType)
	return ok && o.Kind == s.Kind && o.Type.Equals(s.Type)
}

func (s *StringMappingType) typeNode() {}

// IsStringMappingName reports whether name is one of the intrinsic mappings.
func IsStringMappingName(name string) bool {
	switch name {
	case "Uppercase", "Lowercase", "Capitalize", "Uncapitalize":
		return true
	}
	return false
}

// ApplyStringMapping maps text the way the intrinsic kind does at runtime.
func ApplyStringMapping(kind, text string) string {
	switch kind {
	case "Uppercase":
		return strings.ToUpper(text)
	case "Lowercase":
		return strings.ToLower(text)
	case "Capitalize", "Uncapitalize":
		if text == "" {
			return text
		}
		r, size := utf8.DecodeRuneInString(text)
		first := string(r)
		if kind == "Capitalize" {
			first = strings.ToUpper(first)
		} else {
			first = strings.ToLower(first)
		}
		return first + text[size:]
	}
	return text
}

// NewStringMappingType is tsc's getStringMappingType: kind<t>, applied to
// what it can be applied to now.
func NewStringMappingType(kind string, t Type) Type {
	switch tt := t.(type) {
	case *UnionType:
		members := make([]Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = NewStringMappingType(kind, m)
		}
		return NewUnionType(members...)
	case *LiteralType:
		if tt.Value.Type() == vm.TypeString {
			return &LiteralType{Value: vm.String(ApplyStringMapping(kind, tt.Value.ToString()))}
		}
		return t
	case *TemplateLiteralType:
		return mapTemplateLiteral(kind, tt)
	case *StringMappingType:
		if tt.Kind == kind {
			return t
		}
		return &StringMappingType{Kind: kind, Type: t}
	case *IntersectionType:
		// "foo" & string is "foo".
		var literal Type
		for _, m := range tt.Types {
			if lit, ok := m.(*LiteralType); ok && lit.Value.Type() == vm.TypeString && literal == nil {
				literal = m
			} else if m != String {
				literal = nil
				break
			}
		}
		if literal != nil {
			return NewStringMappingType(kind, literal)
		}
		return &StringMappingType{Kind: kind, Type: t}
	case *TypeParameterType, *KeyofType, *IndexedAccessType, *ConditionalType:
		return &StringMappingType{Kind: kind, Type: t}
	}
	switch t {
	case Never:
		return Never
	case String, Any:
		return &StringMappingType{Kind: kind, Type: t}
	case Number, BigInt:
		// Mapping<`${number}`>: the pattern placeholder as a template.
		return &StringMappingType{Kind: kind, Type: &TemplateLiteralType{Parts: []TemplateLiteralPart{{Type: t}}}}
	}
	return t
}

// mapTemplateLiteral is tsc's applyTemplateStringMapping: Uppercase and
// Lowercase map every text and placeholder, Capitalize and Uncapitalize the
// leading text, or the leading placeholder when the template starts with one.
func mapTemplateLiteral(kind string, t *TemplateLiteralType) Type {
	parts := make([]TemplateLiteralPart, 0, len(t.Parts))
	for _, p := range t.Parts {
		if p.IsLiteral && p.Literal == "" {
			continue
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return &LiteralType{Value: vm.String("")}
	}
	switch kind {
	case "Uppercase", "Lowercase":
		for i, p := range parts {
			if p.IsLiteral {
				parts[i].Literal = ApplyStringMapping(kind, p.Literal)
			} else {
				parts[i].Type = mapPlaceholder(kind, p.Type)
			}
		}
	default:
		if parts[0].IsLiteral {
			parts[0].Literal = ApplyStringMapping(kind, parts[0].Literal)
		} else {
			parts[0].Type = mapPlaceholder(kind, parts[0].Type)
		}
	}
	// `${X}` is X itself for a string mapping, string, or string literals.
	if len(parts) == 1 && !parts[0].IsLiteral && isStringLike(parts[0].Type) {
		return parts[0].Type
	}
	return &TemplateLiteralType{Parts: parts}
}

// mapPlaceholder maps a template placeholder `${T}`: a pattern placeholder
// (`${number}`) stays one inside the mapping.
func mapPlaceholder(kind string, t Type) Type {
	switch t {
	case Number, BigInt:
		return &StringMappingType{Kind: kind, Type: &TemplateLiteralType{Parts: []TemplateLiteralPart{{Type: t}}}}
	case Boolean:
		return NewStringMappingType(kind, NewUnionType(&LiteralType{Value: vm.String("true")}, &LiteralType{Value: vm.String("false")}))
	}
	return NewStringMappingType(kind, t)
}

// memberOfStringMapping is tsc's isMemberOfStringMapping: source (a string
// literal or template) is in target's set when applying target's mappings to
// it changes nothing and it is in the innermost mapped type.
func (r *Relation) memberOfStringMapping(source Type, target Type) bool {
	if target == Any {
		return true
	}
	if _, isMapping := target.(*StringMappingType); !isMapping {
		return r.IsAssignable(source, target)
	}
	var kinds []string
	inner := target
	for {
		m, ok := inner.(*StringMappingType)
		if !ok {
			break
		}
		kinds = append([]string{m.Kind}, kinds...)
		inner = m.Type
	}
	mapped := source
	for _, k := range kinds {
		mapped = NewStringMappingType(k, mapped)
	}
	if !sameStringType(mapped, source) {
		return false
	}
	switch inner.(type) {
	case *TemplateLiteralType:
		return r.IsAssignable(source, inner)
	}
	if inner == String || inner == Any {
		return true
	}
	return r.IsAssignable(source, inner)
}

// stringMappingRelated relates a source or target string mapping, or
// reports decided false when neither side is one.
func (r *Relation) stringMappingRelated(source, target Type) (related, decided bool) {
	sm, sourceIsMapping := source.(*StringMappingType)
	tm, targetIsMapping := target.(*StringMappingType)
	switch {
	case sourceIsMapping && targetIsMapping:
		if sm.Kind == tm.Kind {
			return r.IsAssignable(sm.Type, tm.Type), true
		}
		return false, true
	case targetIsMapping:
		switch s := source.(type) {
		case *LiteralType, *TemplateLiteralType:
			return r.memberOfStringMapping(source, target), true
		case *UnionType:
			for _, m := range s.Types {
				if !r.IsAssignable(m, target) {
					return false, true
				}
			}
			return true, true
		}
		if source == Never || source == Any {
			return true, true
		}
		if _, isParam := source.(*TypeParameterType); isParam {
			return false, false
		}
		return false, true
	case sourceIsMapping:
		// A mapping is a string; it fits string and `${string}` only.
		switch t := target.(type) {
		case *TemplateLiteralType:
			parts := nonEmptyTemplateParts(t.Parts)
			return len(parts) == 1 && !parts[0].IsLiteral && (parts[0].Type == String || parts[0].Type == Any), true
		case *UnionType:
			for _, m := range t.Types {
				if r.IsAssignable(source, m) {
					return true, true
				}
			}
			return false, true
		}
		if target == String || target == Any || target == Unknown {
			return true, true
		}
		return false, false
	}
	return false, false
}

// isStringLike reports whether `${t}` is t itself: string, a string mapping,
// or string literals.
func isStringLike(t Type) bool {
	switch tt := t.(type) {
	case *StringMappingType:
		return true
	case *LiteralType:
		return tt.Value.Type() == vm.TypeString
	case *UnionType:
		for _, m := range tt.Types {
			if !isStringLike(m) {
				return false
			}
		}
		return true
	}
	return t == String
}

// sameStringType compares string types ignoring empty template texts.
func sameStringType(a, b Type) bool {
	at, aok := a.(*TemplateLiteralType)
	bt, bok := b.(*TemplateLiteralType)
	if aok && bok {
		return (&TemplateLiteralType{Parts: nonEmptyTemplateParts(at.Parts)}).Equals(&TemplateLiteralType{Parts: nonEmptyTemplateParts(bt.Parts)})
	}
	return a.Equals(b)
}

// IsGenericStringType reports whether a string type is still generic:
// a type parameter, or one inside an intersection, union, template literal,
// mapping, keyof or indexed access.
func IsGenericStringType(t Type) bool {
	switch tt := t.(type) {
	case *TypeParameterType:
		return true
	case *StringMappingType:
		return IsGenericStringType(tt.Type)
	case *IntersectionType:
		for _, m := range tt.Types {
			if IsGenericStringType(m) {
				return true
			}
		}
	case *UnionType:
		for _, m := range tt.Types {
			if IsGenericStringType(m) {
				return true
			}
		}
	case *TemplateLiteralType:
		for _, p := range tt.Parts {
			if !p.IsLiteral && IsGenericStringType(p.Type) {
				return true
			}
		}
	case *KeyofType:
		return IsGenericStringType(tt.OperandType)
	case *IndexedAccessType:
		return IsGenericStringType(tt.ObjectType) || IsGenericStringType(tt.IndexType)
	}
	return false
}

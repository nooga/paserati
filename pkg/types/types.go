package types

import (
	"fmt"
	"strings"
)

// Type is the interface implemented by all type representations.
type Type interface {
	// String returns a string representation of the type, suitable for debugging or printing.
	String() string
	// Equals checks if this type is structurally equivalent to another type.
	Equals(other Type) bool
	// TODO: Add IsAssignableTo(target Type) bool

	// typeNode() is a marker method to ensure only types defined in this package
	// can be assigned to the Type interface. This prevents accidental implementation
	// elsewhere and makes the type system closed for now.
	typeNode()
}

// ForwardReferenceType represents a forward reference to a generic class being defined
type ForwardReferenceType struct {
	ClassName      string
	TypeParameters []*TypeParameter
}

// TypeAliasForwardReference represents a forward reference to a type alias being defined
type TypeAliasForwardReference struct {
	AliasName string
}

// GenericTypeAliasForwardReference represents a forward reference to a generic type alias being defined
type GenericTypeAliasForwardReference struct {
	AliasName     string
	TypeArguments []Type
}

func (frt *ForwardReferenceType) String() string {
	return frt.ClassName
}

func (frt *ForwardReferenceType) Equals(other Type) bool {
	if otherFrt, ok := other.(*ForwardReferenceType); ok {
		return frt.ClassName == otherFrt.ClassName
	}
	return false
}

func (frt *ForwardReferenceType) typeNode() {}

func (tafr *TypeAliasForwardReference) String() string {
	return tafr.AliasName
}

func (tafr *TypeAliasForwardReference) Equals(other Type) bool {
	if otherTafr, ok := other.(*TypeAliasForwardReference); ok {
		return tafr.AliasName == otherTafr.AliasName
	}
	return false
}

func (tafr *TypeAliasForwardReference) typeNode() {}

func (gtafr *GenericTypeAliasForwardReference) String() string {
	return gtafr.AliasName + "<...>"
}

func (gtafr *GenericTypeAliasForwardReference) Equals(other Type) bool {
	if otherGtafr, ok := other.(*GenericTypeAliasForwardReference); ok {
		return gtafr.AliasName == otherGtafr.AliasName
	}
	return false
}

func (gtafr *GenericTypeAliasForwardReference) typeNode() {}

// MappedType represents a mapped type like { [P in K]: T }
// This is used for utility types like Partial<T>, Readonly<T>, etc.
type MappedType struct {
	TypeParameter  string // The iteration variable (e.g., "P" in [P in K])
	ConstraintType Type   // The type being iterated over (e.g., K in [P in K])
	ValueType      Type   // The resulting value type for each property
	
	// Modifiers for the mapped type
	ReadonlyModifier string // "+", "-", or "" (for readonly modifier)
	OptionalModifier string // "+", "-", or "" (for optional modifier)
}

func (mt *MappedType) String() string {
	var modifiers []string
	if mt.ReadonlyModifier == "+" {
		modifiers = append(modifiers, "readonly")
	} else if mt.ReadonlyModifier == "-" {
		modifiers = append(modifiers, "-readonly")
	}
	
	optionalMark := ""
	if mt.OptionalModifier == "+" {
		optionalMark = "?"
	} else if mt.OptionalModifier == "-" {
		optionalMark = "-?"
	}
	
	modifierStr := ""
	if len(modifiers) > 0 {
		modifierStr = strings.Join(modifiers, " ") + " "
	}
	
	constraintStr := "unknown"
	if mt.ConstraintType != nil {
		constraintStr = mt.ConstraintType.String()
	}
	
	valueStr := "unknown"
	if mt.ValueType != nil {
		valueStr = mt.ValueType.String()
	}
	
	return fmt.Sprintf("{ %s[%s in %s]%s: %s }", modifierStr, mt.TypeParameter, constraintStr, optionalMark, valueStr)
}

func (mt *MappedType) Equals(other Type) bool {
	otherMt, ok := other.(*MappedType)
	if !ok {
		return false
	}
	
	if mt.TypeParameter != otherMt.TypeParameter {
		return false
	}
	
	if mt.ReadonlyModifier != otherMt.ReadonlyModifier {
		return false
	}
	
	if mt.OptionalModifier != otherMt.OptionalModifier {
		return false
	}
	
	if !mt.ConstraintType.Equals(otherMt.ConstraintType) {
		return false
	}
	
	return mt.ValueType.Equals(otherMt.ValueType)
}

func (mt *MappedType) typeNode() {}

// KeyofType represents a keyof type operator like keyof T
// This evaluates to a union of string literal types representing the keys of the operand type
type KeyofType struct {
	OperandType Type // The type we're getting keys from
}

func (kt *KeyofType) String() string {
	operandStr := "unknown"
	if kt.OperandType != nil {
		operandStr = kt.OperandType.String()
	}
	return fmt.Sprintf("keyof %s", operandStr)
}

func (kt *KeyofType) Equals(other Type) bool {
	otherKt, ok := other.(*KeyofType)
	if !ok {
		return false
	}
	return kt.OperandType.Equals(otherKt.OperandType)
}

func (kt *KeyofType) typeNode() {}

// TypePredicateType represents a type predicate like 'x is string'
// This is used in function return types to indicate type guards
type TypePredicateType struct {
	ParameterName string // The parameter being tested (e.g., "x" in "x is string")
	Type          Type   // The type being tested for
}

func (tpt *TypePredicateType) String() string {
	typeStr := "unknown"
	if tpt.Type != nil {
		typeStr = tpt.Type.String()
	}
	return fmt.Sprintf("%s is %s", tpt.ParameterName, typeStr)
}

func (tpt *TypePredicateType) Equals(other Type) bool {
	otherTpt, ok := other.(*TypePredicateType)
	if !ok {
		return false
	}
	if tpt.ParameterName != otherTpt.ParameterName {
		return false
	}
	return tpt.Type.Equals(otherTpt.Type)
}

func (tpt *TypePredicateType) typeNode() {}

// IndexedAccessType represents an indexed access type like T[K]
// This is used to access properties of a type using a key type
type IndexedAccessType struct {
	ObjectType Type // The type we're indexing into (e.g., T in T[K])
	IndexType  Type // The key type used for indexing (e.g., K in T[K])
}

func (iat *IndexedAccessType) String() string {
	objectStr := "unknown"
	if iat.ObjectType != nil {
		objectStr = iat.ObjectType.String()
	}
	indexStr := "unknown"
	if iat.IndexType != nil {
		indexStr = iat.IndexType.String()
	}
	return fmt.Sprintf("%s[%s]", objectStr, indexStr)
}

func (iat *IndexedAccessType) Equals(other Type) bool {
	otherIat, ok := other.(*IndexedAccessType)
	if !ok {
		return false
	}
	return iat.ObjectType.Equals(otherIat.ObjectType) && iat.IndexType.Equals(otherIat.IndexType)
}

func (iat *IndexedAccessType) typeNode() {}

// ConditionalType represents a conditional type: CheckType extends ExtendsType ? TrueType : FalseType
type ConditionalType struct {
	CheckType   Type // The type being checked (T in T extends U ? X : Y)
	ExtendsType Type // The type being extended/checked against (U in T extends U ? X : Y)
	TrueType    Type // The type when condition is true (X in T extends U ? X : Y)
	FalseType   Type // The type when condition is false (Y in T extends U ? X : Y)
}

func (ct *ConditionalType) String() string {
	checkStr := "unknown"
	if ct.CheckType != nil {
		checkStr = ct.CheckType.String()
	}
	extendsStr := "unknown"
	if ct.ExtendsType != nil {
		extendsStr = ct.ExtendsType.String()
	}
	trueStr := "unknown"
	if ct.TrueType != nil {
		trueStr = ct.TrueType.String()
	}
	falseStr := "unknown"
	if ct.FalseType != nil {
		falseStr = ct.FalseType.String()
	}
	return fmt.Sprintf("%s extends %s ? %s : %s", checkStr, extendsStr, trueStr, falseStr)
}

func (ct *ConditionalType) Equals(other Type) bool {
	otherCt, ok := other.(*ConditionalType)
	if !ok {
		return false
	}
	return ct.CheckType.Equals(otherCt.CheckType) &&
		ct.ExtendsType.Equals(otherCt.ExtendsType) &&
		ct.TrueType.Equals(otherCt.TrueType) &&
		ct.FalseType.Equals(otherCt.FalseType)
}

func (ct *ConditionalType) typeNode() {}

// TemplateLiteralType represents a template literal type like `Hello ${T}!`
// This is used for string manipulation at the type level
type TemplateLiteralType struct {
	Parts []TemplateLiteralPart // Alternating string and type parts
}

// TemplateLiteralPart represents a part of a template literal type
type TemplateLiteralPart struct {
	IsLiteral bool // true for string literals, false for type interpolations
	Literal   string // string content (when IsLiteral=true)
	Type      Type   // interpolated type (when IsLiteral=false)
}

func (tlt *TemplateLiteralType) String() string {
	var out strings.Builder
	out.WriteString("`")
	for _, part := range tlt.Parts {
		if part.IsLiteral {
			// Escape backticks and dollar signs in string parts
			escaped := strings.ReplaceAll(part.Literal, "`", "\\`")
			escaped = strings.ReplaceAll(escaped, "$", "\\$")
			out.WriteString(escaped)
		} else {
			out.WriteString("${")
			if part.Type != nil {
				out.WriteString(part.Type.String())
			} else {
				out.WriteString("unknown")
			}
			out.WriteString("}")
		}
	}
	out.WriteString("`")
	return out.String()
}

func (tlt *TemplateLiteralType) Equals(other Type) bool {
	otherTlt, ok := other.(*TemplateLiteralType)
	if !ok {
		return false
	}
	if len(tlt.Parts) != len(otherTlt.Parts) {
		return false
	}
	for i, part := range tlt.Parts {
		otherPart := otherTlt.Parts[i]
		if part.IsLiteral != otherPart.IsLiteral {
			return false
		}
		if part.IsLiteral {
			if part.Literal != otherPart.Literal {
				return false
			}
		} else {
			if (part.Type == nil) != (otherPart.Type == nil) {
				return false
			}
			if part.Type != nil && !part.Type.Equals(otherPart.Type) {
				return false
			}
		}
	}
	return true
}

func (tlt *TemplateLiteralType) typeNode() {}

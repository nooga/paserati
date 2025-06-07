package types

import (
	"fmt"
	"strings"
)

// --- Function/Object Signatures ---

// Signature represents a function or constructor signature
type Signature struct {
	ParameterTypes    []Type
	ReturnType        Type
	OptionalParams    []bool // Tracks which parameters are optional
	IsVariadic        bool   // Indicates if the function accepts variable arguments
	RestParameterType Type   // Type of the rest parameter (...args), if present
}

func (sig *Signature) String() string {
	var params strings.Builder
	params.WriteString("(")
	numParams := len(sig.ParameterTypes)
	for i, p := range sig.ParameterTypes {
		if sig.IsVariadic && i == numParams-1 {
			params.WriteString("...")
			if p != nil {
				params.WriteString(p.String())
			} else {
				params.WriteString("<nil>")
			}
		} else {
			if p != nil {
				params.WriteString(p.String())
			} else {
				params.WriteString("<nil>")
			}
			// Add optional marker if this parameter is optional
			if i < len(sig.OptionalParams) && sig.OptionalParams[i] {
				params.WriteString("?")
			}
		}
		if i < numParams-1 {
			params.WriteString(", ")
		}
	}

	// Add rest parameter if present
	if sig.RestParameterType != nil {
		if numParams > 0 {
			params.WriteString(", ")
		}
		params.WriteString("...")
		params.WriteString(sig.RestParameterType.String())
	}

	params.WriteString(")")

	retTypeStr := "void"
	if sig.ReturnType != nil {
		retTypeStr = sig.ReturnType.String()
	}

	return fmt.Sprintf("%s => %s", params.String(), retTypeStr)
}

func (sig *Signature) Equals(other *Signature) bool {
	if sig == nil || other == nil {
		return sig == other
	}
	if len(sig.ParameterTypes) != len(other.ParameterTypes) {
		return false
	}
	if sig.IsVariadic != other.IsVariadic {
		return false
	}
	if len(sig.OptionalParams) != len(other.OptionalParams) {
		return false
	}

	// Check parameter types
	for i, p1 := range sig.ParameterTypes {
		p2 := other.ParameterTypes[i]
		if (p1 == nil) != (p2 == nil) {
			return false
		}
		if p1 != nil && !p1.Equals(p2) {
			return false
		}
	}

	// Check optional parameter markers
	for i, opt1 := range sig.OptionalParams {
		if opt1 != other.OptionalParams[i] {
			return false
		}
	}

	// Check rest parameter type
	if (sig.RestParameterType == nil) != (other.RestParameterType == nil) {
		return false
	}
	if sig.RestParameterType != nil && !sig.RestParameterType.Equals(other.RestParameterType) {
		return false
	}

	// Check return type
	if (sig.ReturnType == nil) != (other.ReturnType == nil) {
		return false
	}
	if sig.ReturnType != nil && !sig.ReturnType.Equals(other.ReturnType) {
		return false
	}
	return true
}

// --- Object Types ---

// ObjectType represents the type of an object literal or interface.
type ObjectType struct {
	// Using a map for simplicity now. Consider ordered map or slice for stability.
	Properties         map[string]Type
	OptionalProperties map[string]bool // Tracks which properties are optional

	// NEW: Unified callable/constructor support
	CallSignatures      []*Signature // Object call signatures: obj(args)
	ConstructSignatures []*Signature // Object constructor signatures: new obj(args)
	BaseTypes           []Type       // For inheritance (classes, interfaces)

	// TODO: Index Signatures?
}

func (ot *ObjectType) String() string {
	props := ""
	i := 0
	for name, typ := range ot.Properties {
		if i > 0 {
			props += "; "
		}
		typStr := "<nil>"
		if typ != nil { // Add nil check
			typStr = typ.String()
		}
		optional := ""
		if ot.OptionalProperties != nil && ot.OptionalProperties[name] {
			optional = "?"
		}
		props += fmt.Sprintf("%s%s: %s", name, optional, typStr)
		i++
	}
	return fmt.Sprintf("{ %s }", props)
}
func (ot *ObjectType) typeNode() {}
func (ot *ObjectType) Equals(other Type) bool {
	otherOt, ok := other.(*ObjectType)
	if !ok {
		return false // Not an ObjectType
	}
	if ot == nil || otherOt == nil {
		return ot == otherOt // Both must be nil or non-nil
	}

	// Check properties
	if len(ot.Properties) != len(otherOt.Properties) {
		return false // Different number of properties
	}
	for key, t1 := range ot.Properties {
		t2, exists := otherOt.Properties[key]
		if !exists {
			return false // Key missing in other object
		}
		if (t1 == nil) != (t2 == nil) {
			return false
		} // One type is nil, the other isn't
		if t1 != nil && !t1.Equals(t2) {
			return false // Property types are not equal
		}

		// Check optional property flags
		optional1 := ot.OptionalProperties != nil && ot.OptionalProperties[key]
		optional2 := otherOt.OptionalProperties != nil && otherOt.OptionalProperties[key]
		if optional1 != optional2 {
			return false // Optionality mismatch
		}
	}

	// Check call signatures
	if len(ot.CallSignatures) != len(otherOt.CallSignatures) {
		return false
	}
	for i, sig1 := range ot.CallSignatures {
		sig2 := otherOt.CallSignatures[i]
		if !sig1.Equals(sig2) {
			return false
		}
	}

	// Check constructor signatures
	if len(ot.ConstructSignatures) != len(otherOt.ConstructSignatures) {
		return false
	}
	for i, sig1 := range ot.ConstructSignatures {
		sig2 := otherOt.ConstructSignatures[i]
		if !sig1.Equals(sig2) {
			return false
		}
	}

	// Check base types
	if len(ot.BaseTypes) != len(otherOt.BaseTypes) {
		return false
	}
	for i, base1 := range ot.BaseTypes {
		base2 := otherOt.BaseTypes[i]
		if (base1 == nil) != (base2 == nil) {
			return false
		}
		if base1 != nil && !base1.Equals(base2) {
			return false
		}
	}

	return true // All properties, signatures, and base types match
}

// --- ObjectType Helper Methods ---

// IsCallable returns true if this ObjectType has call signatures
func (ot *ObjectType) IsCallable() bool {
	return len(ot.CallSignatures) > 0
}

// IsConstructable returns true if this ObjectType has constructor signatures
func (ot *ObjectType) IsConstructable() bool {
	return len(ot.ConstructSignatures) > 0
}

// IsPureFunction returns true if this ObjectType is callable but has no properties
// (i.e., it's a pure function)
func (ot *ObjectType) IsPureFunction() bool {
	return ot.IsCallable() && len(ot.Properties) == 0
}

// GetCallSignatures returns the call signatures of this ObjectType
func (ot *ObjectType) GetCallSignatures() []*Signature {
	return ot.CallSignatures
}

// GetConstructSignatures returns the constructor signatures of this ObjectType
func (ot *ObjectType) GetConstructSignatures() []*Signature {
	return ot.ConstructSignatures
}

// GetEffectiveProperties returns all properties including inherited ones from base types
func (ot *ObjectType) GetEffectiveProperties() map[string]Type {
	result := make(map[string]Type)

	// First, add properties from base types (in reverse order for proper precedence)
	for i := len(ot.BaseTypes) - 1; i >= 0; i-- {
		baseType := ot.BaseTypes[i]
		if baseObj, ok := baseType.(*ObjectType); ok {
			baseProps := baseObj.GetEffectiveProperties()
			for name, typ := range baseProps {
				result[name] = typ
			}
		}
	}

	// Then add our own properties (these override base type properties)
	for name, typ := range ot.Properties {
		result[name] = typ
	}

	return result
}

// AddCallSignature adds a call signature to this ObjectType
func (ot *ObjectType) AddCallSignature(sig *Signature) {
	ot.CallSignatures = append(ot.CallSignatures, sig)
}

// AddConstructSignature adds a constructor signature to this ObjectType
func (ot *ObjectType) AddConstructSignature(sig *Signature) {
	ot.ConstructSignatures = append(ot.ConstructSignatures, sig)
}

// AddBaseType adds a base type for inheritance
func (ot *ObjectType) AddBaseType(baseType Type) {
	ot.BaseTypes = append(ot.BaseTypes, baseType)
}

// --- Constructor Functions ---

// NewFunctionType creates an ObjectType representing a pure function
func NewFunctionType(sig *Signature) *ObjectType {
	return &ObjectType{
		Properties:          make(map[string]Type),
		OptionalProperties:  make(map[string]bool),
		CallSignatures:      []*Signature{sig},
		ConstructSignatures: []*Signature{},
		BaseTypes:           []Type{},
	}
}

// NewOverloadedFunctionType creates an ObjectType representing an overloaded function
func NewOverloadedFunctionType(sigs []*Signature) *ObjectType {
	return &ObjectType{
		Properties:          make(map[string]Type),
		OptionalProperties:  make(map[string]bool),
		CallSignatures:      sigs,
		ConstructSignatures: []*Signature{},
		BaseTypes:           []Type{},
	}
}

// NewConstructorType creates an ObjectType representing a pure constructor
func NewConstructorType(sig *Signature) *ObjectType {
	return &ObjectType{
		Properties:          make(map[string]Type),
		OptionalProperties:  make(map[string]bool),
		CallSignatures:      []*Signature{},
		ConstructSignatures: []*Signature{sig},
		BaseTypes:           []Type{},
	}
}

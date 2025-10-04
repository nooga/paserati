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

// IndexSignature represents an index signature like [key: string]: Type
// or a mapped type pattern like [P in K]: V
type IndexSignature struct {
	KeyType   Type // The type of the key (e.g., string, number, symbol)
	ValueType Type // The type of the value
	
	// For mapped types: [P in K]: V
	IsMapped       bool   // Whether this is a mapped type pattern
	TypeParameter  string // The type parameter name (e.g., "P" in [P in K])
	ConstraintType Type   // The constraint type (e.g., K in [P in K])
}

func (is *IndexSignature) String() string {
	if is.IsMapped {
		// Mapped type pattern: [P in K]: V
		constraintStr := "unknown"
		if is.ConstraintType != nil {
			constraintStr = is.ConstraintType.String()
		}
		valueStr := "unknown"
		if is.ValueType != nil {
			valueStr = is.ValueType.String()
		}
		return fmt.Sprintf("[%s in %s]: %s", is.TypeParameter, constraintStr, valueStr)
	}
	
	// Regular index signature: [key: string]: Type
	keyStr := "unknown"
	if is.KeyType != nil {
		keyStr = is.KeyType.String()
	}
	valueStr := "unknown"
	if is.ValueType != nil {
		valueStr = is.ValueType.String()
	}
	return fmt.Sprintf("[key: %s]: %s", keyStr, valueStr)
}

func (is *IndexSignature) Equals(other *IndexSignature) bool {
	if is == nil || other == nil {
		return is == other
	}
	
	// Check if both are mapped types or both are regular index signatures
	if is.IsMapped != other.IsMapped {
		return false
	}
	
	if is.IsMapped {
		// For mapped types, compare type parameter and constraint
		if is.TypeParameter != other.TypeParameter {
			return false
		}
		if !is.ConstraintType.Equals(other.ConstraintType) {
			return false
		}
	} else {
		// For regular index signatures, compare key type
		if !is.KeyType.Equals(other.KeyType) {
			return false
		}
	}
	
	return is.ValueType.Equals(other.ValueType)
}

// ObjectType represents the type of an object literal or interface.
type ObjectType struct {
	// Using a map for simplicity now. Consider ordered map or slice for stability.
	Properties         map[string]Type
	OptionalProperties map[string]bool // Tracks which properties are optional
	ReadOnlyProperties map[string]bool // Tracks which properties are readonly

	// NEW: Unified callable/constructor support
	CallSignatures      []*Signature // Object call signatures: obj(args)
	ConstructSignatures []*Signature // Object constructor signatures: new obj(args)
	BaseTypes           []Type       // For inheritance (classes, interfaces)

	// NEW: Class metadata for access control
	ClassMeta *ClassMetadata // Contains access control information for class types

	// Index signatures for dynamic property access
	IndexSignatures []*IndexSignature // Index signatures like [key: string]: Type
}

func (ot *ObjectType) String() string {
	// If this is a pure function (callable with no properties), show it as a function signature
	if ot.IsPureFunction() && len(ot.CallSignatures) == 1 {
		return ot.CallSignatures[0].String()
	}

	// If this is a pure constructor, show it as a constructor signature
	if len(ot.Properties) == 0 && len(ot.CallSignatures) == 0 && len(ot.ConstructSignatures) == 1 {
		return fmt.Sprintf("new %s", ot.ConstructSignatures[0].String())
	}

	// Build object type representation
	var parts []string

	// Add call signatures
	for _, sig := range ot.CallSignatures {
		parts = append(parts, sig.String())
	}

	// Add constructor signatures
	for _, sig := range ot.ConstructSignatures {
		parts = append(parts, fmt.Sprintf("new %s", sig.String()))
	}

	// Add properties
	for name, typ := range ot.Properties {
		typStr := "<nil>"
		if typ != nil {
			typStr = typ.String()
		}
		optional := ""
		if ot.OptionalProperties != nil && ot.OptionalProperties[name] {
			optional = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", name, optional, typStr))
	}

	// Add index signatures
	for _, indexSig := range ot.IndexSignatures {
		parts = append(parts, indexSig.String())
	}

	if len(parts) == 0 {
		return "{ }"
	}

	return fmt.Sprintf("{ %s }", strings.Join(parts, "; "))
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

	// Check class metadata
	if (ot.ClassMeta == nil) != (otherOt.ClassMeta == nil) {
		return false
	}
	if ot.ClassMeta != nil {
		// Compare class metadata - for type equality, class names must match
		if ot.ClassMeta.ClassName != otherOt.ClassMeta.ClassName ||
		   ot.ClassMeta.IsClassInstance != otherOt.ClassMeta.IsClassInstance ||
		   ot.ClassMeta.IsClassConstructor != otherOt.ClassMeta.IsClassConstructor {
			return false
		}
		// Note: We don't compare member access info for type equality
		// Access modifiers are metadata, not part of the structural type
	}

	// Check index signatures
	if len(ot.IndexSignatures) != len(otherOt.IndexSignatures) {
		return false
	}
	for i, sig1 := range ot.IndexSignatures {
		sig2 := otherOt.IndexSignatures[i]
		if !sig1.Equals(sig2) {
			return false
		}
	}

	return true // All properties, signatures, base types, class metadata, and index signatures match
}

// --- ObjectType Helper Methods ---

// IsCallable returns true if this ObjectType has call signatures
func (ot *ObjectType) IsCallable() bool {
	return ot != nil && ot.CallSignatures != nil && len(ot.CallSignatures) > 0
}

// IsClassInstance returns true if this ObjectType represents a class instance
func (ot *ObjectType) IsClassInstance() bool {
	return ot.ClassMeta != nil && ot.ClassMeta.IsClassInstance
}

// IsClassConstructor returns true if this ObjectType represents a class constructor
func (ot *ObjectType) IsClassConstructor() bool {
	return ot.ClassMeta != nil && ot.ClassMeta.IsClassConstructor
}

// GetClassName returns the class name if this is a class type, empty string otherwise
func (ot *ObjectType) GetClassName() string {
	if ot.ClassMeta != nil {
		return ot.ClassMeta.ClassName
	}
	return ""
}

// GetMemberAccessInfo returns access information for a member, or nil if not a class type
func (ot *ObjectType) GetMemberAccessInfo(memberName string) *MemberAccessInfo {
	if ot.ClassMeta != nil {
		// First check this class
		if info := ot.ClassMeta.GetMemberAccess(memberName); info != nil {
			return info
		}
		
		// Then check parent classes
		for _, baseType := range ot.BaseTypes {
			if baseObj, ok := baseType.(*ObjectType); ok {
				if info := baseObj.GetMemberAccessInfo(memberName); info != nil {
					return info
				}
			}
		}
	}
	return nil
}

// IsAccessibleFrom checks if a member is accessible from the given context
func (ot *ObjectType) IsAccessibleFrom(memberName string, accessContext *AccessContext) bool {
	if ot.ClassMeta != nil {
		// First check if member exists in this class
		if ot.ClassMeta.HasMember(memberName) {
			return ot.ClassMeta.IsAccessibleFrom(memberName, accessContext)
		}

		// Then check parent classes
		for _, baseType := range ot.BaseTypes {
			if baseObj, ok := baseType.(*ObjectType); ok {
				if baseObj.IsAccessibleFrom(memberName, accessContext) {
					return true
				}
			}
		}

		// Member not found in class hierarchy
		return false
	}
	return true // Non-class types have no access restrictions
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

// --- Smart Constructors/Builder Pattern ---

// NewObjectType creates a new ObjectType with empty properties and signatures
func NewObjectType() *ObjectType {
	return &ObjectType{
		Properties:          make(map[string]Type),
		OptionalProperties:  make(map[string]bool),
		CallSignatures:      []*Signature{},
		ConstructSignatures: []*Signature{},
		BaseTypes:           []Type{},
		ClassMeta:           nil, // No class metadata by default
	}
}

// WithProperty adds a required property to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithProperty(name string, propType Type) *ObjectType {
	ot.Properties[name] = propType
	return ot
}

// WithOptionalProperty adds an optional property to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithOptionalProperty(name string, propType Type) *ObjectType {
	ot.Properties[name] = propType
	ot.OptionalProperties[name] = true
	return ot
}

// WithReadOnlyProperty adds a readonly property to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithReadOnlyProperty(name string, propType Type) *ObjectType {
	ot.Properties[name] = propType
	if ot.ReadOnlyProperties == nil {
		ot.ReadOnlyProperties = make(map[string]bool)
	}
	ot.ReadOnlyProperties[name] = true
	return ot
}

// IsReadOnly returns whether a property is readonly
func (ot *ObjectType) IsReadOnly(name string) bool {
	return ot.ReadOnlyProperties != nil && ot.ReadOnlyProperties[name]
}

// WithVariadicProperty adds a variadic method property to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithVariadicProperty(name string, paramTypes []Type, returnType Type, restType Type) *ObjectType {
	methodType := NewObjectType().WithVariadicCallSignature(paramTypes, returnType, restType)
	return ot.WithProperty(name, methodType)
}

// WithCallSignature adds a call signature to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithCallSignature(sig *Signature) *ObjectType {
	ot.CallSignatures = append(ot.CallSignatures, sig)
	return ot
}

// WithSimpleCallSignature adds a simple call signature (params->return) and returns the same instance for chaining
func (ot *ObjectType) WithSimpleCallSignature(paramTypes []Type, returnType Type) *ObjectType {
	sig := &Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    make([]bool, len(paramTypes)),
		IsVariadic:        false,
		RestParameterType: nil,
	}
	return ot.WithCallSignature(sig)
}

// WithVariadicCallSignature adds a variadic call signature and returns the same instance for chaining
func (ot *ObjectType) WithVariadicCallSignature(paramTypes []Type, returnType Type, restType Type) *ObjectType {
	sig := &Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    make([]bool, len(paramTypes)),
		IsVariadic:        true,
		RestParameterType: restType,
	}
	return ot.WithCallSignature(sig)
}

// WithConstructSignature adds a constructor signature to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithConstructSignature(sig *Signature) *ObjectType {
	ot.ConstructSignatures = append(ot.ConstructSignatures, sig)
	return ot
}

// WithSimpleConstructSignature adds a simple constructor signature and returns the same instance for chaining
func (ot *ObjectType) WithSimpleConstructSignature(paramTypes []Type, returnType Type) *ObjectType {
	sig := &Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    make([]bool, len(paramTypes)),
		IsVariadic:        false,
		RestParameterType: nil,
	}
	return ot.WithConstructSignature(sig)
}

// Inherits adds a base type for inheritance and returns the same instance for chaining
func (ot *ObjectType) Inherits(baseType Type) *ObjectType {
	ot.BaseTypes = append(ot.BaseTypes, baseType)
	return ot
}

// AsClassInstance sets this ObjectType as a class instance type with the given class name
func (ot *ObjectType) AsClassInstance(className string) *ObjectType {
	ot.ClassMeta = NewClassMetadata(className, true)
	return ot
}

// AsClassConstructor sets this ObjectType as a class constructor type with the given class name
func (ot *ObjectType) AsClassConstructor(className string) *ObjectType {
	ot.ClassMeta = NewClassMetadata(className, false)
	return ot
}

// WithClassMember adds a class member with access control information
func (ot *ObjectType) WithClassMember(memberName string, memberType Type, accessLevel AccessModifier, isStatic, isReadonly bool) *ObjectType {
	// Add the property to the type
	ot.Properties[memberName] = memberType
	
	// Add access control metadata
	if ot.ClassMeta != nil {
		ot.ClassMeta.AddMember(memberName, accessLevel, isStatic, isReadonly)
	}
	
	return ot
}

// --- Convenience constructors for common patterns ---

// NewSimpleFunction creates a function type: (params) => returnType
func NewSimpleFunction(paramTypes []Type, returnType Type) *ObjectType {
	return NewObjectType().WithSimpleCallSignature(paramTypes, returnType)
}

// NewVariadicFunction creates a variadic function type: (params, ...rest) => returnType
func NewVariadicFunction(paramTypes []Type, returnType Type, restType Type) *ObjectType {
	return NewObjectType().WithVariadicCallSignature(paramTypes, returnType, restType)
}

// NewSimpleConstructor creates a constructor type: new (params) => returnType
func NewSimpleConstructor(paramTypes []Type, returnType Type) *ObjectType {
	return NewObjectType().WithSimpleConstructSignature(paramTypes, returnType)
}

// NewOptionalFunction creates a function type with optional parameters: (param1, param2?) => returnType
func NewOptionalFunction(paramTypes []Type, returnType Type, optionalParams []bool) *ObjectType {
	return NewObjectType().WithCallSignature(SigOptional(paramTypes, returnType, optionalParams))
}

// --- Helper functions for complex types ---

// Sig creates a Signature with the given parameters and return type
func Sig(paramTypes []Type, returnType Type) *Signature {
	return &Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    make([]bool, len(paramTypes)),
		IsVariadic:        false,
		RestParameterType: nil,
	}
}

// SigVariadic creates a variadic Signature
func SigVariadic(paramTypes []Type, returnType Type, restType Type) *Signature {
	return &Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    make([]bool, len(paramTypes)),
		IsVariadic:        true,
		RestParameterType: restType,
	}
}

// SigOptional creates a Signature with optional parameters
func SigOptional(paramTypes []Type, returnType Type, optionalParams []bool) *Signature {
	return &Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    optionalParams,
		IsVariadic:        false,
		RestParameterType: nil,
	}
}

// --- Fluent Signature Builder ---

// NewSignature creates a new signature builder with the given parameter types
func NewSignature(paramTypes ...Type) *Signature {
	return &Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        Void, // Default return type
		OptionalParams:    make([]bool, len(paramTypes)),
		IsVariadic:        false,
		RestParameterType: nil,
	}
}

// WithOptional marks specific parameters as optional (fluent interface)
func (sig *Signature) WithOptional(mask ...bool) *Signature {
	if len(mask) > len(sig.ParameterTypes) {
		// Extend OptionalParams if mask is longer than parameters
		sig.OptionalParams = make([]bool, len(mask))
	}
	for i, isOptional := range mask {
		if i < len(sig.OptionalParams) {
			sig.OptionalParams[i] = isOptional
		}
	}
	return sig
}

// WithOptionalAt marks specific parameter indices as optional (fluent interface)
func (sig *Signature) WithOptionalAt(indices ...int) *Signature {
	for _, index := range indices {
		if index >= 0 && index < len(sig.OptionalParams) {
			sig.OptionalParams[index] = true
		}
	}
	return sig
}

// Returns sets the return type (fluent interface)
func (sig *Signature) Returns(returnType Type) *Signature {
	sig.ReturnType = returnType
	return sig
}

// WithRest adds a rest parameter type (automatically wrapped in ArrayType if needed)
func (sig *Signature) WithRest(restType Type) *Signature {
	sig.IsVariadic = true
	// If restType is already an ArrayType, use it directly
	// Otherwise, wrap it in an ArrayType
	if _, isArray := restType.(*ArrayType); isArray {
		sig.RestParameterType = restType
	} else {
		sig.RestParameterType = &ArrayType{ElementType: restType}
	}
	return sig
}

// ToFunction wraps this signature in an ObjectType with a single call signature
func (sig *Signature) ToFunction() *ObjectType {
	return NewObjectType().WithCallSignature(sig)
}

// --- Legacy Constructor Functions (for backward compatibility) ---

// NewFunctionType creates an ObjectType representing a pure function
func NewFunctionType(sig *Signature) *ObjectType {
	return NewObjectType().WithCallSignature(sig)
}

// NewOverloadedFunctionType creates an ObjectType representing an overloaded function
func NewOverloadedFunctionType(sigs []*Signature) *ObjectType {
	obj := NewObjectType()
	for _, sig := range sigs {
		obj.WithCallSignature(sig)
	}
	return obj
}

// NewConstructorType creates an ObjectType representing a pure constructor
func NewConstructorType(sig *Signature) *ObjectType {
	return NewObjectType().WithConstructSignature(sig)
}

// --- Class Type Constructors ---

// NewClassInstanceType creates an ObjectType representing a class instance
func NewClassInstanceType(className string) *ObjectType {
	return NewObjectType().AsClassInstance(className)
}

// NewClassConstructorType creates an ObjectType representing a class constructor
func NewClassConstructorType(className string, sig *Signature) *ObjectType {
	return NewObjectType().AsClassConstructor(className).WithConstructSignature(sig)
}

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

// --- Legacy Function Types (to be deprecated in later phases) ---

// FunctionType represents the type of a function.
type FunctionType struct {
	ParameterTypes    []Type
	ReturnType        Type
	IsVariadic        bool   // Indicates if the function accepts variable arguments
	OptionalParams    []bool // Tracks which parameters are optional (same length as ParameterTypes)
	RestParameterType Type   // Type of the rest parameter (...args), if present
}

func (ft *FunctionType) String() string {
	var params strings.Builder // Use strings.Builder for efficiency
	params.WriteString("(")
	numParams := len(ft.ParameterTypes)
	for i, p := range ft.ParameterTypes {
		if ft.IsVariadic && i == numParams-1 {
			// For variadic, assume last param type describes the rest elements
			params.WriteString("...")
			if p != nil {
				params.WriteString(p.String()) // e.g., ...any[] or ...number[]
			} else {
				params.WriteString("<nil>") // Default if type is nil? Or maybe error?
			}
		} else {
			if p != nil {
				params.WriteString(p.String())
			} else {
				params.WriteString("<nil>") // Represent nil param type as any?
			}
			// Add optional marker if this parameter is optional
			if i < len(ft.OptionalParams) && ft.OptionalParams[i] {
				params.WriteString("?")
			}
		}
		if i < numParams-1 {
			params.WriteString(", ")
		}
	}

	// Add rest parameter if present
	if ft.RestParameterType != nil {
		if numParams > 0 {
			params.WriteString(", ")
		}
		params.WriteString("...")
		params.WriteString(ft.RestParameterType.String())
	}

	params.WriteString(")")

	retTypeStr := "void" // Default to void if nil? Or unknown?
	if ft.ReturnType != nil {
		retTypeStr = ft.ReturnType.String()
	}

	return fmt.Sprintf("%s => %s", params.String(), retTypeStr)
}
func (ft *FunctionType) typeNode() {}
func (ft *FunctionType) Equals(other Type) bool {
	otherFt, ok := other.(*FunctionType)
	if !ok {
		return false // Not even a FunctionType
	}
	if ft == nil || otherFt == nil {
		return ft == otherFt // Both must be nil or non-nil
	}
	if len(ft.ParameterTypes) != len(otherFt.ParameterTypes) {
		return false // Different number of parameters
	}
	if ft.IsVariadic != otherFt.IsVariadic {
		return false
	}
	if len(ft.OptionalParams) != len(otherFt.OptionalParams) {
		return false // Different optional parameter tracking
	}

	// Check parameter types (invariant check for simplicity)
	for i, p1 := range ft.ParameterTypes {
		p2 := otherFt.ParameterTypes[i]
		if (p1 == nil) != (p2 == nil) {
			return false
		} // One nil, one not
		if p1 != nil && !p1.Equals(p2) {
			return false // Parameter types differ
		}
	}

	// Check optional parameter markers
	for i, opt1 := range ft.OptionalParams {
		if opt1 != otherFt.OptionalParams[i] {
			return false // Optional parameter markers differ
		}
	}

	// Check rest parameter type
	if (ft.RestParameterType == nil) != (otherFt.RestParameterType == nil) {
		return false // One has rest parameter, other doesn't
	}
	if ft.RestParameterType != nil && !ft.RestParameterType.Equals(otherFt.RestParameterType) {
		return false // Rest parameter types differ
	}

	// Check return type
	if (ft.ReturnType == nil) != (otherFt.ReturnType == nil) {
		return false
	} // One nil, one not
	if ft.ReturnType != nil && !ft.ReturnType.Equals(otherFt.ReturnType) {
		return false // Return types differ
	}
	return true // All checks passed
}

// CallableType represents a type that is both callable and has properties
// This matches TypeScript's callable interfaces:
//
//	interface StringConstructor {
//	  (value?: any): string;        // Call signature
//	  fromCharCode(...codes: number[]): string;  // Property
//	}
type CallableType struct {
	CallSignature *FunctionType   // The call signature
	Properties    map[string]Type // Properties/methods on the callable
}

func (ct *CallableType) String() string {
	var result strings.Builder

	// Show call signature
	if ct.CallSignature != nil {
		result.WriteString(ct.CallSignature.String())
	} else {
		result.WriteString("() => unknown")
	}

	// Show properties if any
	if len(ct.Properties) > 0 {
		result.WriteString(" & { ")
		i := 0
		for name, propType := range ct.Properties {
			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(name)
			result.WriteString(": ")
			result.WriteString(propType.String())
			i++
		}
		result.WriteString(" }")
	}

	return result.String()
}

func (ct *CallableType) typeNode() {}

func (ct *CallableType) Equals(other Type) bool {
	otherCt, ok := other.(*CallableType)
	if !ok {
		return false
	}

	// Check call signature
	if (ct.CallSignature == nil) != (otherCt.CallSignature == nil) {
		return false
	}
	if ct.CallSignature != nil && !ct.CallSignature.Equals(otherCt.CallSignature) {
		return false
	}

	// Check properties
	if len(ct.Properties) != len(otherCt.Properties) {
		return false
	}

	for name, propType := range ct.Properties {
		otherPropType, exists := otherCt.Properties[name]
		if !exists {
			return false
		}
		if !propType.Equals(otherPropType) {
			return false
		}
	}

	return true
}

// OverloadedFunctionType represents a function with multiple overload signatures.
type OverloadedFunctionType struct {
	Name           string          // The function name
	Overloads      []*FunctionType // The overload signatures
	Implementation *FunctionType   // The implementation signature (must be compatible with all overloads)
}

func (oft *OverloadedFunctionType) String() string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("overloaded %s:\n", oft.Name))
	for i, overload := range oft.Overloads {
		result.WriteString(fmt.Sprintf("  [%d] %s\n", i, overload.String()))
	}
	if oft.Implementation != nil {
		result.WriteString(fmt.Sprintf("  impl: %s", oft.Implementation.String()))
	}
	return result.String()
}

func (oft *OverloadedFunctionType) typeNode() {}

func (oft *OverloadedFunctionType) Equals(other Type) bool {
	otherOft, ok := other.(*OverloadedFunctionType)
	if !ok {
		return false
	}
	if oft == nil || otherOft == nil {
		return oft == otherOft
	}
	if oft.Name != otherOft.Name {
		return false
	}
	if len(oft.Overloads) != len(otherOft.Overloads) {
		return false
	}
	for i, overload := range oft.Overloads {
		if !overload.Equals(otherOft.Overloads[i]) {
			return false
		}
	}
	// Check implementation
	if (oft.Implementation == nil) != (otherOft.Implementation == nil) {
		return false
	}
	if oft.Implementation != nil && !oft.Implementation.Equals(otherOft.Implementation) {
		return false
	}
	return true
}

// FindBestOverload finds the best matching overload for the given argument types.
// Returns the overload index and the return type, or -1 if no match is found.
func (oft *OverloadedFunctionType) FindBestOverload(argTypes []Type) (int, Type) {
	// Simple matching strategy: find the first overload where all argument types are assignable
	for i, overload := range oft.Overloads {
		if len(argTypes) != len(overload.ParameterTypes) {
			continue // Argument count mismatch
		}

		// Check if all argument types are assignable to parameter types
		allMatch := true
		for j, argType := range argTypes {
			paramType := overload.ParameterTypes[j]
			if !isAssignableSimple(argType, paramType) {
				allMatch = false
				break
			}
		}

		if allMatch {
			return i, overload.ReturnType
		}
	}

	return -1, nil // No matching overload found
}

// Helper function to check if source type is assignable to target type
// This is a simplified version - the full version is now in assignable.go
func isAssignableSimple(source, target Type) bool {
	if source == nil || target == nil {
		return source == target
	}

	// Exact match
	if source.Equals(target) {
		return true
	}

	// Any is assignable to/from anything
	if source == Any || target == Any {
		return true
	}

	return false
}

// ConstructorType represents a constructor function type.
// This is used for functions that can be called with `new` to create instances.
type ConstructorType struct {
	ParameterTypes  []Type // Parameters for the constructor call
	ConstructedType Type   // The type of object this constructor creates
	IsVariadic      bool   // Indicates if the constructor accepts variable arguments
}

func (ct *ConstructorType) String() string {
	var params strings.Builder
	params.WriteString("new (")
	for i, p := range ct.ParameterTypes {
		if ct.IsVariadic && i == len(ct.ParameterTypes)-1 {
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
		}
		if i < len(ct.ParameterTypes)-1 {
			params.WriteString(", ")
		}
	}
	params.WriteString("): ")

	if ct.ConstructedType != nil {
		params.WriteString(ct.ConstructedType.String())
	} else {
		params.WriteString("<nil>")
	}

	return params.String()
}

func (ct *ConstructorType) typeNode() {}

func (ct *ConstructorType) Equals(other Type) bool {
	otherCt, ok := other.(*ConstructorType)
	if !ok {
		return false
	}
	if ct == nil || otherCt == nil {
		return ct == otherCt
	}
	if len(ct.ParameterTypes) != len(otherCt.ParameterTypes) {
		return false
	}
	if ct.IsVariadic != otherCt.IsVariadic {
		return false
	}

	// Check parameter types
	for i, p1 := range ct.ParameterTypes {
		p2 := otherCt.ParameterTypes[i]
		if (p1 == nil) != (p2 == nil) {
			return false
		}
		if p1 != nil && !p1.Equals(p2) {
			return false
		}
	}

	// Check constructed type
	if (ct.ConstructedType == nil) != (otherCt.ConstructedType == nil) {
		return false
	}
	if ct.ConstructedType != nil && !ct.ConstructedType.Equals(otherCt.ConstructedType) {
		return false
	}

	return true
}

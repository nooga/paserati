package types

import (
	"fmt"
	"paserati/pkg/vm"
	"sort"
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

// --- Primitive Types ---

// Primitive represents a fundamental, non-composite type.
type Primitive struct {
	Name string
}

func (p *Primitive) String() string {
	return p.Name
}
func (p *Primitive) typeNode() {}
func (p *Primitive) Equals(other Type) bool {
	// Primitives are singletons, so pointer equality is sufficient.
	return p == other
}

// Pre-defined instances for common primitive types
var (
	Number    = &Primitive{Name: "number"}
	String    = &Primitive{Name: "string"}
	Boolean   = &Primitive{Name: "boolean"}
	Null      = &Primitive{Name: "null"}
	Undefined = &Primitive{Name: "undefined"}
	Any       = &Primitive{Name: "any"}
	Unknown   = &Primitive{Name: "unknown"}
	Never     = &Primitive{Name: "never"}
	Void      = &Primitive{Name: "void"}
	// Add Void later if needed for function returns? TS uses undefined.
)

// TypeofResultType represents the union of all possible string literals that the typeof operator can return
var TypeofResultType = NewUnionType(
	&LiteralType{Value: vm.String("undefined")},
	&LiteralType{Value: vm.String("boolean")},
	&LiteralType{Value: vm.String("number")},
	&LiteralType{Value: vm.String("string")},
	&LiteralType{Value: vm.String("function")},
	&LiteralType{Value: vm.String("object")},
	// Note: In TypeScript/JavaScript, typeof can also return "symbol" and "bigint" in newer versions
	// but for now we'll stick to the basic set that our VM supports
)

// --- Simple Composite Types (Placeholder Structs) ---

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
			if !isAssignable(argType, paramType) {
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
// This is a simplified version - we'll need to improve this
func isAssignable(source, target Type) bool {
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

	// TODO: Add more sophisticated assignability rules
	// - Union type handling
	// - Structural compatibility for objects
	// - etc.

	return false
}

// ArrayType represents the type of an array.
type ArrayType struct {
	ElementType Type
}

func (at *ArrayType) String() string {
	elemTypeStr := "<nil>"
	if at.ElementType != nil { // Add nil check
		elemTypeStr = at.ElementType.String()
	}
	return fmt.Sprintf("%s[]", elemTypeStr)
}
func (at *ArrayType) typeNode() {}
func (at *ArrayType) Equals(other Type) bool {
	otherAt, ok := other.(*ArrayType)
	if !ok {
		return false // Not an ArrayType
	}
	if at == nil || otherAt == nil {
		return at == otherAt // Both must be nil or non-nil
	}
	// Check element type equality
	if (at.ElementType == nil) != (otherAt.ElementType == nil) {
		return false
	} // One nil, one not
	if at.ElementType != nil && !at.ElementType.Equals(otherAt.ElementType) {
		return false // Element types differ
	}
	return true
}

// TupleType represents a tuple type with fixed-length, ordered elements.
// Design mirrors FunctionType's parameter structure for compatibility with spread syntax.
type TupleType struct {
	ElementTypes     []Type // Types of each tuple element (like ParameterTypes in FunctionType)
	OptionalElements []bool // Which elements are optional [string, number?] (like OptionalParams in FunctionType)
	RestElementType  Type   // Type for rest elements [string, ...number[]] (like RestParameterType in FunctionType)
}

func (tt *TupleType) String() string {
	var elements strings.Builder
	elements.WriteString("[")

	numElements := len(tt.ElementTypes)
	for i, elemType := range tt.ElementTypes {
		if elemType != nil {
			elements.WriteString(elemType.String())
		} else {
			elements.WriteString("<nil>")
		}

		// Add optional marker if this element is optional
		if i < len(tt.OptionalElements) && tt.OptionalElements[i] {
			elements.WriteString("?")
		}

		if i < numElements-1 {
			elements.WriteString(", ")
		}
	}

	// Add rest element if present
	if tt.RestElementType != nil {
		if numElements > 0 {
			elements.WriteString(", ")
		}
		elements.WriteString("...")
		elements.WriteString(tt.RestElementType.String())
	}

	elements.WriteString("]")
	return elements.String()
}

func (tt *TupleType) typeNode() {}

func (tt *TupleType) Equals(other Type) bool {
	otherTt, ok := other.(*TupleType)
	if !ok {
		return false // Not a TupleType
	}
	if tt == nil || otherTt == nil {
		return tt == otherTt // Both must be nil or non-nil
	}

	// Check element types
	if len(tt.ElementTypes) != len(otherTt.ElementTypes) {
		return false // Different number of elements
	}
	if len(tt.OptionalElements) != len(otherTt.OptionalElements) {
		return false // Different optional element tracking
	}

	// Check element types (invariant check for simplicity)
	for i, elem1 := range tt.ElementTypes {
		elem2 := otherTt.ElementTypes[i]
		if (elem1 == nil) != (elem2 == nil) {
			return false // One nil, one not
		}
		if elem1 != nil && !elem1.Equals(elem2) {
			return false // Element types differ
		}
	}

	// Check optional element markers
	for i, opt1 := range tt.OptionalElements {
		if opt1 != otherTt.OptionalElements[i] {
			return false // Optional element markers differ
		}
	}

	// Check rest element type
	if (tt.RestElementType == nil) != (otherTt.RestElementType == nil) {
		return false // One has rest element, other doesn't
	}
	if tt.RestElementType != nil && !tt.RestElementType.Equals(otherTt.RestElementType) {
		return false // Rest element types differ
	}

	return true // All checks passed
}

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

// --- NEW: UnionType ---

// UnionType represents a union of multiple types (e.g., string | number).
// Stores constituent types in a slice.
type UnionType struct {
	Types []Type // Slice holding the types in the union
	// TODO: Consider storing unique types or a canonical representation?
}

func (ut *UnionType) String() string {
	typesStr := ""
	for i, t := range ut.Types {
		if i > 0 {
			typesStr += " | "
		}
		typesStr += t.String()
	}
	return typesStr
}
func (ut *UnionType) typeNode() {}
func (ut *UnionType) Equals(other Type) bool {
	otherUt, ok := other.(*UnionType)
	if !ok {
		return false // Not a UnionType
	}
	if ut == nil || otherUt == nil {
		return ut == otherUt // Both must be nil or non-nil
	}

	// Unions are equal if they contain the same set of unique types, regardless of order.
	if len(ut.Types) != len(otherUt.Types) {
		return false // Must have the same number of unique constituent types
	}

	// Create boolean maps to track matches
	matched1 := make([]bool, len(ut.Types))
	matched2 := make([]bool, len(otherUt.Types))

	for i, t1 := range ut.Types {
		foundMatch := false
		for j, t2 := range otherUt.Types {
			if !matched2[j] && t1.Equals(t2) {
				matched1[i] = true
				matched2[j] = true
				foundMatch = true
				break
			}
		}
		if !foundMatch {
			return false // Type t1 from first union not found in second
		}
	}

	// We only need to check one way because we already verified lengths are equal.
	// If every element in ut.Types has a unique match in otherUt.Types, they are equivalent sets.
	return true
}

// ContainsType checks if the union contains a type that equals the given type
func (ut *UnionType) ContainsType(target Type) bool {
	for _, t := range ut.Types {
		if t.Equals(target) {
			return true
		}
	}
	return false
}

// RemoveType returns a new union with the specified type removed
// Returns the modified union type, or the single remaining type if only one remains
func (ut *UnionType) RemoveType(target Type) Type {
	var remainingTypes []Type

	for _, t := range ut.Types {
		if !t.Equals(target) {
			remainingTypes = append(remainingTypes, t)
		}
	}

	// Use NewUnionType to handle simplification (single type, etc.)
	return NewUnionType(remainingTypes...)
}

// --- NEW: LiteralType ---

// LiteralType represents a specific literal value used as a type.
type LiteralType struct {
	Value vm.Value // Holds the literal value (e.g., vm.Number(5), vm.String("hello"))
}

func (lt *LiteralType) isType()        {}
func (lt *LiteralType) Name() string   { return lt.Value.ToString() }
func (lt *LiteralType) String() string { return lt.Value.ToString() }
func (lt *LiteralType) typeNode()      {}
func (lt *LiteralType) Equals(other Type) bool {
	otherLt, ok := other.(*LiteralType)
	if !ok {
		return false // Not a LiteralType
	}
	if lt == nil || otherLt == nil {
		return lt == otherLt // Both must be nil or non-nil
	}

	// Check if underlying vm.Value types are the same
	if lt.Value.Type() != otherLt.Value.Type() {
		return false
	}

	// Compare the actual values based on type
	// Using reflect.DeepEqual is generally safe for the underlying value types (float64, string, bool)
	// Note: This won't work correctly if vm.Value.Value can hold pointers that aren't comparable this way,
	// but for primitives it should be fine.
	//return reflect.DeepEqual(lt.Value.Value, otherLt.Value.Value)
	//Alternative: Use vm specific comparisons if DeepEqual isn't appropriate
	switch lt.Value.Type() {
	case vm.TypeFloatNumber, vm.TypeIntegerNumber:
		return vm.AsNumber(lt.Value) == vm.AsNumber(otherLt.Value)
	case vm.TypeString:
		return vm.AsString(lt.Value) == vm.AsString(otherLt.Value)
	case vm.TypeBoolean:
		return lt.Value.AsBoolean() == otherLt.Value.AsBoolean()
	case vm.TypeNull, vm.TypeUndefined:
		return true // Already checked type match
	default:
		return false // Should not happen for literal types
	}
}

// --- NEW: UnionType Constructor ---

// NewUnionType creates a new union type from the given types.
// It flattens nested unions and removes duplicate types using structural equality.
func NewUnionType(ts ...Type) Type {
	// Use a slice to collect potential members after flattening
	potentialMembers := make([]Type, 0, len(ts))

	var collectTypes func(t Type)
	collectTypes = func(t Type) {
		if t == nil {
			return
		}
		if union, ok := t.(*UnionType); ok {
			// Flatten nested unions
			for _, member := range union.Types {
				collectTypes(member)
			}
		} else if t != Never { // Don't include Never in unions unless it's the only type
			potentialMembers = append(potentialMembers, t)
		}
	}

	// Collect and flatten all input types
	for _, t := range ts {
		collectTypes(t)
	}

	// Filter for unique types using the Equals method
	uniqueMembers := make([]Type, 0, len(potentialMembers))
	for _, pm := range potentialMembers {
		isDuplicate := false
		for _, um := range uniqueMembers {
			if pm.Equals(um) { // <<< USE Equals METHOD
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			uniqueMembers = append(uniqueMembers, pm)
		}
	}

	// Handle simplification
	if len(uniqueMembers) == 0 {
		// If only Never types were input, or input was empty
		return Never
	} else if len(uniqueMembers) == 1 {
		// If only one unique type remains, return it directly
		return uniqueMembers[0]
	}

	// Sort the unique types for a canonical string representation (optional but good)
	sort.SliceStable(uniqueMembers, func(i, j int) bool {
		// Basic sort by string representation for consistency
		return uniqueMembers[i].String() < uniqueMembers[j].String()
	})

	return &UnionType{Types: uniqueMembers}
}

// --- NEW: IntersectionType ---

// IntersectionType represents an intersection of multiple types (e.g., A & B).
// A value of intersection type must satisfy ALL constituent types simultaneously.
type IntersectionType struct {
	Types []Type // Slice holding the types in the intersection
}

func (it *IntersectionType) String() string {
	typesStr := ""
	for i, t := range it.Types {
		if i > 0 {
			typesStr += " & "
		}
		typesStr += t.String()
	}
	return typesStr
}
func (it *IntersectionType) typeNode() {}
func (it *IntersectionType) Equals(other Type) bool {
	otherIt, ok := other.(*IntersectionType)
	if !ok {
		return false // Not an IntersectionType
	}
	if it == nil || otherIt == nil {
		return it == otherIt // Both must be nil or non-nil
	}

	// Intersections are equal if they contain the same set of unique types, regardless of order.
	if len(it.Types) != len(otherIt.Types) {
		return false // Must have the same number of unique constituent types
	}

	// Create boolean maps to track matches
	matched1 := make([]bool, len(it.Types))
	matched2 := make([]bool, len(otherIt.Types))

	for i, t1 := range it.Types {
		foundMatch := false
		for j, t2 := range otherIt.Types {
			if !matched2[j] && t1.Equals(t2) {
				matched1[i] = true
				matched2[j] = true
				foundMatch = true
				break
			}
		}
		if !foundMatch {
			return false // Type t1 from first intersection not found in second
		}
	}

	// We only need to check one way because we already verified lengths are equal.
	return true
}

// --- NEW: IntersectionType Constructor ---

// NewIntersectionType creates a new intersection type from the given types.
// It flattens nested intersections and handles simplifications.
func NewIntersectionType(ts ...Type) Type {
	// Use a slice to collect potential members after flattening
	potentialMembers := make([]Type, 0, len(ts))

	var collectTypes func(t Type)
	collectTypes = func(t Type) {
		if t == nil {
			return
		}
		if intersection, ok := t.(*IntersectionType); ok {
			// Flatten nested intersections
			for _, member := range intersection.Types {
				collectTypes(member)
			}
		} else {
			potentialMembers = append(potentialMembers, t)
		}
	}

	// Collect and flatten all input types
	for _, t := range ts {
		collectTypes(t)
	}

	// Filter for unique types using the Equals method
	uniqueMembers := make([]Type, 0, len(potentialMembers))
	for _, pm := range potentialMembers {
		isDuplicate := false
		for _, um := range uniqueMembers {
			if pm.Equals(um) {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			uniqueMembers = append(uniqueMembers, pm)
		}
	}

	// Handle simplification rules
	if len(uniqueMembers) == 0 {
		// Empty intersection (should not happen normally)
		return Any
	} else if len(uniqueMembers) == 1 {
		// If only one unique type remains, return it directly
		return uniqueMembers[0]
	}

	// Check for any & T = any (any absorbs everything in intersections)
	for _, member := range uniqueMembers {
		if member == Any {
			return Any
		}
	}

	// Check for never & T = never (never propagates in intersections)
	for _, member := range uniqueMembers {
		if member == Never {
			return Never
		}
	}

	// TODO: Add more sophisticated conflict detection for incompatible types
	// For now, let the type checker handle conflicts during assignability checks

	// Sort the unique types for a canonical string representation
	sort.SliceStable(uniqueMembers, func(i, j int) bool {
		return uniqueMembers[i].String() < uniqueMembers[j].String()
	})

	return &IntersectionType{Types: uniqueMembers}
}

// AliasType represents a named type alias.
type AliasType struct {
	Name         string
	ResolvedType Type // The actual type this alias points to after resolution
}

func (at *AliasType) String() string {
	return at.Name
}
func (at *AliasType) typeNode() {}
func (at *AliasType) Equals(other Type) bool {
	// An alias type should probably be considered equal to its resolved type
	// for the purposes of structural comparison. Comparing two aliases directly
	// might be less useful than comparing what they resolve to.
	// However, resolving cycles needs care. Let's compare the resolved type for now.

	// Avoid infinite recursion if an alias points to itself or has cycles
	// TODO: Implement proper cycle detection if needed. For now, assume simple cases.

	if at == nil || other == nil {
		return at == other
	}

	// Check if the other type IS the same alias by pointer
	if otherAt, ok := other.(*AliasType); ok && at == otherAt {
		return true
	}

	// Compare based on the *resolved* type of the alias
	// Note: Ensure ResolvedType is populated before Equals is called.
	if at.ResolvedType == nil {
		// Cannot compare if not resolved
		return false // Or panic? Indicate internal error.
	}
	// Recursively call Equals on the resolved type against the other type
	return at.ResolvedType.Equals(other)

	// Alternative: Consider two different AliasTypes equal if their names
	// AND resolved types are equal?
	// otherAt, ok := other.(*AliasType)
	// if !ok { return false } // Not an AliasType
	// if at.Name != otherAt.Name { return false } // Different names
	// if (at.ResolvedType == nil) != (otherAt.ResolvedType == nil) { return false }
	// if at.ResolvedType != nil && !at.ResolvedType.Equals(otherAt.ResolvedType) { return false }
	// return true
}

// --- NEW: GetWidenedType Function ---

// GetWidenedType converts literal types to their corresponding primitive base types.
// Other types are returned unchanged.
func GetWidenedType(t Type) Type {
	if litType, ok := t.(*LiteralType); ok {
		switch litType.Value.Type() {
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return Number
		case vm.TypeString:
			return String
		case vm.TypeBoolean:
			return Boolean
		case vm.TypeNull:
			return Null // Null widens to null
		case vm.TypeUndefined:
			return Undefined // Undefined widens to undefined
		default:
			// Should not happen for valid literal types (like Function/Closure)
			return t // Return original if unexpected underlying type
		}
	}
	// TODO: Should unions containing only literals of the same base type also widen?
	// e.g., should (1 | 2 | 3) widen to number? Probably.
	// This would require more complex logic here or in NewUnionType.
	return t // Not a literal type, return as is
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

// --- NEW: ObjectType Helper Methods ---

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

// --- NEW: Type-Level Operations (moved from checker) ---

// IsPrimitive returns true if the type is a primitive type
func IsPrimitive(t Type) bool {
	_, ok := t.(*Primitive)
	return ok
}

// GetEffectiveType resolves aliases and returns the actual type
func GetEffectiveType(t Type) Type {
	if aliasType, ok := t.(*AliasType); ok && aliasType.ResolvedType != nil {
		return GetEffectiveType(aliasType.ResolvedType)
	}
	return t
}

// WidenType converts literal types to their primitive equivalents
func WidenType(t Type) Type {
	return GetWidenedType(t) // Use existing function
}

// IsAssignable checks if a value of type `source` can be assigned to a variable
// of type `target`. This is moved from checker to types package for clean separation.
func IsAssignable(source, target Type) bool {
	if source == nil || target == nil {
		return false
	}

	// Basic rules:
	if target == Any || source == Any {
		return true
	}

	if target == Unknown {
		return true
	}
	if source == Unknown {
		return target == Unknown
	}

	if source == Never {
		return true
	}

	// Check for identical types
	if source == target {
		return true
	}

	// Union type handling
	sourceUnion, sourceIsUnion := source.(*UnionType)
	targetUnion, targetIsUnion := target.(*UnionType)

	if targetIsUnion {
		if sourceIsUnion {
			// Union to union: every type in source must be assignable to at least one in target
			for _, sType := range sourceUnion.Types {
				assignable := false
				for _, tType := range targetUnion.Types {
					if IsAssignable(sType, tType) {
						assignable = true
						break
					}
				}
				if !assignable {
					return false
				}
			}
			return true
		} else {
			// Non-union to union: source must be assignable to at least one type in target
			for _, tType := range targetUnion.Types {
				if IsAssignable(source, tType) {
					return true
				}
			}
			return false
		}
	} else if sourceIsUnion {
		// Union to non-union: every type in source must be assignable to target
		for _, sType := range sourceUnion.Types {
			if !IsAssignable(sType, target) {
				return false
			}
		}
		return true
	}

	// Intersection type handling
	sourceIntersection, sourceIsIntersection := source.(*IntersectionType)
	targetIntersection, targetIsIntersection := target.(*IntersectionType)

	if targetIsIntersection {
		// Source must be assignable to ALL types in target intersection
		for _, tType := range targetIntersection.Types {
			if !IsAssignable(source, tType) {
				return false
			}
		}
		return true
	} else if sourceIsIntersection {
		// At least one type in source intersection must be assignable to target
		for _, sType := range sourceIntersection.Types {
			if IsAssignable(sType, target) {
				return true
			}
		}
		return false
	}

	// Literal type handling
	sourceLiteral, sourceIsLiteral := source.(*LiteralType)
	targetLiteral, targetIsLiteral := target.(*LiteralType)

	if sourceIsLiteral && targetIsLiteral {
		// Both literals: values must be equal
		if sourceLiteral.Value.Type() != targetLiteral.Value.Type() {
			return false
		}
		switch sourceLiteral.Value.Type() {
		case vm.TypeNull, vm.TypeUndefined:
			return true
		case vm.TypeBoolean:
			return sourceLiteral.Value.AsBoolean() == targetLiteral.Value.AsBoolean()
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return vm.AsNumber(sourceLiteral.Value) == vm.AsNumber(targetLiteral.Value)
		case vm.TypeString:
			return vm.AsString(sourceLiteral.Value) == vm.AsString(targetLiteral.Value)
		default:
			return false
		}
	} else if sourceIsLiteral {
		// Literal to non-literal: check if literal's primitive type is assignable
		var primitiveType Type
		switch sourceLiteral.Value.Type() {
		case vm.TypeString:
			primitiveType = String
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			primitiveType = Number
		case vm.TypeBoolean:
			primitiveType = Boolean
		default:
			return false
		}
		return IsAssignable(primitiveType, target)
	} else if targetIsLiteral {
		// Non-literal to literal: generally false except for special cases
		return false
	}

	// Array type handling
	sourceArray, sourceIsArray := source.(*ArrayType)
	targetArray, targetIsArray := target.(*ArrayType)

	if sourceIsArray && targetIsArray {
		if sourceArray.ElementType == nil || targetArray.ElementType == nil {
			return false
		}
		return IsAssignable(sourceArray.ElementType, targetArray.ElementType)
	}

	// Tuple type handling
	sourceTuple, sourceIsTuple := source.(*TupleType)
	targetTuple, targetIsTuple := target.(*TupleType)

	if sourceIsTuple && targetIsTuple {
		sourceLen := len(sourceTuple.ElementTypes)
		targetLen := len(targetTuple.ElementTypes)

		// Check each target element against source
		for i := 0; i < targetLen; i++ {
			targetElementType := targetTuple.ElementTypes[i]
			targetIsOptional := i < len(targetTuple.OptionalElements) && targetTuple.OptionalElements[i]

			if i < sourceLen {
				sourceElementType := sourceTuple.ElementTypes[i]
				if !IsAssignable(sourceElementType, targetElementType) {
					return false
				}
			} else if !targetIsOptional {
				// Target element is required but source doesn't have it
				return false
			}
		}

		// Check if source has extra elements that target can't handle
		if sourceLen > targetLen && targetTuple.RestElementType == nil {
			return false
		}

		return true
	}

	// Object type handling
	sourceObj, sourceIsObj := source.(*ObjectType)
	targetObj, targetIsObj := target.(*ObjectType)

	if sourceIsObj && targetIsObj {
		// Check that all required properties in target exist in source and are assignable
		targetProps := targetObj.GetEffectiveProperties()
		sourceProps := sourceObj.GetEffectiveProperties()

		for propName, targetPropType := range targetProps {
			sourcePropType, exists := sourceProps[propName]
			if !exists {
				// Check if property is optional in target
				isOptional := targetObj.OptionalProperties != nil && targetObj.OptionalProperties[propName]
				if !isOptional {
					return false
				}
			} else {
				if !IsAssignable(sourcePropType, targetPropType) {
					return false
				}
			}
		}

		// Check call signatures
		if len(targetObj.CallSignatures) > 0 {
			if len(sourceObj.CallSignatures) == 0 {
				return false
			}
			// For now, require at least one compatible signature
			// TODO: More sophisticated overload matching
			compatible := false
			for _, targetSig := range targetObj.CallSignatures {
				for _, sourceSig := range sourceObj.CallSignatures {
					if isSignatureAssignable(sourceSig, targetSig) {
						compatible = true
						break
					}
				}
				if compatible {
					break
				}
			}
			if !compatible {
				return false
			}
		}

		return true
	}

	// Function type compatibility with ObjectType
	sourceFn, sourceIsFn := source.(*FunctionType)
	if sourceIsFn && targetIsObj && targetObj.IsCallable() {
		// Function can be assigned to callable object if signatures match
		for _, targetSig := range targetObj.CallSignatures {
			if isSignatureAssignable(functionTypeToSignature(sourceFn), targetSig) {
				return true
			}
		}
		return false
	}

	targetFn, targetIsFn := target.(*FunctionType)
	if sourceIsObj && sourceObj.IsCallable() && targetIsFn {
		// Callable object can be assigned to function if signatures match
		for _, sourceSig := range sourceObj.CallSignatures {
			if isSignatureAssignable(sourceSig, functionTypeToSignature(targetFn)) {
				return true
			}
		}
		return false
	}

	if sourceIsFn && targetIsFn {
		return isSignatureAssignable(functionTypeToSignature(sourceFn), functionTypeToSignature(targetFn))
	}

	return false
}

// Helper function to check signature assignability
func isSignatureAssignable(source, target *Signature) bool {
	if source == nil || target == nil {
		return source == target
	}

	// Check parameter count compatibility
	sourceParamCount := len(source.ParameterTypes)
	targetParamCount := len(target.ParameterTypes)

	// For now, require exact parameter count match (can be relaxed later)
	if sourceParamCount != targetParamCount {
		return false
	}

	// Check parameter types (contravariant)
	for i, targetParam := range target.ParameterTypes {
		sourceParam := source.ParameterTypes[i]
		if !IsAssignable(targetParam, sourceParam) { // Note: reversed for contravariance
			return false
		}
	}

	// Check return type (covariant)
	return IsAssignable(source.ReturnType, target.ReturnType)
}

// Helper function to convert FunctionType to Signature
func functionTypeToSignature(ft *FunctionType) *Signature {
	return &Signature{
		ParameterTypes:    ft.ParameterTypes,
		ReturnType:        ft.ReturnType,
		OptionalParams:    ft.OptionalParams,
		IsVariadic:        ft.IsVariadic,
		RestParameterType: ft.RestParameterType,
	}
}

// --- NEW: Constructor Functions ---

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

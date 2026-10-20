package types

import (
	"fmt"
	"strings"
	"sync"
)

// stringifyVisited tracks types being stringified to prevent infinite recursion
// in self-referencing types (e.g., interface Foo { next(): Foo; })
var stringifyVisited sync.Map
var objectEqualsVisited sync.Map

// objectPair keys objectEqualsVisited (a comparable struct, not a formatted
// string: Equals is hot in union de-duplication).
type objectPair struct{ a, b *ObjectType }

// equalsState is one outermost comparison's memo: the object pairs proved
// equal so far, in proof order. Without it, comparing two large mutually
// recursive graphs (playwright's BrowserContext against a `this`-rebound copy
// of it) re-proves every pair once per path that reaches it: exponential.
// A pair proved while an enclosing pair was still assumed equal holds only
// if that one does, so a failing pair drops what was proved under it (like
// TypeScript's maybe-keys); the memo is dropped when the outermost call
// returns, since object types are still filled in between comparisons. It is
// passed down rather than kept in a package variable, since independent
// checkers may run on separate goroutines.
type equalsState struct {
	proved map[objectPair]bool
	log    []objectPair
	// depth counts the object comparisons in progress. An infinitely
	// expanding instantiation (interface L<T> { next: L<L<T>> }) makes
	// fresh objects at every level, which the visited pairs cannot catch;
	// past maxEqualsDepth they are assumed equal, like tsc's deeply nested
	// types relate as Maybe.
	depth int
}

const maxEqualsDepth = 50

// stateEqualer is a type whose Equals passes an equalsState on to the types
// it contains. A type without it starts a fresh memo below itself, which
// costs only memoization: cycles are still caught by objectEqualsVisited.
type stateEqualer interface {
	equalsIn(other Type, s *equalsState) bool
}

// equalsIn compares a and b within the comparison s (nil: a new one).
func equalsIn(a, b Type, s *equalsState) bool {
	if se, ok := a.(stateEqualer); ok {
		return se.equalsIn(b, s)
	}
	return a.Equals(b)
}

// --- Function/Object Signatures ---

// Signature represents a function or constructor signature
type Signature struct {
	TypeParameters    []*TypeParameter
	ParameterNames    []string
	ParameterTypes    []Type
	ReturnType        Type
	OptionalParams    []bool // Tracks which parameters are optional
	IsVariadic        bool   // Indicates if the function accepts variable arguments
	RestParameterType Type   // Type of the rest parameter (...args), if present
	// ThisType is the declared type of an explicit `this` parameter. It
	// constrains the receiver, not the arguments, so it is not a parameter.
	ThisType Type
	// StrictVariance marks a signature whose declaration is not a method, so
	// that with strictFunctionTypes its parameters are compared
	// contravariantly (TypeScript keys this off the target's declaration kind).
	StrictVariance bool
	// IsAbstract marks the construct signature of an abstract class or an
	// `abstract new (...) => T` type: it cannot be called with `new`, nor
	// stand in for a construct signature that is not abstract.
	IsAbstract bool
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
	return sig.equalsIn(other, nil)
}

func (sig *Signature) equalsIn(other *Signature, s *equalsState) bool {
	if sig == nil || other == nil {
		return sig == other
	}
	// Pointer identity check - prevents infinite recursion
	if sig == other {
		return true
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
		if p1 != nil && !equalsIn(p1, p2, s) {
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
	if sig.RestParameterType != nil && !equalsIn(sig.RestParameterType, other.RestParameterType, s) {
		return false
	}

	// Check return type
	if (sig.ReturnType == nil) != (other.ReturnType == nil) {
		return false
	}
	if sig.ReturnType != nil && !equalsIn(sig.ReturnType, other.ReturnType, s) {
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
	Readonly  bool // `readonly [k: K]: V` only permits reading

	// For mapped types: [P in K]: V
	IsMapped       bool   // Whether this is a mapped type pattern
	TypeParameter  string // The type parameter name (e.g., "P" in [P in K])
	ConstraintType Type   // The constraint type (e.g., K in [P in K])

	// Synthetic marks a signature the checker invents for a dynamic computed
	// property name; it is not a declared index signature.
	Synthetic bool
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
	return is.equalsIn(other, nil)
}

func (is *IndexSignature) equalsIn(other *IndexSignature, s *equalsState) bool {
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
		if !equalsIn(is.ConstraintType, other.ConstraintType, s) {
			return false
		}
	} else {
		// For regular index signatures, compare key type
		if !equalsIn(is.KeyType, other.KeyType, s) {
			return false
		}
	}

	return is.ValueType.Equals(other.ValueType)
}

// ObjectType represents the type of an object literal or interface.
type ObjectType struct {
	// Pending marks a class type registered before its declaration is
	// checked; the declaration fills this object in place. Substitution keeps
	// the reference instead of copying it (a copy would stay empty).
	Pending bool

	// Instantiates is the generic this object instantiates with GenericArgs,
	// when the checker made it by instantiation. Substituting into the
	// object re-instantiates the generic with substituted arguments instead
	// of copying every member.
	Instantiates *GenericType

	// ClosedInstance marks an instantiation of a generic whose body names no
	// type parameter but its own: its members are the body's with those
	// replaced by GenericArgs, so only GenericArgs can mention an outer type
	// parameter. Walks looking for type parameters stop at its arguments,
	// like tsc's at a type reference's: a generic naming itself with growing
	// arguments (`next(): Grow<Wrap<T>>`) has no last instantiation.
	ClosedInstance bool

	// Using a map for simplicity now. Consider ordered map or slice for stability.
	Properties         map[string]Type
	OptionalProperties map[string]bool // Tracks which properties are optional
	ReadOnlyProperties map[string]bool // Tracks which properties are readonly

	// PropertyOrder records the order own properties were first added in.
	// Write through SetProperty to keep it in step with Properties; code
	// that fills the map directly still works, those names are listed last.
	PropertyOrder []string

	// PropertyDocs holds the JSDoc comment of each documented own property
	// (delimiters stripped, tags left in the text). Read it with PropertyDoc.
	PropertyDocs map[string]string

	// DisplayName, when set, is how the type prints: `X` for a declared
	// class's instance type, `typeof X` for its constructor value. Clones made
	// by substitution do not carry it, so instantiations print structurally.
	DisplayName string

	// IsFunctionInterface marks the global Function interface, to which
	// every function type is assignable.
	IsFunctionInterface bool

	// GenericName and GenericArgs name an instance of a declared generic
	// class: `Box<string>`. The declared type carries the class's own type
	// parameters as arguments; substitution maps them to the instantiation's.
	GenericName string
	GenericArgs []Type

	// Doc is the JSDoc comment of the declaration that introduced this type
	// (an interface or class), delimiters stripped.
	Doc string

	// NEW: Unified callable/constructor support
	CallSignatures      []*Signature // Object call signatures: obj(args)
	ConstructSignatures []*Signature // Object constructor signatures: new obj(args)
	BaseTypes           []Type       // For inheritance (classes, interfaces)

	// NEW: Class metadata for access control
	ClassMeta *ClassMetadata // Contains access control information for class types

	// Index signatures for dynamic property access
	IndexSignatures []*IndexSignature // Index signatures like [key: string]: Type

	// IsInterface marks the type of an interface declaration. Interfaces do
	// not get an implicit index signature (unlike type literals and object
	// literal types), which matters when relating them to index signatures.
	IsInterface bool

	// lazy, when set, fills in the members on first access (see Materialize):
	// lazySource's, substituted with lazySubstitution.
	lazy             func(*ObjectType)
	lazySource       *ObjectType
	lazySubstitution map[string]Type

	// IsReflectIntrinsic marks this as a compile-time type reflection intrinsic
	// When the checker sees a call to a function with this flag, it resolves the type argument
	// and stores it for the compiler to emit a type descriptor object
	IsReflectIntrinsic bool
}

func (ot *ObjectType) String() string {
	// Detect cycles: if we're already stringifying this exact ObjectType, return a placeholder
	ptr := fmt.Sprintf("%p", ot)
	if _, visited := stringifyVisited.LoadOrStore(ptr, true); visited {
		return "<self>"
	}
	defer stringifyVisited.Delete(ptr)

	// Declared classes print by name, as tsc does.
	if ot.DisplayName != "" {
		return ot.DisplayName
	}
	if ot.GenericName != "" && len(ot.GenericArgs) > 0 {
		args := make([]string, len(ot.GenericArgs))
		for i, a := range ot.GenericArgs {
			args[i] = a.String()
		}
		return ot.GenericName + "<" + strings.Join(args, ", ") + ">"
	}

	// If this is a pure function (callable with no properties), show it as a function signature
	if ot.IsPureFunction() && len(ot.CallSigs()) == 1 {
		return ot.CallSigs()[0].String()
	}

	// If this is a pure constructor, show it as a constructor signature
	if len(ot.Props()) == 0 && len(ot.CallSigs()) == 0 && len(ot.ConstructSigs()) == 1 {
		return fmt.Sprintf("new %s", ot.ConstructSigs()[0].String())
	}

	// Build object type representation
	var parts []string

	// Add call signatures
	for _, sig := range ot.CallSigs() {
		parts = append(parts, sig.String())
	}

	// Add constructor signatures
	for _, sig := range ot.ConstructSigs() {
		parts = append(parts, fmt.Sprintf("new %s", sig.String()))
	}

	// Add properties
	for _, name := range SortedPropertyNames(ot.Props()) {
		typ := ot.Props()[name]
		typStr := "<nil>"
		if typ != nil {
			typStr = typ.String()
		}
		optional := ""
		if ot.OptionalProps() != nil && ot.OptionalProps()[name] {
			optional = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", name, optional, typStr))
	}

	// Add index signatures
	for _, indexSig := range ot.IndexSigs() {
		parts = append(parts, indexSig.String())
	}

	if len(parts) == 0 {
		return "{}"
	}

	return fmt.Sprintf("{ %s }", strings.Join(parts, "; "))
}
func (ot *ObjectType) typeNode() {}
func (ot *ObjectType) Equals(other Type) bool {
	return ot.equalsIn(other, nil)
}

func (ot *ObjectType) equalsIn(other Type, s *equalsState) bool {
	otherOt, ok := other.(*ObjectType)
	if !ok {
		return false // Not an ObjectType
	}
	if ot == nil || otherOt == nil {
		return ot == otherOt // Both must be nil or non-nil
	}

	// Pointer identity check - prevents infinite recursion for self-referencing types
	if ot == otherOt {
		return true
	}

	if ot.ClosedInstance && otherOt.ClosedInstance && ot.Instantiates == otherOt.Instantiates &&
		len(ot.GenericArgs) == len(otherOt.GenericArgs) {
		// Their members are one generic's with these arguments: comparing
		// them member by member follows `next(): Grow<Wrap<T>>` forever.
		for i, a := range ot.GenericArgs {
			if a != otherOt.GenericArgs[i] && !equalsIn(a, otherOt.GenericArgs[i], s) {
				return false
			}
		}
		return true
	}

	pairKey := objectPair{ot, otherOt}
	if s != nil && s.proved[pairKey] {
		return true
	}
	if _, visited := objectEqualsVisited.LoadOrStore(pairKey, true); visited {
		return true
	}
	defer objectEqualsVisited.Delete(pairKey)

	if s == nil {
		// The outermost comparison: its memo dies with it.
		return ot.structurallyEquals(otherOt, &equalsState{})
	}
	if s.depth >= maxEqualsDepth {
		return true
	}
	s.depth++
	logLen := len(s.log)
	equal := ot.structurallyEquals(otherOt, s)
	s.depth--
	if equal {
		if s.proved == nil {
			s.proved = make(map[objectPair]bool)
		}
		s.proved[pairKey] = true
		s.log = append(s.log, pairKey)
	} else {
		for _, proved := range s.log[logLen:] {
			delete(s.proved, proved)
		}
		s.log = s.log[:logLen]
	}
	return equal
}

// structurallyEquals compares members, signatures, bases, class metadata and
// index signatures; Equals handles identity, cycles and memoization.
func (ot *ObjectType) structurallyEquals(otherOt *ObjectType, s *equalsState) bool {
	// Check properties
	if len(ot.Props()) != len(otherOt.Props()) {
		return false // Different number of properties
	}
	for key, t1 := range ot.Props() {
		t2, exists := otherOt.Props()[key]
		if !exists {
			return false // Key missing in other object
		}
		if (t1 == nil) != (t2 == nil) {
			return false
		} // One type is nil, the other isn't
		if t1 != nil && !equalsIn(t1, t2, s) {
			return false // Property types are not equal
		}

		// Check optional property flags
		optional1 := ot.OptionalProps() != nil && ot.OptionalProps()[key]
		optional2 := otherOt.OptionalProps() != nil && otherOt.OptionalProps()[key]
		if optional1 != optional2 {
			return false // Optionality mismatch
		}
	}

	// Check call signatures
	if len(ot.CallSigs()) != len(otherOt.CallSigs()) {
		return false
	}
	for i, sig1 := range ot.CallSigs() {
		sig2 := otherOt.CallSigs()[i]
		if !sig1.equalsIn(sig2, s) {
			return false
		}
	}

	// Check constructor signatures
	if len(ot.ConstructSigs()) != len(otherOt.ConstructSigs()) {
		return false
	}
	for i, sig1 := range ot.ConstructSigs() {
		sig2 := otherOt.ConstructSigs()[i]
		if !sig1.equalsIn(sig2, s) {
			return false
		}
	}

	// Check base types
	if len(ot.Bases()) != len(otherOt.Bases()) {
		return false
	}
	for i, base1 := range ot.Bases() {
		base2 := otherOt.Bases()[i]
		if (base1 == nil) != (base2 == nil) {
			return false
		}
		if base1 != nil && !equalsIn(base1, base2, s) {
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
	if len(ot.IndexSigs()) != len(otherOt.IndexSigs()) {
		return false
	}
	for i, sig1 := range ot.IndexSigs() {
		sig2 := otherOt.IndexSigs()[i]
		if !sig1.equalsIn(sig2, s) {
			return false
		}
	}

	return true // All properties, signatures, base types, class metadata, and index signatures match
}

// --- ObjectType Helper Methods ---

// IsCallable returns true if this ObjectType has call signatures
func (ot *ObjectType) IsCallable() bool {
	return ot != nil && ot.CallSigs() != nil && len(ot.CallSigs()) > 0
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
		for _, baseType := range ot.Bases() {
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
		for _, baseType := range ot.Bases() {
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
	return len(ot.ConstructSigs()) > 0
}

// IsPureFunction returns true if this ObjectType is callable but has no properties
// (i.e., it's a pure function)
func (ot *ObjectType) IsPureFunction() bool {
	return ot.IsCallable() && len(ot.Props()) == 0
}

// GetCallSignatures returns the call signatures of this ObjectType
func (ot *ObjectType) GetCallSignatures() []*Signature {
	return ot.CallSigs()
}

// GetConstructSignatures returns the constructor signatures of this ObjectType
func (ot *ObjectType) GetConstructSignatures() []*Signature {
	return ot.ConstructSigs()
}

// GetEffectiveProperties returns all properties including inherited ones from base types
func (ot *ObjectType) GetEffectiveProperties() map[string]Type {
	result := make(map[string]Type, len(ot.Props()))

	// First, add properties from base types (in reverse order for proper precedence)
	for i := len(ot.Bases()) - 1; i >= 0; i-- {
		baseType := resolveBaseType(ot.Bases()[i])
		if baseObj, ok := baseType.(*ObjectType); ok {
			baseProps := baseObj.GetEffectiveProperties()
			for name, typ := range baseProps {
				result[name] = typ
			}
		}
	}

	// Then add our own properties (these override base type properties)
	for name, typ := range ot.Props() {
		result[name] = typ
	}

	return result
}

// EffectiveProperty looks up one property, own or inherited, with the same
// precedence as GetEffectiveProperties but without building the whole map.
func (ot *ObjectType) EffectiveProperty(name string) (Type, bool) {
	if t, ok := ot.Props()[name]; ok {
		return t, true
	}
	for _, base := range ot.Bases() {
		if baseObj, ok := resolveBaseType(base).(*ObjectType); ok {
			if t, ok := baseObj.EffectiveProperty(name); ok {
				return t, true
			}
		}
	}
	return nil, false
}

// IsPropertyOptional checks whether a property is optional, including inherited properties
func (ot *ObjectType) IsPropertyOptional(name string) bool {
	if ot.OptionalProps() != nil {
		if opt, exists := ot.OptionalProps()[name]; exists {
			return opt
		}
	}
	// A member the type declares itself replaces the inherited declaration,
	// optionality included.
	if _, declared := ot.Props()[name]; declared {
		return false
	}
	// Check base types
	for _, baseType := range ot.Bases() {
		if baseObj, ok := resolveBaseType(baseType).(*ObjectType); ok {
			if baseObj.IsPropertyOptional(name) {
				return true
			}
		}
	}
	return false
}

// AddCallSignature adds a call signature to this ObjectType
func (ot *ObjectType) AddCallSignature(sig *Signature) {
	ot.Materialize()
	ot.CallSignatures = append(ot.CallSigs(), sig)
}

// AddConstructSignature adds a constructor signature to this ObjectType
func (ot *ObjectType) AddConstructSignature(sig *Signature) {
	ot.Materialize()
	ot.ConstructSignatures = append(ot.ConstructSigs(), sig)
}

// AddBaseType adds a base type for inheritance
func (ot *ObjectType) AddBaseType(baseType Type) {
	ot.Materialize()
	ot.BaseTypes = append(ot.Bases(), baseType)
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
	ot.SetProperty(name, propType)
	return ot
}

// WithOptionalProperty adds an optional property to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithOptionalProperty(name string, propType Type) *ObjectType {
	ot.SetProperty(name, propType)
	ot.OptionalProps()[name] = true
	return ot
}

// WithReadOnlyProperty adds a readonly property to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithReadOnlyProperty(name string, propType Type) *ObjectType {
	ot.SetProperty(name, propType)
	if ot.ReadOnlyProps() == nil {
		ot.Materialize()
		ot.ReadOnlyProperties = make(map[string]bool)
	}
	ot.ReadOnlyProps()[name] = true
	return ot
}

// IsReadOnly returns whether a property is readonly
func (ot *ObjectType) IsReadOnly(name string) bool {
	return ot.ReadOnlyProps() != nil && ot.ReadOnlyProps()[name]
}

// WithVariadicProperty adds a variadic method property to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithVariadicProperty(name string, paramTypes []Type, returnType Type, restType Type) *ObjectType {
	methodType := NewObjectType().WithVariadicCallSignature(paramTypes, returnType, restType)
	return ot.WithProperty(name, methodType)
}

// WithCallSignature adds a call signature to the ObjectType and returns the same instance for chaining
func (ot *ObjectType) WithCallSignature(sig *Signature) *ObjectType {
	ot.Materialize()
	ot.CallSignatures = append(ot.CallSigs(), sig)
	return ot
}

// WithSimpleCallSignature adds a simple call signature (params->return) and returns the same instance for chaining
func (ot *ObjectType) WithSimpleCallSignature(paramTypes []Type, returnType Type) *ObjectType {
	if prev := extendableSimpleSignature(ot.CallSigs(), paramTypes, returnType); prev != nil {
		extendWithOptionalParams(prev, paramTypes)
		return ot
	}
	sig := &Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    make([]bool, len(paramTypes)),
		IsVariadic:        false,
		RestParameterType: nil,
	}
	return ot.WithCallSignature(sig)
}

// extendableSimpleSignature returns the last of sigs when paramTypes extends
// its parameters with the same return type: builtins write `f()`, `f(a)`,
// `f(a, b)` for what tsc's lib declares as one signature with optional
// trailing parameters, which (unlike the overloads) also accepts undefined.
func extendableSimpleSignature(sigs []*Signature, paramTypes []Type, returnType Type) *Signature {
	if len(sigs) == 0 {
		return nil
	}
	prev := sigs[len(sigs)-1]
	if prev.IsVariadic || len(prev.TypeParameters) > 0 || prev.ThisType != nil || len(paramTypes) <= len(prev.ParameterTypes) {
		return nil
	}
	if prev.ReturnType != returnType && (prev.ReturnType == nil || returnType == nil || !prev.ReturnType.Equals(returnType)) {
		return nil
	}
	for i, p := range prev.ParameterTypes {
		if p != paramTypes[i] && (p == nil || paramTypes[i] == nil || !p.Equals(paramTypes[i])) {
			return nil
		}
	}
	return prev
}

func extendWithOptionalParams(sig *Signature, paramTypes []Type) {
	optional := make([]bool, len(paramTypes))
	copy(optional, sig.OptionalParams)
	for i := len(sig.ParameterTypes); i < len(paramTypes); i++ {
		optional[i] = true
	}
	sig.ParameterTypes = append(append([]Type(nil), sig.ParameterTypes...), paramTypes[len(sig.ParameterTypes):]...)
	sig.OptionalParams = optional
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
	ot.Materialize()
	ot.ConstructSignatures = append(ot.ConstructSigs(), sig)
	return ot
}

// WithSimpleConstructSignature adds a simple constructor signature and returns the same instance for chaining
func (ot *ObjectType) WithSimpleConstructSignature(paramTypes []Type, returnType Type) *ObjectType {
	if prev := extendableSimpleSignature(ot.ConstructSigs(), paramTypes, returnType); prev != nil {
		extendWithOptionalParams(prev, paramTypes)
		return ot
	}
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
	ot.Materialize()
	ot.BaseTypes = append(ot.Bases(), baseType)
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
	ot.SetProperty(memberName, memberType)
	if isReadonly {
		if ot.ReadOnlyProps() == nil {
			ot.Materialize()
			ot.ReadOnlyProperties = make(map[string]bool)
		}
		ot.ReadOnlyProps()[memberName] = true
	}

	// Add access control metadata
	if ot.ClassMeta != nil {
		ot.ClassMeta.AddMember(memberName, accessLevel, isStatic, isReadonly)
	}

	return ot
}

// WithClassMethod adds a class method: a member declared as a method, which
// (unlike a property) may be read through super before ES2015.
func (ot *ObjectType) WithClassMethod(memberName string, memberType Type, accessLevel AccessModifier, isStatic, isReadonly bool) *ObjectType {
	ot.WithClassMember(memberName, memberType, accessLevel, isStatic, isReadonly)
	if ot.ClassMeta != nil {
		if info := ot.ClassMeta.GetMemberAccess(memberName); info != nil {
			info.IsMethod = true
		}
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

// resolveBaseType turns an instantiated generic base (class C<T>) into its
// concrete object type so that inherited members can be read.
func resolveBaseType(t Type) Type {
	if inst, ok := t.(*InstantiatedType); ok {
		if sub := inst.Substitute(); sub != nil {
			return sub
		}
	}
	return t
}

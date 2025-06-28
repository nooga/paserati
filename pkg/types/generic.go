package types

import (
	"fmt"
	"strings"
)

// TypeParameter represents a generic type parameter (e.g., T in Array<T> or function<T>)
type TypeParameter struct {
	Name       string // The parameter name (e.g., "T", "U", "K", "V")
	Constraint Type   // Optional constraint (e.g., T extends string), nil if unconstrained
	Index      int    // Position in the type parameter list (0-based)
}

func (tp *TypeParameter) String() string {
	if tp.Constraint != nil {
		return fmt.Sprintf("%s extends %s", tp.Name, tp.Constraint.String())
	}
	return tp.Name
}

// TypeParameterType represents a reference to a type parameter within a generic type or function
// This is what gets used inside the generic body (e.g., the "T" in "return x: T")
type TypeParameterType struct {
	Parameter *TypeParameter // Reference to the parameter definition
}

func (t *TypeParameterType) String() string {
	return t.Parameter.Name
}

func (t *TypeParameterType) Equals(other Type) bool {
	if o, ok := other.(*TypeParameterType); ok {
		// Two type parameter types are equal if they reference the same parameter
		return t.Parameter == o.Parameter
	}
	return false
}

func (t *TypeParameterType) typeNode() {}

// GenericType represents a generic type definition (before instantiation)
// This is the "template" that gets instantiated with concrete types
type GenericType struct {
	Name           string           // Name of the generic type (e.g., "Array", "Promise")
	TypeParameters []*TypeParameter // The type parameters (e.g., [T] for Array<T>)
	Body           Type             // The body type that may contain TypeParameterType references
}

func (g *GenericType) String() string {
	paramNames := make([]string, len(g.TypeParameters))
	for i, param := range g.TypeParameters {
		paramNames[i] = param.String()
	}
	return fmt.Sprintf("%s<%s>", g.Name, strings.Join(paramNames, ", "))
}

func (g *GenericType) Equals(other Type) bool {
	if o, ok := other.(*GenericType); ok {
		if g.Name != o.Name || len(g.TypeParameters) != len(o.TypeParameters) {
			return false
		}
		// For now, just compare names and arity
		// Full structural comparison would require checking body types
		return true
	}
	return false
}

func (g *GenericType) typeNode() {}

// InstantiatedType represents a generic type with concrete type arguments
// This is what you get when you write Array<string> - an instantiation of the Array generic
type InstantiatedType struct {
	Generic       *GenericType // The generic type being instantiated
	TypeArguments []Type       // The concrete type arguments (e.g., [string] for Array<string>)
	substituted   Type         // Cached result of substitution (computed lazily)
}

func (i *InstantiatedType) String() string {
	argStrings := make([]string, len(i.TypeArguments))
	for idx, arg := range i.TypeArguments {
		argStrings[idx] = arg.String()
	}
	return fmt.Sprintf("%s<%s>", i.Generic.Name, strings.Join(argStrings, ", "))
}

func (i *InstantiatedType) Equals(other Type) bool {
	if o, ok := other.(*InstantiatedType); ok {
		if !i.Generic.Equals(o.Generic) || len(i.TypeArguments) != len(o.TypeArguments) {
			return false
		}
		for idx, arg := range i.TypeArguments {
			if !arg.Equals(o.TypeArguments[idx]) {
				return false
			}
		}
		return true
	}
	return false
}

func (i *InstantiatedType) typeNode() {}

// Substitute returns the concrete type by replacing type parameters with type arguments
// This is where the "type-level lambda" application happens
func (i *InstantiatedType) Substitute() Type {
	if i.substituted != nil {
		return i.substituted
	}
	
	if len(i.TypeArguments) != len(i.Generic.TypeParameters) {
		// This should be caught earlier, but return Any as fallback
		i.substituted = Any
		return i.substituted
	}
	
	// Build substitution map
	substitutions := make(map[*TypeParameter]Type)
	for idx, param := range i.Generic.TypeParameters {
		substitutions[param] = i.TypeArguments[idx]
	}
	
	// Substitute in the body
	i.substituted = substituteType(i.Generic.Body, substitutions)
	return i.substituted
}

// substituteType performs type parameter substitution in a type
func substituteType(t Type, substitutions map[*TypeParameter]Type) Type {
	if t == nil {
		return nil
	}
	
	switch t := t.(type) {
	case *TypeParameterType:
		// Replace type parameter with concrete type
		if replacement, ok := substitutions[t.Parameter]; ok {
			return replacement
		}
		return t // Not found, return unchanged
		
	case *ArrayType:
		// Recursively substitute in element type
		newElementType := substituteType(t.ElementType, substitutions)
		return &ArrayType{ElementType: newElementType}
		
	case *ObjectType:
		// Deep copy and substitute in properties
		newObj := NewObjectType()
		for name, propType := range t.Properties {
			newObj.Properties[name] = substituteType(propType, substitutions)
		}
		// Copy optional properties
		for name, isOptional := range t.OptionalProperties {
			newObj.OptionalProperties[name] = isOptional
		}
		
		// Handle call signatures
		for _, sig := range t.CallSignatures {
			newSig := substituteSignature(sig, substitutions)
			newObj.CallSignatures = append(newObj.CallSignatures, newSig)
		}
		
		// Handle constructor signatures
		for _, sig := range t.ConstructSignatures {
			newSig := substituteSignature(sig, substitutions)
			newObj.ConstructSignatures = append(newObj.ConstructSignatures, newSig)
		}
		
		// Copy index signatures
		for _, indexSig := range t.IndexSignatures {
			newIndexSig := &IndexSignature{
				KeyType:   substituteType(indexSig.KeyType, substitutions),
				ValueType: substituteType(indexSig.ValueType, substitutions),
			}
			newObj.IndexSignatures = append(newObj.IndexSignatures, newIndexSig)
		}
		
		return newObj
		
	case *UnionType:
		// Substitute in all constituent types
		newTypes := make([]Type, len(t.Types))
		for i, constituent := range t.Types {
			newTypes[i] = substituteType(constituent, substitutions)
		}
		return NewUnionType(newTypes...)
		
	case *IntersectionType:
		// Substitute in all constituent types
		newTypes := make([]Type, len(t.Types))
		for i, constituent := range t.Types {
			newTypes[i] = substituteType(constituent, substitutions)
		}
		return NewIntersectionType(newTypes...)
		
	case *InstantiatedType:
		// Recursively substitute in type arguments
		newArgs := make([]Type, len(t.TypeArguments))
		for i, arg := range t.TypeArguments {
			newArgs[i] = substituteType(arg, substitutions)
		}
		newInstantiated := NewInstantiatedType(t.Generic, newArgs)
		// Return the substituted result, not the InstantiatedType itself
		return newInstantiated.Substitute()
		
	case *ReadonlyType:
		// Substitute in the inner type
		newInnerType := substituteType(t.InnerType, substitutions)
		return NewReadonlyType(newInnerType)
		
	// For primitive types and other types that don't contain type parameters
	default:
		return t
	}
}

// Helper functions for creating generic types

// NewTypeParameter creates a new type parameter
func NewTypeParameter(name string, index int, constraint Type) *TypeParameter {
	return &TypeParameter{
		Name:       name,
		Constraint: constraint,
		Index:      index,
	}
}

// NewGenericType creates a new generic type definition
func NewGenericType(name string, typeParams []*TypeParameter, body Type) *GenericType {
	return &GenericType{
		Name:           name,
		TypeParameters: typeParams,
		Body:           body,
	}
}

// NewInstantiatedType creates a new instantiated generic type
func NewInstantiatedType(generic *GenericType, typeArgs []Type) *InstantiatedType {
	return &InstantiatedType{
		Generic:       generic,
		TypeArguments: typeArgs,
	}
}

// Built-in generic types (these replace our hardcoded approach)

var (
	// Array<T> generic type
	ArrayGeneric *GenericType
	
	// Promise<T> generic type  
	PromiseGeneric *GenericType
)

func init() {
	// Create Array<T> generic type
	arrayT := NewTypeParameter("T", 0, nil)
	arrayBody := &ArrayType{ElementType: &TypeParameterType{Parameter: arrayT}}
	ArrayGeneric = NewGenericType("Array", []*TypeParameter{arrayT}, arrayBody)
	
	// Create Promise<T> generic type
	promiseT := NewTypeParameter("T", 0, nil)
	promiseBody := NewObjectType() // Simplified Promise structure for now
	promiseBody.WithProperty("then", Any)
	promiseBody.WithProperty("catch", Any)
	PromiseGeneric = NewGenericType("Promise", []*TypeParameter{promiseT}, promiseBody)
}

// substituteSignature performs type parameter substitution in a signature
func substituteSignature(sig *Signature, substitutions map[*TypeParameter]Type) *Signature {
	if sig == nil {
		return nil
	}
	
	// Substitute parameter types
	newParamTypes := make([]Type, len(sig.ParameterTypes))
	for i, paramType := range sig.ParameterTypes {
		newParamTypes[i] = substituteType(paramType, substitutions)
	}
	
	// Substitute return type
	newReturnType := substituteType(sig.ReturnType, substitutions)
	
	// Substitute rest parameter type if present
	var newRestParamType Type
	if sig.RestParameterType != nil {
		newRestParamType = substituteType(sig.RestParameterType, substitutions)
	}
	
	return &Signature{
		ParameterTypes:    newParamTypes,
		ReturnType:        newReturnType,
		OptionalParams:    sig.OptionalParams, // Copy as-is
		IsVariadic:        sig.IsVariadic,
		RestParameterType: newRestParamType,
	}
}
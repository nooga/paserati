package builtins

import (
	"paserati/pkg/types"
)

type UtilityTypesInitializer struct{}

func (u *UtilityTypesInitializer) Name() string {
	return "UtilityTypes"
}

func (u *UtilityTypesInitializer) Priority() int {
	return PriorityObject - 1 // Initialize before Object so other types can use utility types
}

func (u *UtilityTypesInitializer) InitTypes(ctx *TypeContext) error {
	// Register built-in utility types as proper mapped types
	
	// Partial<T> = { [P in keyof T]?: T[P] }
	u.registerPartialType(ctx)
	
	// Required<T> = { [P in keyof T]: T[P] }
	u.registerRequiredType(ctx)
	
	// Readonly<T> = { readonly [P in keyof T]: T[P] }
	u.registerReadonlyType(ctx)
	
	// Pick<T, K> = { [P in K]: T[P] }
	u.registerPickType(ctx)
	
	// Omit<T, K> = { [P in Exclude<keyof T, K>]: T[P] }
	u.registerOmitType(ctx)
	
	// Record<K, T> = { [P in K]: T }
	u.registerRecordType(ctx)
	
	// ReturnType<T> = T extends (...args: any[]) => infer R ? R : never
	u.registerReturnType(ctx)
	
	// Parameters<T> = T extends (...args: infer P) => any ? P : never
	u.registerParametersType(ctx)
	
	// ConstructorParameters<T> = T extends new (...args: infer P) => any ? P : never
	u.registerConstructorParametersType(ctx)
	
	// InstanceType<T> = T extends new (...args: any[]) => infer R ? R : any
	u.registerInstanceType(ctx)
	
	return nil
}

func (u *UtilityTypesInitializer) InitRuntime(ctx *RuntimeContext) error {
	// Utility types are compile-time only, no runtime representation needed
	return nil
}

// registerPartialType registers Partial<T> = { [P in keyof T]?: T[P] }
func (u *UtilityTypesInitializer) registerPartialType(ctx *TypeContext) {
	// Create type parameter T
	tParam := types.NewTypeParameter("T", 0, nil)
	
	// Create keyof T
	keyofT := &types.KeyofType{
		OperandType: &types.TypeParameterType{Parameter: tParam},
	}
	
	// Create T[P] (indexed access)
	indexedAccess := &types.IndexedAccessType{
		ObjectType: &types.TypeParameterType{Parameter: tParam},
		IndexType:  &types.TypeParameterType{Parameter: types.NewTypeParameter("P", 1, nil)},
	}
	
	// Create the mapped type { [P in keyof T]?: T[P] }
	mappedType := &types.MappedType{
		TypeParameter:    "P",
		ConstraintType:   keyofT,
		ValueType:        indexedAccess,
		OptionalModifier: "+", // Make properties optional
		ReadonlyModifier: "",  // No readonly modifier
	}
	
	// Create the generic type
	partialGeneric := types.NewGenericType("Partial", []*types.TypeParameter{tParam}, mappedType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("Partial", partialGeneric)
}

// registerRequiredType registers Required<T> = { [P in keyof T]: T[P] }
func (u *UtilityTypesInitializer) registerRequiredType(ctx *TypeContext) {
	// Create type parameter T
	tParam := types.NewTypeParameter("T", 0, nil)
	
	// Create keyof T
	keyofT := &types.KeyofType{
		OperandType: &types.TypeParameterType{Parameter: tParam},
	}
	
	// Create T[P] (indexed access)
	indexedAccess := &types.IndexedAccessType{
		ObjectType: &types.TypeParameterType{Parameter: tParam},
		IndexType:  &types.TypeParameterType{Parameter: types.NewTypeParameter("P", 1, nil)},
	}
	
	// Create the mapped type { [P in keyof T]: T[P] }
	mappedType := &types.MappedType{
		TypeParameter:    "P",
		ConstraintType:   keyofT,
		ValueType:        indexedAccess,
		OptionalModifier: "", // No optional modifier
		ReadonlyModifier: "", // No readonly modifier
	}
	
	// Create the generic type
	requiredGeneric := types.NewGenericType("Required", []*types.TypeParameter{tParam}, mappedType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("Required", requiredGeneric)
}

// registerReadonlyType registers Readonly<T> = { readonly [P in keyof T]: T[P] }
func (u *UtilityTypesInitializer) registerReadonlyType(ctx *TypeContext) {
	// Create type parameter T
	tParam := types.NewTypeParameter("T", 0, nil)
	
	// Create keyof T
	keyofT := &types.KeyofType{
		OperandType: &types.TypeParameterType{Parameter: tParam},
	}
	
	// Create T[P] (indexed access)
	indexedAccess := &types.IndexedAccessType{
		ObjectType: &types.TypeParameterType{Parameter: tParam},
		IndexType:  &types.TypeParameterType{Parameter: types.NewTypeParameter("P", 1, nil)},
	}
	
	// Create the mapped type { readonly [P in keyof T]: T[P] }
	mappedType := &types.MappedType{
		TypeParameter:    "P",
		ConstraintType:   keyofT,
		ValueType:        indexedAccess,
		OptionalModifier: "", // No optional modifier
		ReadonlyModifier: "+", // Make properties readonly
	}
	
	// Create the generic type
	readonlyGeneric := types.NewGenericType("Readonly", []*types.TypeParameter{tParam}, mappedType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("Readonly", readonlyGeneric)
}

// registerPickType registers Pick<T, K> = { [P in K]: T[P] }
func (u *UtilityTypesInitializer) registerPickType(ctx *TypeContext) {
	// Create type parameters T and K
	tParam := types.NewTypeParameter("T", 0, nil)
	kParam := types.NewTypeParameter("K", 1, nil)
	
	// Create T[P] (indexed access)
	indexedAccess := &types.IndexedAccessType{
		ObjectType: &types.TypeParameterType{Parameter: tParam},
		IndexType:  &types.TypeParameterType{Parameter: types.NewTypeParameter("P", 2, nil)},
	}
	
	// Create the mapped type { [P in K]: T[P] }
	mappedType := &types.MappedType{
		TypeParameter:    "P",
		ConstraintType:   &types.TypeParameterType{Parameter: kParam}, // Iterate over K
		ValueType:        indexedAccess,
		OptionalModifier: "", // No optional modifier
		ReadonlyModifier: "", // No readonly modifier
	}
	
	// Create the generic type
	pickGeneric := types.NewGenericType("Pick", []*types.TypeParameter{tParam, kParam}, mappedType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("Pick", pickGeneric)
}

// registerRecordType registers Record<K, T> = { [P in K]: T }
func (u *UtilityTypesInitializer) registerRecordType(ctx *TypeContext) {
	// Create type parameters K and T
	kParam := types.NewTypeParameter("K", 0, nil)
	tParam := types.NewTypeParameter("T", 1, nil)
	
	// Create the mapped type { [P in K]: T }
	mappedType := &types.MappedType{
		TypeParameter:    "P",
		ConstraintType:   &types.TypeParameterType{Parameter: kParam}, // Iterate over K
		ValueType:        &types.TypeParameterType{Parameter: tParam},  // Value type is T
		OptionalModifier: "", // No optional modifier
		ReadonlyModifier: "", // No readonly modifier
	}
	
	// Create the generic type
	recordGeneric := types.NewGenericType("Record", []*types.TypeParameter{kParam, tParam}, mappedType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("Record", recordGeneric)
}

// registerReturnType registers ReturnType<T> = T extends (...args: any[]) => infer R ? R : never
func (u *UtilityTypesInitializer) registerReturnType(ctx *TypeContext) {
	// Create type parameter T
	tParam := types.NewTypeParameter("T", 0, nil)
	
	// Create the function signature pattern: (...args: any[]) => infer R
	// For now, we'll create a simplified version that works with our existing infrastructure
	// This represents: T extends Function ? ReturnTypeOfT : never
	
	// Create a conditional type that checks if T is a function
	// Since we don't have full infer support yet, we'll use a simplified approach
	conditionalType := &types.ConditionalType{
		CheckType:   &types.TypeParameterType{Parameter: tParam},
		ExtendsType: types.Any, // We'll improve this to check for function types
		TrueType:    types.Any, // This should be the inferred return type
		FalseType:   types.Never,
	}
	
	// Create the generic type
	returnTypeGeneric := types.NewGenericType("ReturnType", []*types.TypeParameter{tParam}, conditionalType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("ReturnType", returnTypeGeneric)
}

// registerOmitType registers Omit<T, K> = { [P in Exclude<keyof T, K>]: T[P] }
func (u *UtilityTypesInitializer) registerOmitType(ctx *TypeContext) {
	// Create type parameters T and K
	tParam := types.NewTypeParameter("T", 0, nil)
	kParam := types.NewTypeParameter("K", 1, nil)
	
	// Create keyof T
	keyofT := &types.KeyofType{
		OperandType: &types.TypeParameterType{Parameter: tParam},
	}
	
	// Create Exclude<keyof T, K> as a conditional type
	excludeType := &types.ConditionalType{
		CheckType:   keyofT,
		ExtendsType: &types.TypeParameterType{Parameter: kParam},
		TrueType:    types.Never, // If keyof T extends K, exclude it (never)
		FalseType:   keyofT,      // If keyof T doesn't extend K, keep it
	}
	
	// Create T[P] (indexed access)
	indexedAccess := &types.IndexedAccessType{
		ObjectType: &types.TypeParameterType{Parameter: tParam},
		IndexType:  &types.TypeParameterType{Parameter: types.NewTypeParameter("P", 2, nil)},
	}
	
	// Create the mapped type { [P in Exclude<keyof T, K>]: T[P] }
	mappedType := &types.MappedType{
		TypeParameter:    "P",
		ConstraintType:   excludeType, // Iterate over Exclude<keyof T, K>
		ValueType:        indexedAccess,
		OptionalModifier: "", // No optional modifier
		ReadonlyModifier: "", // No readonly modifier
	}
	
	// Create the generic type
	omitGeneric := types.NewGenericType("Omit", []*types.TypeParameter{tParam, kParam}, mappedType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("Omit", omitGeneric)
}

// registerParametersType registers Parameters<T> = T extends (...args: infer P) => any ? P : never
func (u *UtilityTypesInitializer) registerParametersType(ctx *TypeContext) {
	// Create type parameter T
	tParam := types.NewTypeParameter("T", 0, nil)
	
	// Create infer P for parameters
	inferP := &types.InferType{
		TypeParameter: "P",
	}
	
	// Create function call signature pattern: (...args: infer P) => any
	callSignature := &types.Signature{
		ParameterTypes:    []types.Type{}, // Will be empty, rest parameter handles variadic
		ReturnType:        types.Any,
		OptionalParams:    []bool{},
		IsVariadic:        true,
		RestParameterType: inferP, // This will be inferred as the parameter tuple
	}
	
	// Create function object type with the call signature
	functionType := &types.ObjectType{
		Properties:         map[string]types.Type{},
		OptionalProperties: map[string]bool{},
		CallSignatures:     []*types.Signature{callSignature},
	}
	
	// Create conditional type: T extends (...args: infer P) => any ? P : never
	conditionalType := &types.ConditionalType{
		CheckType:   &types.TypeParameterType{Parameter: tParam},
		ExtendsType: functionType,
		TrueType:    inferP, // Return the inferred parameters as tuple
		FalseType:   types.Never,
	}
	
	// Create the generic type
	parametersGeneric := types.NewGenericType("Parameters", []*types.TypeParameter{tParam}, conditionalType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("Parameters", parametersGeneric)
}

// registerConstructorParametersType registers ConstructorParameters<T> = T extends new (...args: infer P) => any ? P : never
func (u *UtilityTypesInitializer) registerConstructorParametersType(ctx *TypeContext) {
	// Create type parameter T
	tParam := types.NewTypeParameter("T", 0, nil)
	
	// Create infer P for constructor parameters
	inferP := &types.InferType{
		TypeParameter: "P",
	}
	
	// Create constructor signature pattern: new (...args: infer P) => any
	constructSignature := &types.Signature{
		ParameterTypes:    []types.Type{}, // Will be empty, rest parameter handles variadic
		ReturnType:        types.Any,
		OptionalParams:    []bool{},
		IsVariadic:        true,
		RestParameterType: inferP, // This will be inferred as the parameter tuple
	}
	
	// Create constructor object type with the construct signature
	constructorType := &types.ObjectType{
		Properties:         map[string]types.Type{},
		OptionalProperties: map[string]bool{},
		ConstructSignatures: []*types.Signature{constructSignature},
	}
	
	// Create conditional type: T extends new (...args: infer P) => any ? P : never
	conditionalType := &types.ConditionalType{
		CheckType:   &types.TypeParameterType{Parameter: tParam},
		ExtendsType: constructorType,
		TrueType:    inferP, // Return the inferred parameters as tuple
		FalseType:   types.Never,
	}
	
	// Create the generic type
	constructorParametersGeneric := types.NewGenericType("ConstructorParameters", []*types.TypeParameter{tParam}, conditionalType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("ConstructorParameters", constructorParametersGeneric)
}

// registerInstanceType registers InstanceType<T> = T extends new (...args: any[]) => infer R ? R : any
func (u *UtilityTypesInitializer) registerInstanceType(ctx *TypeContext) {
	// Create type parameter T
	tParam := types.NewTypeParameter("T", 0, nil)
	
	// Create infer R for instance type
	inferR := &types.InferType{
		TypeParameter: "R",
	}
	
	// Create any[] for constructor parameters
	anyArray := &types.ArrayType{ElementType: types.Any}
	
	// Create constructor signature pattern: new (...args: any[]) => infer R
	constructSignature := &types.Signature{
		ParameterTypes:    []types.Type{}, // Will be empty, rest parameter handles variadic
		ReturnType:        inferR, // This will be inferred as the instance type
		OptionalParams:    []bool{},
		IsVariadic:        true,
		RestParameterType: anyArray, // Accept any parameters
	}
	
	// Create constructor object type with the construct signature
	constructorType := &types.ObjectType{
		Properties:         map[string]types.Type{},
		OptionalProperties: map[string]bool{},
		ConstructSignatures: []*types.Signature{constructSignature},
	}
	
	// Create conditional type: T extends new (...args: any[]) => infer R ? R : any
	conditionalType := &types.ConditionalType{
		CheckType:   &types.TypeParameterType{Parameter: tParam},
		ExtendsType: constructorType,
		TrueType:    inferR, // Return the inferred instance type
		FalseType:   types.Any, // Fallback to any if not a constructor
	}
	
	// Create the generic type
	instanceTypeGeneric := types.NewGenericType("InstanceType", []*types.TypeParameter{tParam}, conditionalType)
	
	// Register it in the environment
	ctx.DefineTypeAlias("InstanceType", instanceTypeGeneric)
}
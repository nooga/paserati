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
	
	// Omit<T, K> = { [P in Exclude<keyof T, K>]: T[P] } (simplified for now)
	// Record<K, T> = { [P in K]: T }
	u.registerRecordType(ctx)
	
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
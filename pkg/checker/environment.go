package checker

import (
	"fmt"
	"paserati/pkg/builtins"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// Global environment for prototype method resolution
// This is shared with type_utils.go
var globalEnvironment *Environment

// --- NEW: Symbol Information ---
type SymbolInfo struct {
	Type    types.Type
	IsConst bool
}

// WithObject represents an object in a with statement
type WithObject struct {
	ExprType   types.Type            // Type of the with expression
	Properties map[string]types.Type // Known properties and their types
}

// Environment manages type information within scopes.
type Environment struct {
	symbols     map[string]SymbolInfo // UPDATED: Stores SymbolInfo (type + const status)
	typeAliases map[string]types.Type // Stores resolved types for type aliases
	outer       *Environment          // Pointer to the enclosing environment

	// --- Generic type parameter support ---
	typeParameters map[string]*types.TypeParameter // Maps type parameter names to their definitions

	// --- Function overload support ---
	// Maps function names to their collected overload signatures (before implementation is found)
	pendingOverloads map[string][]*parser.FunctionSignature
	// Maps function names to their completed ObjectType with call signatures
	overloadedFunctions map[string]*types.ObjectType

	// --- Primitive prototype registry (only for global environment) ---
	primitivePrototypes map[string]*types.ObjectType // Stores prototype types for primitives

	// --- With statement support ---
	withObjects []WithObject // Stack of objects from enclosing with statements

	// --- Type narrowing support for member expressions ---
	// Maps member expression keys (e.g., "this.value", "obj.prop") to their narrowed types
	narrowings map[string]types.Type
}

// NewEnvironment creates a new top-level type environment.
func NewEnvironment() *Environment {
	return &Environment{
		symbols:             make(map[string]SymbolInfo), // Initialize with SymbolInfo
		typeAliases:         make(map[string]types.Type), // Initialize
		outer:               nil,
		typeParameters:      make(map[string]*types.TypeParameter), // Initialize type parameters
		pendingOverloads:    make(map[string][]*parser.FunctionSignature),
		overloadedFunctions: make(map[string]*types.ObjectType),
		primitivePrototypes: nil, // Only initialized for global environment
		withObjects:         []WithObject{}, // Initialize empty with objects stack
		narrowings:          make(map[string]types.Type), // Initialize narrowings map
	}
}

// NewEnclosedEnvironment creates a new environment nested within an outer one.
func NewEnclosedEnvironment(outer *Environment) *Environment {
	return &Environment{
		symbols:             make(map[string]SymbolInfo), // Initialize with SymbolInfo
		typeAliases:         make(map[string]types.Type), // Initialize
		outer:               outer,
		typeParameters:      make(map[string]*types.TypeParameter), // Initialize type parameters
		pendingOverloads:    make(map[string][]*parser.FunctionSignature),
		overloadedFunctions: make(map[string]*types.ObjectType),
		primitivePrototypes: nil, // Nested environments don't need primitive prototypes
		withObjects:         []WithObject{}, // Initialize empty with objects stack
		narrowings:          make(map[string]types.Type), // Initialize narrowings map
	}
}

// NewGlobalEnvironment creates a new top-level global environment.
// It populates the environment with built-in types using the new initializer system.
func NewGlobalEnvironment(initializers []builtins.BuiltinInitializer) *Environment {
	env := &Environment{
		symbols:             make(map[string]SymbolInfo), // Initialize with SymbolInfo
		typeAliases:         make(map[string]types.Type), // Initialize
		outer:               nil,
		typeParameters:      make(map[string]*types.TypeParameter), // Initialize type parameters
		pendingOverloads:    make(map[string][]*parser.FunctionSignature),
		overloadedFunctions: make(map[string]*types.ObjectType),
		primitivePrototypes: make(map[string]*types.ObjectType), // Initialize for global environment
		withObjects:         []WithObject{}, // Initialize empty with objects stack
		narrowings:          make(map[string]types.Type), // Initialize narrowings map
	}

	// Create type context for builtin initialization
	typeCtx := &builtins.TypeContext{
		DefineGlobal: func(name string, typ types.Type) error {
			if !env.Define(name, typ, true) {
				return fmt.Errorf("global %s already defined", name)
			}
			return nil
		},
		DefineTypeAlias: func(name string, typ types.Type) error {
			if !env.DefineTypeAlias(name, typ) {
				return fmt.Errorf("type alias %s already defined", name)
			}
			return nil
		},
		GetType: func(name string) (types.Type, bool) {
			if info, found := env.symbols[name]; found {
				return info.Type, true
			}
			return nil, false
		},
		SetPrimitivePrototype: func(primitiveName string, prototypeType *types.ObjectType) {
			env.primitivePrototypes[primitiveName] = prototypeType
		},
	}

	// Initialize all builtins using the provided initializers
	for _, init := range initializers {
		if err := init.InitTypes(typeCtx); err != nil {
			fmt.Printf("Warning: failed to initialize %s types: %v\n", init.Name(), err)
		}
	}

	// Set this as the global environment for prototype method resolution
	// Note: This is used by the types package for property resolution
	globalEnvironment = env

	return env
}

// NewStandardGlobalEnvironment creates a new global environment with standard built-in types.
// This is a convenience function that uses the default set of initializers.
func NewStandardGlobalEnvironment() *Environment {
	return NewGlobalEnvironment(builtins.GetStandardInitializers())
}

// GetPrimitivePrototypeMethodType returns the type of a method on a primitive prototype
// This replaces the old builtins.GetPrototypeMethodType function
func (e *Environment) GetPrimitivePrototypeMethodType(primitiveName, methodName string) types.Type {
	// Walk up to find the global environment (which has primitivePrototypes)
	current := e
	for current != nil {
		if current.primitivePrototypes != nil {
			// Found global environment
			if prototypeType, exists := current.primitivePrototypes[primitiveName]; exists {
				if methodType, found := prototypeType.Properties[methodName]; found {
					return methodType
				}
			}
			break
		}
		current = current.outer
	}
	return nil
}

// Define adds a new *variable* type binding and its const status to the current environment scope.
// Returns false if the name conflicts with an existing variable/const in this scope.
// Note: TypeScript-style declaration merging allows the same name to exist as both a value and a type.
func (e *Environment) Define(name string, typ types.Type, isConst bool) bool {
	// Check for conflict with existing variable/constant in this scope
	if _, exists := e.symbols[name]; exists {
		return false // Name already taken by a variable/const
	}
	// Allow coexistence with type aliases (types) - this enables declaration merging for classes
	e.symbols[name] = SymbolInfo{Type: typ, IsConst: isConst}
	return true
}

// --- NEW: Update method ---

// Update modifies the type of an *existing* variable symbol in the current environment scope.
// It does NOT change the IsConst status.
// Returns true if the symbol was found and updated, false otherwise.
func (e *Environment) Update(name string, typ types.Type) bool {
	debugPrintf("// [Env Update] Attempting to update '%s' in env %p with type %T\n", name, e, typ)
	info, exists := e.symbols[name]
	if !exists {
		debugPrintf("// [Env Update] Symbol '%s' not found in env %p\n", name, e)
		return false // Symbol not found in this scope
	}
	// Update the type, keep the original IsConst status
	debugPrintf("// [Env Update] Updating '%s' from %T to %T in env %p\n", name, info.Type, typ, e)
	e.symbols[name] = SymbolInfo{Type: typ, IsConst: info.IsConst}
	return true
}

// DefineTypeAlias adds a new *type alias* binding to the current environment scope.
// Returns false if the alias name conflicts with an existing type alias in this scope.
// Note: TypeScript-style declaration merging allows the same name to exist as both a value and a type.
func (e *Environment) DefineTypeAlias(name string, typ types.Type) bool {
	// Check for conflict with existing type alias in this scope
	if _, exists := e.typeAliases[name]; exists {
		return false
	}
	// Allow coexistence with symbols (values) - this enables declaration merging for classes
	e.typeAliases[name] = typ
	return true
}

// Resolve looks up a *variable* name in the current environment and its outer scopes.
// Returns the type, whether it's constant, and true if found. Otherwise returns nil, false, false.
func (e *Environment) Resolve(name string) (typ types.Type, isConst bool, found bool) {
	// --- DEBUG ---
	if checkerDebug {
		debugPrintf("// [Env Resolve] env=%p, name='%s', outer=%p\n", e, name, e.outer) // Log entry
	}
	if e == nil {
		debugPrintf("// [Env Resolve] ERROR: Attempted to resolve '%s' on nil environment!\n", name)
		// Prevent panic, but this indicates a bug elsewhere.
		return nil, false, false
	}
	if e.symbols == nil {
		debugPrintf("// [Env Resolve] ERROR: env %p has nil symbols map!\n", e)
		// Prevent panic, indicate bug.
		return nil, false, false
	}
	// --- END DEBUG ---

	// Check current scope first
	info, ok := e.symbols[name]
	if ok {
		debugPrintf("// [Env Resolve] Found '%s' in env %p\n", name, e) // DEBUG
		
		// If this is a forward reference, continue looking in outer scopes for the real type
		if _, isForwardRef := info.Type.(*types.ForwardReferenceType); isForwardRef {
			if e.outer != nil {
				debugPrintf("// [Env Resolve] '%s' is a forward reference, checking outer scope for real type...\n", name)
				outerType, outerIsConst, outerFound := e.outer.Resolve(name)
				if outerFound {
					debugPrintf("// [Env Resolve] Outer type for '%s': %T = %s\n", name, outerType, outerType.String())
					// Prefer the outer type if it's not also a forward reference
					if _, isOuterForwardRef := outerType.(*types.ForwardReferenceType); !isOuterForwardRef {
						debugPrintf("// [Env Resolve] Using real type from outer scope for '%s'\n", name)
						return outerType, outerIsConst, true
					} else {
						debugPrintf("// [Env Resolve] Outer type for '%s' is also a forward reference\n", name)
					}
				} else {
					debugPrintf("// [Env Resolve] No outer type found for '%s'\n", name)
				}
			}
		}
		
		return info.Type, info.IsConst, true                            // Return type, const status, and found=true
	}

	// If not found and there's an outer scope, check there recursively
	if e.outer != nil {
		debugPrintf("// [Env Resolve] '%s' not in env %p, checking outer %p...\n", name, e, e.outer) // DEBUG
		return e.outer.Resolve(name)                                                                 // Propagate results from outer scope
	}

	// Not found in any scope
	debugPrintf("// [Env Resolve] '%s' not found in env %p (no outer)\n", name, e) // DEBUG
	return nil, false, false                                                       // Return nil type, isConst=false, found=false
}

// ResolveType looks up a *type name* (could be alias or primitive) in the current environment and its outer scopes.
// Returns the resolved type and true if found, otherwise nil and false.
func (e *Environment) ResolveType(name string) (types.Type, bool) {
	// --- DEBUG ---
	debugPrintf("// [Env ResolveType] env=%p, name='%s', outer=%p\n", e, name, e.outer)
	if e == nil {
		return nil, false
	} // Safety
	if e.typeAliases == nil {
		debugPrintf("// [Env ResolveType] ERROR: env %p has nil typeAliases map!\n", e)
		return nil, false
	}
	// --- END DEBUG ---

	// 1. Check type aliases in current scope
	typ, ok := e.typeAliases[name]
	if ok {
		return typ, true
	}

	// 2. If not found in current aliases, check outer scopes recursively
	if e.outer != nil {
		debugPrintf("// [Env ResolveType] Alias '%s' not in env %p, checking outer %p...\n", name, e, e.outer)
		return e.outer.ResolveType(name)
	}

	// 3. If not found in any alias scope, check built-in primitives (only at global level?)
	//    (This check is actually done in the Checker's resolveTypeAnnotation after trying env.ResolveType)

	debugPrintf("// [Env ResolveType] Alias '%s' not found in env %p (no outer)\n", name, e)
	return nil, false
}

// GetAllTypeAliases returns all type aliases in the current environment (not including outer scopes)
func (e *Environment) GetAllTypeAliases() map[string]types.Type {
	if e.typeAliases == nil {
		return make(map[string]types.Type)
	}
	
	// Create a copy to avoid external modifications
	result := make(map[string]types.Type)
	for name, typ := range e.typeAliases {
		result[name] = typ
	}
	return result
}

// GetAllVariables returns all variables in the current environment (not including outer scopes)
func (e *Environment) GetAllVariables() map[string]SymbolInfo {
	if e.symbols == nil {
		return make(map[string]SymbolInfo)
	}
	
	// Create a copy to avoid external modifications
	result := make(map[string]SymbolInfo)
	for name, symbolInfo := range e.symbols {
		result[name] = symbolInfo
	}
	return result
}

// --- NEW: Function Overload Support ---

// AddOverloadSignature adds a function signature to the pending overloads for the given function name.
func (e *Environment) AddOverloadSignature(name string, sig *parser.FunctionSignature) {
	if e.pendingOverloads == nil {
		e.pendingOverloads = make(map[string][]*parser.FunctionSignature)
	}
	e.pendingOverloads[name] = append(e.pendingOverloads[name], sig)
}

// GetPendingOverloads returns the pending overload signatures for the given function name.
func (e *Environment) GetPendingOverloads(name string) []*parser.FunctionSignature {
	if e.pendingOverloads == nil {
		return nil
	}
	return e.pendingOverloads[name]
}

// CompleteOverloadedFunction creates a unified ObjectType from pending signatures,
// then stores it and clears the pending overloads.
func (e *Environment) CompleteOverloadedFunction(name string, overloadSignatures []*types.Signature) bool {
	// Create the unified overloaded function type as ObjectType with multiple call signatures
	overloadedFunc := &types.ObjectType{
		Properties:     make(map[string]types.Type),
		CallSignatures: overloadSignatures,
	}

	// Store it
	if e.overloadedFunctions == nil {
		e.overloadedFunctions = make(map[string]*types.ObjectType)
	}
	e.overloadedFunctions[name] = overloadedFunc

	// Clear pending overloads for this function
	if e.pendingOverloads != nil {
		delete(e.pendingOverloads, name)
	}

	// Define the function in the environment with the unified overloaded type
	return e.Update(name, overloadedFunc)
}

// --- NEW: Unified Type System Helpers ---

// CompleteOverloadedFunctionUTS creates an ObjectType with multiple call signatures
// from pending signatures and implementation, then stores it and clears the pending overloads.
// This is the UTS replacement for CompleteOverloadedFunction.
func (e *Environment) CompleteOverloadedFunctionUTS(name string, overloadSignatures []*types.Signature, implementation *types.Signature) bool {
	// Create a new ObjectType with all call signatures
	obj := &types.ObjectType{
		Properties:     make(map[string]types.Type),
		CallSignatures: append(overloadSignatures, implementation),
	}

	// Clear pending overloads for this function
	if e.pendingOverloads != nil {
		delete(e.pendingOverloads, name)
	}

	// Define the function in the environment with the unified object type
	return e.Update(name, obj)
}

// ResolveOverloadedFunction looks up an overloaded function by name in this environment and outer scopes.
func (e *Environment) ResolveOverloadedFunction(name string) (*types.ObjectType, bool) {
	// Check current scope
	if e.overloadedFunctions != nil {
		if overloaded, exists := e.overloadedFunctions[name]; exists {
			return overloaded, true
		}
	}

	// Check outer scopes
	if e.outer != nil {
		return e.outer.ResolveOverloadedFunction(name)
	}

	return nil, false
}

// IsOverloadedFunction checks if a function name has overloads (either pending or completed).
func (e *Environment) IsOverloadedFunction(name string) bool {
	// Check completed overloads (legacy or unified)
	if e.overloadedFunctions != nil {
		if _, exists := e.overloadedFunctions[name]; exists {
			return true
		}
	}

	// Check if the function has multiple call signatures (indicating overloads)
	if e.overloadedFunctions != nil {
		if unified, exists := e.overloadedFunctions[name]; exists {
			// Verify it actually has multiple call signatures
			if len(unified.CallSignatures) > 1 {
				return true
			}
		}
	}

	// Check pending overloads
	if e.pendingOverloads != nil {
		if sigs := e.pendingOverloads[name]; len(sigs) > 0 {
			return true
		}
	}

	// Check outer scopes
	if e.outer != nil {
		return e.outer.IsOverloadedFunction(name)
	}

	return false
}

// --- Type Parameter Management ---

// DefineTypeParameter defines a type parameter in the current scope.
// Returns true if successful, false if the parameter name already exists.
func (e *Environment) DefineTypeParameter(name string, param *types.TypeParameter) bool {
	if e.typeParameters == nil {
		e.typeParameters = make(map[string]*types.TypeParameter)
	}

	// Check if type parameter already exists in current scope
	if _, exists := e.typeParameters[name]; exists {
		return false
	}

	e.typeParameters[name] = param
	return true
}

// ResolveTypeParameter looks up a type parameter by name.
// It searches the current scope and outer scopes.
// Returns the TypeParameter and true if found, nil and false otherwise.
func (e *Environment) ResolveTypeParameter(name string) (*types.TypeParameter, bool) {
	// Check current scope
	if e.typeParameters != nil {
		if param, exists := e.typeParameters[name]; exists {
			return param, true
		}
	}

	// Check outer scopes
	if e.outer != nil {
		return e.outer.ResolveTypeParameter(name)
	}

	return nil, false
}

// IsTypeParameterInScope checks if a type parameter name is currently in scope.
func (e *Environment) IsTypeParameterInScope(name string) bool {
	_, found := e.ResolveTypeParameter(name)
	return found
}

// GetCurrentScopeTypeParameters returns all type parameters defined in the current scope.
// This is useful for creating generic function types.
func (e *Environment) GetCurrentScopeTypeParameters() map[string]*types.TypeParameter {
	if e.typeParameters == nil {
		return make(map[string]*types.TypeParameter)
	}

	// Return a copy to prevent external modification
	result := make(map[string]*types.TypeParameter)
	for name, param := range e.typeParameters {
		result[name] = param
	}
	return result
}

// ClearTypeParameters removes all type parameters from the current scope.
// This is useful when exiting a generic function.
func (e *Environment) ClearTypeParameters() {
	if e.typeParameters != nil {
		e.typeParameters = make(map[string]*types.TypeParameter)
	}
}

// --- With statement support methods ---

// PushWithObject adds a new with object to the current environment's stack
func (e *Environment) PushWithObject(withObj WithObject) {
	e.withObjects = append(e.withObjects, withObj)
}

// PopWithObject removes the most recent with object from the stack
func (e *Environment) PopWithObject() {
	if len(e.withObjects) > 0 {
		e.withObjects = e.withObjects[:len(e.withObjects)-1]
	}
}

// ResolveWithFallback tries to resolve an identifier, checking with objects if not found as a variable
// Returns: (type, isFromWith, found)
func (e *Environment) ResolveWithFallback(name string) (types.Type, bool, bool) {
	debugPrintf("// [Env ResolveWithFallback] Looking for '%s', withObjects count: %d\n", name, len(e.withObjects))
	
	// First try normal variable resolution
	if info, found := e.symbols[name]; found {
		debugPrintf("// [Env ResolveWithFallback] Found '%s' as regular variable\n", name)
		return info.Type, false, true
	}

	// If not found in current scope, check with objects
	for i := len(e.withObjects) - 1; i >= 0; i-- {
		withObj := e.withObjects[i]
		debugPrintf("// [Env ResolveWithFallback] Checking with object %d for '%s', properties: %d\n", i, name, len(withObj.Properties))
		if propType, exists := withObj.Properties[name]; exists {
			debugPrintf("// [Env ResolveWithFallback] Found '%s' in with object as property type: %s\n", name, propType.String())
			return propType, true, true
		}
	}

	// If not found in current scope's with objects, check outer scope
	if e.outer != nil {
		return e.outer.ResolveWithFallback(name)
	}

	debugPrintf("// [Env ResolveWithFallback] '%s' not found anywhere\n", name)
	return nil, false, false
}

package checker

import (
	"fmt"
	"paserati/pkg/builtins"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// --- NEW: Symbol Information ---
type SymbolInfo struct {
	Type    types.Type
	IsConst bool
}

// Environment manages type information within scopes.
type Environment struct {
	symbols     map[string]SymbolInfo // UPDATED: Stores SymbolInfo (type + const status)
	typeAliases map[string]types.Type // Stores resolved types for type aliases
	outer       *Environment          // Pointer to the enclosing environment

	// --- Function overload support ---
	// Maps function names to their collected overload signatures (before implementation is found)
	pendingOverloads map[string][]*parser.FunctionSignature
	// Maps function names to their completed ObjectType with call signatures
	overloadedFunctions map[string]*types.ObjectType
}

// NewEnvironment creates a new top-level type environment.
func NewEnvironment() *Environment {
	return &Environment{
		symbols:                   make(map[string]SymbolInfo), // Initialize with SymbolInfo
		typeAliases:               make(map[string]types.Type), // Initialize
		outer:                     nil,
		pendingOverloads:    make(map[string][]*parser.FunctionSignature),
		overloadedFunctions: make(map[string]*types.ObjectType),
	}
}

// NewEnclosedEnvironment creates a new environment nested within an outer one.
func NewEnclosedEnvironment(outer *Environment) *Environment {
	return &Environment{
		symbols:                   make(map[string]SymbolInfo), // Initialize with SymbolInfo
		typeAliases:               make(map[string]types.Type), // Initialize
		outer:               outer,
		pendingOverloads:    make(map[string][]*parser.FunctionSignature),
		overloadedFunctions: make(map[string]*types.ObjectType),
	}
}

// NewGlobalEnvironment creates a new top-level global environment.
// It populates the environment with built-in types.
func NewGlobalEnvironment() *Environment {
	builtins.InitializeRegistry()
	env := &Environment{
		symbols:                   make(map[string]SymbolInfo), // Initialize with SymbolInfo
		typeAliases:               make(map[string]types.Type), // Initialize
		outer:               nil,
		pendingOverloads:    make(map[string][]*parser.FunctionSignature),
		overloadedFunctions: make(map[string]*types.ObjectType),
	}

	// Define built-in primitive types (if not already globally available elsewhere)
	// Example: env.Define("number", types.Number) // Assuming types.Number is a Type itself representing the primitive
	// env.Define("string", types.String)
	// env.Define("boolean", types.Boolean)
	// ... etc

	// Populate with built-in function types
	builtinTypes := builtins.GetAllTypes()
	for name, typ := range builtinTypes {
		if !env.Define(name, typ, true) {
			// This should ideally not happen if names are unique and Define works correctly
			fmt.Printf("Warning: Failed to define built-in '%s' in global environment (already exists?).\n", name)
		} else {
			// fmt.Printf("Checker Env: Defined built-in '%s' with type %s\n", name, typ.String()) // Debug print
		}
	}

	return env
}

// Define adds a new *variable* type binding and its const status to the current environment scope.
// Returns false if the name conflicts with an existing variable/const in this scope.
func (e *Environment) Define(name string, typ types.Type, isConst bool) bool {
	// Check for conflict with existing variable/constant in this scope
	if _, exists := e.symbols[name]; exists {
		return false // Name already taken by a variable/const
	}
	// Check for conflict with existing type alias in this scope
	if _, exists := e.typeAliases[name]; exists {
		return false // Name already taken by a type alias
	}
	e.symbols[name] = SymbolInfo{Type: typ, IsConst: isConst}
	return true
}

// --- NEW: Update method ---

// Update modifies the type of an *existing* variable symbol in the current environment scope.
// It does NOT change the IsConst status.
// Returns true if the symbol was found and updated, false otherwise.
func (e *Environment) Update(name string, typ types.Type) bool {
	info, exists := e.symbols[name]
	if !exists {
		return false // Symbol not found in this scope
	}
	// Update the type, keep the original IsConst status
	e.symbols[name] = SymbolInfo{Type: typ, IsConst: info.IsConst}
	return true
}

// DefineTypeAlias adds a new *type alias* binding to the current environment scope.
// Returns false if the alias name conflicts with an existing variable OR type alias in this scope.
func (e *Environment) DefineTypeAlias(name string, typ types.Type) bool {
	// Check for conflict with existing variable/constant in this scope
	if _, exists := e.symbols[name]; exists {
		return false
	}
	// Check for conflict with existing type alias in this scope
	if _, exists := e.typeAliases[name]; exists {
		return false
	}
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
		debugPrintf("// [Env ResolveType] Found alias '%s' in env %p\n", name, e)
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

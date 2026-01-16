package compiler

import "fmt"

// Register represents a VM register. Using byte for now, max 256 registers.

// Symbol represents an entry in the symbol table.
type Symbol struct {
	Name        string
	Register    Register // The register allocated for this symbol in its scope (only for locals)
	IsGlobal    bool     // True if this is a global variable
	GlobalIndex uint16   // Index in global array (only valid if IsGlobal is true)
	// Spill support: when register pressure is too high, variables can be spilled to memory
	IsSpilled  bool   // True if this variable has been spilled to spillSlots
	SpillIndex uint16 // Index in the function's spillSlots array (only valid if IsSpilled)
	// NFE binding support: named function expression bindings are immutable
	IsImmutable bool // True if this is an NFE binding (assignments are silently ignored in non-strict)
	// TDZ support: let/const variables are in TDZ until initialized
	IsTDZ bool // True if this is a let/const variable that hasn't been initialized yet
	// Const support: const variables cannot be reassigned (throws TypeError)
	IsConst bool // True if this is a const variable
}

// WithObjectInfo tracks information about a with object in the compiler
type WithObjectInfo struct {
	ObjectRegister Register        // Register containing the with object
	Properties     map[string]bool // Set of known properties (true if property exists)
}

// SymbolTable manages symbols for a single scope.
type SymbolTable struct {
	Outer *SymbolTable      // Pointer to the symbol table of the enclosing scope
	store map[string]Symbol // Stores symbols defined in *this* scope

	// --- With statement support ---
	withObjects []WithObjectInfo // Stack of with objects in this scope
}

// NewSymbolTable creates a new, top-level symbol table (global scope).
func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		Outer:       nil, // No outer scope for the global table
		store:       make(map[string]Symbol),
		withObjects: []WithObjectInfo{}, // Initialize empty with objects stack
	}
}

// NewEnclosedSymbolTable creates a new symbol table enclosed by an outer scope.
func NewEnclosedSymbolTable(outer *SymbolTable) *SymbolTable {
	return &SymbolTable{
		Outer:       outer, // Link to the enclosing scope
		store:       make(map[string]Symbol),
		withObjects: []WithObjectInfo{}, // Initialize empty with objects stack
	}
}

// Define adds a new symbol to the *current* scope's table.
// It does not check outer scopes. Assumes the symbol is being defined in this scope.
func (st *SymbolTable) Define(name string, reg Register) Symbol {
	symbol := Symbol{Name: name, Register: reg, IsGlobal: false}
	st.store[name] = symbol
	return symbol
}

// DefineGlobal adds a new global symbol to the *current* scope's table.
func (st *SymbolTable) DefineGlobal(name string, globalIndex uint16) Symbol {
	symbol := Symbol{Name: name, IsGlobal: true, GlobalIndex: globalIndex}
	st.store[name] = symbol
	return symbol
}

// DefineSpilled adds a new spilled symbol to the *current* scope's table.
// Used when register allocation fails and the variable is stored in a spill slot.
func (st *SymbolTable) DefineSpilled(name string, spillIndex uint16) Symbol {
	symbol := Symbol{Name: name, IsGlobal: false, IsSpilled: true, SpillIndex: spillIndex}
	st.store[name] = symbol
	return symbol
}

// DefineImmutable adds a new immutable symbol to the *current* scope's table.
// Used for Named Function Expression (NFE) bindings where assignments should be silently ignored.
func (st *SymbolTable) DefineImmutable(name string, reg Register) Symbol {
	symbol := Symbol{Name: name, Register: reg, IsGlobal: false, IsImmutable: true}
	st.store[name] = symbol
	return symbol
}

// DefineTDZ adds a new let symbol that's in the Temporal Dead Zone.
// The symbol cannot be accessed until InitializeTDZ is called.
func (st *SymbolTable) DefineTDZ(name string, reg Register) Symbol {
	symbol := Symbol{Name: name, Register: reg, IsGlobal: false, IsTDZ: true}
	st.store[name] = symbol
	return symbol
}

// DefineConst adds a new const symbol (not in TDZ).
// Use this for cases where the variable is immediately initialized (e.g., for-of/for-in loops).
func (st *SymbolTable) DefineConst(name string, reg Register) Symbol {
	symbol := Symbol{Name: name, Register: reg, IsGlobal: false, IsConst: true}
	st.store[name] = symbol
	return symbol
}

// DefineConstTDZ adds a new const symbol that's in the Temporal Dead Zone.
// The symbol cannot be accessed until InitializeTDZ is called, and cannot be reassigned.
func (st *SymbolTable) DefineConstTDZ(name string, reg Register) Symbol {
	symbol := Symbol{Name: name, Register: reg, IsGlobal: false, IsTDZ: true, IsConst: true}
	st.store[name] = symbol
	return symbol
}

// DefineTDZSpilled adds a new spilled let symbol that's in the Temporal Dead Zone.
// Used when register allocation fails and the variable is stored in a spill slot.
func (st *SymbolTable) DefineTDZSpilled(name string, spillIndex uint16) Symbol {
	symbol := Symbol{Name: name, IsGlobal: false, IsSpilled: true, SpillIndex: spillIndex, IsTDZ: true}
	st.store[name] = symbol
	return symbol
}

// DefineConstTDZSpilled adds a new spilled const symbol that's in the Temporal Dead Zone.
// Used when register allocation fails and the variable is stored in a spill slot.
func (st *SymbolTable) DefineConstTDZSpilled(name string, spillIndex uint16) Symbol {
	symbol := Symbol{Name: name, IsGlobal: false, IsSpilled: true, SpillIndex: spillIndex, IsTDZ: true, IsConst: true}
	st.store[name] = symbol
	return symbol
}

// InitializeTDZ marks a TDZ symbol as initialized, allowing it to be accessed.
// This should be called when the let/const declaration is actually executed.
func (st *SymbolTable) InitializeTDZ(name string) {
	if symbol, ok := st.store[name]; ok {
		symbol.IsTDZ = false
		st.store[name] = symbol
	}
}

// Resolve looks up a symbol name starting from the current scope and traversing
// up through outer scopes until found or the global scope is reached.
// It returns the found symbol, the table it was found in, and a boolean indicating success.
func (st *SymbolTable) Resolve(name string) (Symbol, *SymbolTable, bool) {
	symbol, ok := st.store[name]
	if ok {
		return symbol, st, true // Found in the current scope
	}

	// If not found in current scope and there's an outer scope, search there
	if st.Outer != nil {
		// Important: Recursively call Resolve on the outer scope
		// AND return the original defining table from the recursive call.
		symbol, definingTable, ok := st.Outer.Resolve(name)
		if ok {
			return symbol, definingTable, true // Found in an outer scope
		}
	}

	// Not found in this scope or any outer scope
	return Symbol{}, nil, false
}

// UpdateRegister modifies the register associated with an existing symbol in the current scope.
// Panics if the symbol is not found in the current scope.
func (st *SymbolTable) UpdateRegister(name string, newRegister Register) {
	symbol, ok := st.store[name]
	if !ok {
		// This should ideally not happen if Define/Resolve logic is correct before calling Update
		panic(fmt.Sprintf("Symbol '%s' not found in current scope for register update", name))
	}
	symbol.Register = newRegister
	st.store[name] = symbol // Reassign the modified struct back to the map
}

// MarkSpilled marks a symbol as spilled to a spill slot.
// The symbol's original register can be reused after spilling.
func (st *SymbolTable) MarkSpilled(name string, spillIndex uint16) {
	symbol, ok := st.store[name]
	if !ok {
		panic(fmt.Sprintf("Symbol '%s' not found in current scope for spill marking", name))
	}
	symbol.IsSpilled = true
	symbol.SpillIndex = spillIndex
	st.store[name] = symbol
}

// --- With statement support methods ---

// PushWithObject adds a new with object to the current scope's stack
func (st *SymbolTable) PushWithObject(objectReg Register, properties map[string]bool) {
	withInfo := WithObjectInfo{
		ObjectRegister: objectReg,
		Properties:     properties,
	}
	st.withObjects = append(st.withObjects, withInfo)
}

// PopWithObject removes the most recent with object from the stack
func (st *SymbolTable) PopWithObject() {
	if len(st.withObjects) > 0 {
		st.withObjects = st.withObjects[:len(st.withObjects)-1]
	}
}

// HasActiveWithObjects returns true if there are any with objects in the current scope chain
func (st *SymbolTable) HasActiveWithObjects() bool {
	// Check current scope
	if len(st.withObjects) > 0 {
		return true
	}

	// Check outer scopes
	if st.Outer != nil {
		return st.Outer.HasActiveWithObjects()
	}

	return false
}

// ResolveWithProperty tries to find a property in the with objects stack
// Returns the register containing the object and true if found
func (st *SymbolTable) ResolveWithProperty(name string) (Register, bool) {
	// Check with objects from innermost to outermost
	for i := len(st.withObjects) - 1; i >= 0; i-- {
		withInfo := st.withObjects[i]
		// If we have specific property information, check it
		if len(withInfo.Properties) > 0 {
			if withInfo.Properties[name] {
				return withInfo.ObjectRegister, true
			}
		} else {
			// If no specific properties (e.g., 'any' type), assume it might have the property
			return withInfo.ObjectRegister, true
		}
	}

	// If not found in current scope, check outer scope
	if st.Outer != nil {
		return st.Outer.ResolveWithProperty(name)
	}

	return 0, false
}

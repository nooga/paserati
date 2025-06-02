package compiler

import "fmt"

// Register represents a VM register. Using byte for now, max 256 registers.

// Symbol represents an entry in the symbol table.
type Symbol struct {
	Name        string
	Register    Register // The register allocated for this symbol in its scope (only for locals)
	IsGlobal    bool     // True if this is a global variable
	GlobalIndex uint16   // Index in global array (only valid if IsGlobal is true)
	// Add ScopeType (Global, Local, Free, Function, etc.) if needed later
	// Add Index for things like builtins or free vars if needed later
}

// SymbolTable manages symbols for a single scope.
type SymbolTable struct {
	Outer *SymbolTable      // Pointer to the symbol table of the enclosing scope
	store map[string]Symbol // Stores symbols defined in *this* scope
}

// NewSymbolTable creates a new, top-level symbol table (global scope).
func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		Outer: nil, // No outer scope for the global table
		store: make(map[string]Symbol),
	}
}

// NewEnclosedSymbolTable creates a new symbol table enclosed by an outer scope.
func NewEnclosedSymbolTable(outer *SymbolTable) *SymbolTable {
	return &SymbolTable{
		Outer: outer, // Link to the enclosing scope
		store: make(map[string]Symbol),
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

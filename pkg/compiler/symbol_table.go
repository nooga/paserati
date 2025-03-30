// ... Resolve method ...

// UpdateRegister modifies the register associated with an existing symbol in the current scope.
// Panics if the symbol is not found in the current scope.
func (st *SymbolTable) UpdateRegister(name string, newRegister Register) {
	symbol, ok := st.store[name]
	if !ok {
		// This should ideally not happen with the current compiler logic,
		// but it's good to have a check.
		panic(fmt.Sprintf("Symbol %s not found in current scope for register update", name))
	}
	symbol.Register = newRegister
	st.store[name] = symbol // Reassign the modified struct back to the map
}

// --- Scope Management ---
// ... NewEnclosedSymbolTable ...

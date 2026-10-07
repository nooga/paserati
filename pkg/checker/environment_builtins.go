package checker

import "github.com/nooga/paserati/pkg/types"

// Built-in globals (Object, Array, ...) live in the global environment, but a
// module has its own top-level scope: `export class Object {}` declares a new
// binding that shadows the global one rather than clashing with it. The
// environment remembers which names it received from the built-in initializers
// so that a module declaration can replace them.

// snapshotBuiltins records every name currently bound as a built-in.
func (e *Environment) snapshotBuiltins() {
	e.builtinNames = make(map[string]bool, len(e.symbols)+len(e.typeAliases))
	for name := range e.symbols {
		e.builtinNames[name] = true
	}
	for name := range e.typeAliases {
		e.builtinNames[name] = true
	}
}

// shadowBuiltin removes the built-in binding for name (value and type) so a
// module-level declaration can take its place. It reports whether name was a
// built-in; user declarations are never touched.
func (e *Environment) shadowBuiltin(name string) bool {
	if !e.builtinNames[name] {
		return false
	}
	delete(e.builtinNames, name)
	delete(e.symbols, name)
	delete(e.typeAliases, name)
	return true
}

// setTypeAlias overwrites the type alias bound to name in this scope.
func (e *Environment) setTypeAlias(name string, typ types.Type) {
	e.typeAliases[name] = typ
}

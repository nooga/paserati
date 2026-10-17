package vm

import "strings"

// Global var and function bindings live in heap slots, but ECMAScript also
// makes them own data properties of the global object (the global
// environment's object record). Each such slot is mirrored onto the global
// object as an ordinary data property kept current by the heap's slot watch
// (see module_namespace.go), so descriptors, key listing and
// hasOwnProperty see it like any other property.

// Flags of OpDeclareGlobal.
const (
	DeclareGlobalFunction  = 1 << 0 // CreateGlobalFunctionBinding (else CreateGlobalVarBinding)
	DeclareGlobalDeletable = 1 << 1 // eval code: the binding is configurable
	DeclareGlobalCheck     = 1 << 2 // only check CanDeclareGlobalFunction / CanDeclareGlobalVar
)

// mirrorGlobalVar makes heap slot idx (global var name) an own data property
// of the global object kept in sync with the slot. A property the global
// object already has keeps its attributes; otherwise one is defined as
// writable, enumerable, and configurable only if deletable.
func (vm *VM) mirrorGlobalVar(idx int, name string, deletable bool) {
	g := vm.GlobalObject
	if g == nil || name == "" || strings.Contains(name, "\x00") {
		return
	}
	val, ok := vm.heap.Get(idx)
	if !ok || val.typ == TypeUninitialized {
		val = Undefined
	}
	if _, exists := g.GetOwn(name); exists {
		g.SetOwn(name, val)
	} else {
		w, e, c := true, true, deletable
		g.DefineOwnProperty(name, val, &w, &e, &c)
	}
	vm.heap.watchGlobal(idx, g, name)
}

// unmirrorGlobalVar drops the mirror of a deleted heap slot.
func (vm *VM) unmirrorGlobalVar(idx int) {
	if vm.GlobalObject == nil {
		return
	}
	if name := vm.heap.unwatchGlobal(idx, vm.GlobalObject); name != "" {
		vm.GlobalObject.DeleteOwn(name)
	}
}

// reflectScriptVars mirrors a script's var and function declarations
// (chunk.VarGlobalIndices, already marked non-configurable) onto the global
// object. Module top-level bindings are not global object properties.
func (vm *VM) reflectScriptVars(chunk *Chunk) {
	if chunk.IsModule || len(chunk.VarGlobalIndices) == 0 || vm.GlobalObject == nil {
		return
	}
	isVar := make(map[int]bool, len(chunk.VarGlobalIndices))
	for _, idx := range chunk.VarGlobalIndices {
		isVar[int(idx)] = true
	}
	for name, idx := range vm.heap.GetNameToIndex() {
		if isVar[idx] {
			vm.mirrorGlobalVar(idx, name, false)
		}
	}
}

// canDeclareGlobalFunction is CanDeclareGlobalFunction (ECMA-262
// 9.1.1.4.16): an existing non-configurable property must be a writable,
// enumerable data property.
func (vm *VM) canDeclareGlobalFunction(name string) bool {
	g := vm.GlobalObject
	if g == nil {
		return true
	}
	i := g.shape.lookupStringField(name)
	if i < 0 {
		return g.IsExtensible()
	}
	f := g.shape.fields[i]
	return f.configurable || (!f.isAccessor && f.writable && f.enumerable)
}

// canDeclareGlobalVar is CanDeclareGlobalVar (ECMA-262 9.1.1.4.15).
func (vm *VM) canDeclareGlobalVar(idx int, name string) bool {
	g := vm.GlobalObject
	if g == nil || vm.heap.IsInitialized(idx) {
		return true
	}
	if _, exists := g.GetOwn(name); exists {
		return true
	}
	return g.IsExtensible()
}

// declareGlobal implements CreateGlobalVarBinding and
// CreateGlobalFunctionBinding (ECMA-262 9.1.1.4.17-18) for eval code, where
// the binding may already exist - as an earlier declaration's heap slot or
// as a property of the global object.
func (vm *VM) declareGlobal(idx int, name string, flags byte, value Value) {
	h := vm.heap
	g := vm.GlobalObject
	deletable := flags&DeclareGlobalDeletable != 0
	if flags&DeclareGlobalFunction == 0 {
		// An existing binding keeps its value and attributes.
		if h.IsInitialized(idx) {
			vm.mirrorGlobalVar(idx, name, deletable)
			return
		}
		if g != nil {
			if v, exists := g.GetOwn(name); exists {
				_ = h.Set(idx, v)
				vm.mirrorGlobalVar(idx, name, deletable)
				return
			}
		}
		_ = h.Set(idx, Undefined)
		_ = h.SetConfigurable(idx, deletable)
		_ = h.SetWritable(idx, true)
		vm.mirrorGlobalVar(idx, name, deletable)
		return
	}
	// A function replaces the binding's value; an existing non-configurable
	// property also keeps its attributes, anything else is redefined.
	redefine := true
	if g != nil {
		if i := g.shape.lookupStringField(name); i >= 0 && !g.shape.fields[i].configurable {
			redefine = false
		}
	}
	if redefine {
		_ = h.SetConfigurable(idx, deletable)
		_ = h.SetWritable(idx, true)
		if g != nil {
			w, e, c := true, true, deletable
			g.DefineOwnProperty(name, value, &w, &e, &c)
		}
	}
	_ = h.Set(idx, value)
	vm.mirrorGlobalVar(idx, name, deletable)
}

// watchGlobal registers the global object property name to mirror slot
// index, once.
func (h *Heap) watchGlobal(index int, g *PlainObject, name string) {
	if index >= 0 && index < len(h.watch) {
		for _, b := range h.watch[index] {
			if b.ns == g && b.name == name {
				return
			}
		}
	}
	h.watchSlot(index, namespaceBinding{ns: g, name: name})
}

// unwatchGlobal removes slot index's mirror on g, returning its name.
func (h *Heap) unwatchGlobal(index int, g *PlainObject) string {
	if index < 0 || index >= len(h.watch) {
		return ""
	}
	bs := h.watch[index]
	for i, b := range bs {
		if b.ns == g {
			h.watch[index] = append(bs[:i:i], bs[i+1:]...)
			return b.name
		}
	}
	return ""
}

// UnmirrorGlobalVar is unmirrorGlobalVar for builtins deleting a global
// object property backed by heap slot idx.
func (vm *VM) UnmirrorGlobalVar(idx int) { vm.unmirrorGlobalVar(idx) }

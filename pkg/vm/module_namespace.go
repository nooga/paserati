package vm

// Live module bindings (paserati#527).
//
// A module namespace's export properties, and a named import, must read the
// exporting module's binding as it is *now* (ECMAScript 10.4.6.8: [[Get]]
// goes through GetBindingValue), not a copy taken when the namespace was
// built or the exports were first collected. `export let count = 0;
// export function inc() { count++ }` is the common shape: counters, lazily
// initialised singletons, bundler state.
//
// An exported local lives in a heap slot (ModuleRecord.GetExportIndices).
// Named imports (OpGetModuleExport) read that slot directly. Namespace
// objects keep ordinary data properties - so descriptors, key listing,
// inline caches and every other read path stay correct - and are kept in
// sync by the heap: each slot backing an export carries a list of the
// namespace properties mirroring it, updated on every Heap.Set.

// namespaceBinding is one namespace property that mirrors a heap slot.
type namespaceBinding struct {
	ns   *PlainObject
	name string
}

func (b namespaceBinding) update(v Value) {
	// A binding still in its TDZ has no value yet; don't leak the sentinel.
	if v.typ == TypeUninitialized {
		v = Undefined
	}
	b.ns.SetOwn(b.name, v)
}

// watchSlot registers b to mirror heap slot index.
func (h *Heap) watchSlot(index int, b namespaceBinding) {
	if index < 0 {
		return
	}
	if index >= len(h.watch) {
		grown := make([][]namespaceBinding, index+1)
		copy(grown, h.watch)
		h.watch = grown
	}
	h.watch[index] = append(h.watch[index], b)
}

// moduleRecordOf returns the loader record for a module context, loading it
// if the context was registered without one.
func (vm *VM) moduleRecordOf(modulePath string, moduleCtx *ModuleContext) ModuleRecord {
	if moduleCtx.record != nil {
		return moduleCtx.record
	}
	if vm.moduleLoader == nil {
		return nil
	}
	from := vm.moduleFromPath()
	if moduleCtx.resolvedPath != "" {
		from = moduleCtx.resolvedPath
	}
	rec, err := vm.moduleLoader.LoadModule(modulePath, from)
	if err != nil {
		return nil
	}
	return rec
}

// exportSlot resolves exportName of the module at modulePath to the heap
// slot holding its live binding, following `export { x as y } from` and
// `export * from` re-exports (which the compiler records as ReExports) into
// their source module. ok is false for exports with no slot (values the
// record only knows by value, host modules).
func (vm *VM) exportSlot(modulePath string, moduleCtx *ModuleContext, exportName string, depth int) (int, bool) {
	if moduleCtx == nil || depth > 32 {
		return 0, false
	}
	rec := vm.moduleRecordOf(modulePath, moduleCtx)
	if rec == nil {
		return 0, false
	}
	// A re-export first: the live binding is the source module's, even when
	// this module also installed a local copy under an export index (a bare
	// `export * from` does, and so can a named re-export's import).
	re, ok := rec.GetReExports()[exportName]
	if !ok {
		if idx, ok := rec.GetExportIndices()[exportName]; ok {
			return int(idx), true
		}
		return 0, false
	}
	prevFrom := vm.currentModulePath
	if moduleCtx.resolvedPath != "" {
		vm.currentModulePath = moduleCtx.resolvedPath
	}
	defer func() { vm.currentModulePath = prevFrom }()
	key, _ := vm.moduleContextKey(re.SourceModule)
	srcCtx, exists := vm.moduleContexts[key]
	if !exists {
		srcCtx, exists = vm.findModuleContextByResolved(re.SourceModule)
	}
	if !exists {
		return 0, false
	}
	return vm.exportSlot(re.SourceModule, srcCtx, re.SourceName, depth+1)
}

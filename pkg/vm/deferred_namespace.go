package vm

import (
	"sort"
)

// Deferred module namespaces (`import defer * as ns`, proposal-defer-import-eval).
//
// A deferred namespace is a separate object from the module's ordinary
// namespace. The module is not evaluated when it is imported; the first
// operation on the namespace that needs its exports evaluates it:
// [[GetOwnProperty]], [[DefineOwnProperty]], [[HasProperty]], [[Get]] and
// [[Delete]] with a string key other than "then", and [[OwnPropertyKeys]].
// Symbol keys, "then", [[Set]] and the prototype/extensibility operations
// do not.
//
// It is built as a Proxy over a frozen-shape stand-in target (null
// prototype, non-extensible, one non-configurable data property per export
// plus @@toStringTag "Deferred Module"), so the Proxy invariants hold while
// the traps forward to the real namespace once the module has run.
// "then" is left off the stand-in, since its [[GetOwnProperty]] must answer
// without evaluating (and so without the export).

// createDeferredNamespace returns the deferred namespace for modulePath,
// the same object for every `import defer` of that module.
func (vm *VM) createDeferredNamespace(modulePath string) Value {
	contextKey, record := vm.moduleContextKey(modulePath)
	if ns, ok := vm.deferredNamespaces[contextKey]; ok {
		return ns
	}
	// Relative specifiers inside the module resolve against the module
	// that imported it, whenever the evaluation is triggered.
	importerPath := vm.currentModulePath

	names := map[string]bool{}
	if record != nil {
		for name := range record.GetExportIndices() {
			names[name] = true
		}
		for name := range record.GetReExports() {
			names[name] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		if name != "then" {
			sorted = append(sorted, name)
		}
	}
	sort.Strings(sorted)

	target := NewObject(Null).AsPlainObject()
	yes, no := true, false
	for _, name := range sorted {
		target.DefineOwnProperty(name, Undefined, &yes, &yes, &no)
	}
	target.DefineOwnPropertyByKey(NewSymbolKey(vm.SymbolToStringTag), NewString("Deferred Module"), &no, &no, &no)
	target.SetExtensible(false)
	targetVal := NewValueFromPlainObject(target)

	var evaluated Value = Undefined
	ensureEvaluated := func() (Value, error) {
		if evaluated != Undefined {
			return evaluated, nil
		}
		if ctx, ok := vm.moduleContexts[contextKey]; ok && ctx.executing {
			return Undefined, vm.NewTypeError("Cannot access a deferred module namespace while its module is evaluating")
		}
		saved := vm.currentModulePath
		vm.currentModulePath = importerPath
		status, thrown := vm.executeModule(modulePath)
		vm.currentModulePath = saved
		if status != InterpretOK {
			if thrown != Undefined && thrown != Null {
				if n := len(vm.errors); n > 0 {
					vm.errors = vm.errors[:n-1]
				}
				return Undefined, exceptionError{exception: thrown}
			}
			msg := "Failed to evaluate module '" + modulePath + "'"
			if n := len(vm.errors); n > 0 {
				msg = vm.errors[n-1].Error()
				vm.errors = vm.errors[:n-1]
			}
			return Undefined, vm.NewTypeError(msg)
		}
		evaluated = vm.createModuleNamespace(contextKey)
		return evaluated, nil
	}

	reflect := func(name string) Value {
		reflectObj, _ := vm.GetGlobal("Reflect")
		fn, _ := vm.GetProperty(reflectObj, name)
		return fn
	}
	// symbolLike keys are answered by the stand-in without evaluating.
	symbolLike := func(key Value) bool {
		return key.IsSymbol() || (key.IsString() && key.ToString() == "then")
	}
	forwarding := func(trap string) Value {
		return NewNativeFunction(2, true, trap, func(args []Value) (Value, error) {
			args = append([]Value(nil), args...)
			for len(args) < 2 {
				args = append(args, Undefined)
			}
			if trap != "ownKeys" && symbolLike(args[1]) {
				return vm.Call(reflect(trap), Undefined, args)
			}
			ns, err := ensureEvaluated()
			if err != nil {
				return Undefined, err
			}
			args[0] = ns
			if trap == "get" && len(args) > 2 {
				args = args[:2] // a namespace [[Get]] ignores the receiver
			}
			result, err := vm.Call(reflect(trap), Undefined, args)
			if err != nil || trap != "ownKeys" || !result.IsArray() {
				return result, err
			}
			// Keep the keys consistent with the stand-in target.
			keys := result.AsArray()
			out := make([]Value, 0, keys.Length())
			for i := 0; i < keys.Length(); i++ {
				if k := keys.Get(i); !(k.IsString() && k.ToString() == "then") {
					out = append(out, k)
				}
			}
			return NewArrayWithArgs(out), nil
		})
	}

	handler := NewObject(Null).AsPlainObject()
	for _, trap := range []string{"get", "has", "getOwnPropertyDescriptor", "defineProperty", "deleteProperty", "ownKeys"} {
		handler.SetOwn(trap, forwarding(trap))
	}
	// [[Set]] on a namespace always fails, without evaluating.
	handler.SetOwn("set", NewNativeFunction(4, false, "set", func(args []Value) (Value, error) {
		return BooleanValue(false), nil
	}))

	ns := NewProxy(targetVal, NewValueFromPlainObject(handler))
	if vm.deferredNamespaces == nil {
		vm.deferredNamespaces = make(map[string]Value)
	}
	vm.deferredNamespaces[contextKey] = ns
	return ns
}

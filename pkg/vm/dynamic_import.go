package vm

import (
	"fmt"
	"unsafe"

	"github.com/nooga/paserati/pkg/errors"
)

// Import phases carried in OpDynamicImport's flags byte.
const (
	ImportPhaseEvaluation byte = 0 // import(...)
	ImportPhaseSource     byte = 1 // import.source(...)
	ImportPhaseDefer      byte = 2 // import.defer(...)

	// DynamicImportHasOptions marks an import() call with a second argument.
	DynamicImportHasOptions byte = 1 << 2
	dynamicImportPhaseMask  byte = 3
)

// supportedImportAttributes are the import attribute keys this host
// understands (HostGetSupportedImportAttributes).
var supportedImportAttributes = map[string]bool{"type": true}

// dynamicImport implements EvaluateImportCall from the point the specifier
// and options have been evaluated: it returns the promise for the module
// namespace, rejecting it on any abrupt completion. modulePath is the module
// the import() call appears in, which relative specifiers resolve against.
func (vm *VM) dynamicImport(specifierValue, options Value, flags byte, modulePath string) Value {
	baseObj := NewObject(vm.PromisePrototype).AsPlainObject()
	promiseObj := &PromiseObject{
		Object:           baseObj.Object,
		State:            PromisePending,
		Result:           Undefined,
		FulfillReactions: []PromiseReaction{},
		RejectReactions:  []PromiseReaction{},
	}
	promiseVal := Value{typ: TypePromise, obj: unsafe.Pointer(promiseObj)}
	reject := func(reason Value) Value {
		vm.rejectPromise(promiseObj, reason)
		return promiseVal
	}
	rejectWith := func(err error) Value {
		if ee, ok := err.(ExceptionError); ok {
			return reject(ee.GetExceptionValue())
		}
		return reject(NewString(err.Error()))
	}

	// ToString(specifier), rejecting instead of throwing.
	var specifier string
	if specifierValue.IsObject() || specifierValue.IsCallable() {
		var toStringMethod Value
		if ok, _, _ := vm.opGetProp(nil, 0, &specifierValue, "toString", &toStringMethod); ok && toStringMethod.IsCallable() {
			result, exc, threw := vm.callCatchingException(func() (Value, error) {
				return vm.Call(toStringMethod, specifierValue, nil)
			})
			if threw {
				return reject(exc)
			}
			specifier = result.ToString()
		} else {
			specifier = specifierValue.ToString()
		}
	} else {
		specifier = specifierValue.ToString()
	}

	// Import attributes from the options argument.
	if flags&DynamicImportHasOptions != 0 && options != Undefined {
		if !options.IsObject() && !options.IsCallable() {
			return rejectWith(vm.NewTypeError("The second argument of import() must be an object"))
		}
		attributesObj, exc, threw := vm.callCatchingException(func() (Value, error) {
			return vm.GetProperty(options, "with")
		})
		if threw {
			return reject(exc)
		}
		if attributesObj != Undefined {
			if !attributesObj.IsObject() && !attributesObj.IsCallable() {
				return rejectWith(vm.NewTypeError("The 'with' option of import() must be an object"))
			}
			entries, exc, threw := vm.callCatchingException(func() (Value, error) {
				objectCtor, _ := vm.GetGlobal("Object")
				entriesFn, err := vm.GetProperty(objectCtor, "entries")
				if err != nil {
					return Undefined, err
				}
				return vm.Call(entriesFn, objectCtor, []Value{attributesObj})
			})
			if threw {
				return reject(exc)
			}
			if entries.IsArray() {
				list := entries.AsArray()
				var unsupported string
				for i := 0; i < list.Length(); i++ {
					entry := list.Get(i)
					if !entry.IsArray() || entry.AsArray().Length() < 2 {
						continue
					}
					key, value := entry.AsArray().Get(0), entry.AsArray().Get(1)
					if !value.IsString() {
						return rejectWith(vm.NewTypeError(fmt.Sprintf("Import attribute '%s' must be a string", key.ToString())))
					}
					if !supportedImportAttributes[key.ToString()] && unsupported == "" {
						unsupported = key.ToString()
					}
				}
				if unsupported != "" {
					return rejectWith(vm.NewSyntaxError(fmt.Sprintf("Unsupported import attribute '%s'", unsupported)))
				}
			}
		}
	}

	// Load, link and evaluate through the standard module machinery. A
	// relative specifier resolves against the module containing this
	// import() call, which is not necessarily the one running now.
	prevImportFrom := vm.currentModulePath
	if modulePath != "" {
		vm.currentModulePath = modulePath
	}
	var status InterpretResult
	var moduleErrVal Value
	phase := flags & dynamicImportPhaseMask
	if phase == ImportPhaseEvaluation {
		status, moduleErrVal = vm.executeModule(specifier)
	} else {
		// Source and defer phases load and link without evaluating.
		status, moduleErrVal = vm.loadModuleWithoutEvaluating(specifier)
	}
	var ctxExists bool
	var namespaceObj Value
	if status == InterpretOK && phase == ImportPhaseDefer {
		ctxExists = true
		namespaceObj = vm.createDeferredNamespace(specifier)
	} else if status == InterpretOK {
		contextKey, _ := vm.moduleContextKey(specifier)
		_, ctxExists = vm.moduleContexts[contextKey]
		if ctxExists {
			// The module's one cached [[Namespace]] (GetModuleNamespace).
			namespaceObj = vm.createModuleNamespace(contextKey)
		}
	}
	vm.currentModulePath = prevImportFrom

	if status != InterpretOK {
		// Reject with the module's own thrown value when there is one; per
		// spec import() rejects with the abrupt completion's value.
		if moduleErrVal != Null && moduleErrVal != Undefined {
			if len(vm.errors) > 0 {
				vm.errors = vm.errors[:len(vm.errors)-1]
			}
			return reject(moduleErrVal)
		}
		errorMsg := fmt.Sprintf("Failed to load module '%s'", specifier)
		syntaxFailure := false
		if len(vm.errors) > 0 {
			lastErr := vm.errors[len(vm.errors)-1]
			errorMsg = lastErr.Error()
			if re, ok := lastErr.(*errors.RuntimeError); ok && re.Resolution {
				syntaxFailure = true
			}
			vm.errors = vm.errors[:len(vm.errors)-1]
		}
		if syntaxFailure {
			return rejectWith(vm.NewSyntaxError(errorMsg))
		}
		errObj := NewObject(vm.ErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("Error"))
		errObj.SetOwn("message", NewString(errorMsg))
		return reject(NewValueFromPlainObject(errObj))
	}
	if phase == ImportPhaseSource {
		// GetModuleSource of a Source Text Module Record is always abrupt.
		return rejectWith(vm.NewSyntaxError(fmt.Sprintf("Source phase import is not available for module '%s'", specifier)))
	}
	if !ctxExists {
		errObj := NewObject(vm.ErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("Error"))
		errObj.SetOwn("message", NewString(fmt.Sprintf("Module '%s' was loaded but context is missing", specifier)))
		return reject(NewValueFromPlainObject(errObj))
	}
	vm.resolvePromise(promiseObj, namespaceObj)
	return promiseVal
}

// loadModuleWithoutEvaluating loads and links the module a source or defer
// phase import names, reporting failures like executeModule does.
func (vm *VM) loadModuleWithoutEvaluating(specifier string) (InterpretResult, Value) {
	if vm.moduleLoader == nil {
		return vm.runtimeError("No module loader available"), Undefined
	}
	record, err := vm.moduleLoader.LoadModule(specifier, vm.moduleFromPath())
	if err != nil {
		return vm.runtimeErrorFrom(err, "Failed to load module '%s': %s", specifier, err.Error()), Undefined
	}
	if record == nil {
		return vm.runtimeError("Failed to load module '%s'", specifier), Undefined
	}
	if moduleErr := record.GetError(); moduleErr != nil {
		status := vm.runtimeErrorFrom(moduleErr, "Module '%s' failed to load: %s", specifier, moduleErr.Error())
		if isModuleSyntaxFailure(moduleErr) {
			if n := len(vm.errors); n > 0 {
				if re, ok := vm.errors[n-1].(*errors.RuntimeError); ok {
					re.Resolution = true
				}
			}
		}
		return status, Undefined
	}
	return InterpretOK, Undefined
}

// callCatchingException runs op, which may call into JavaScript, and turns a
// thrown exception into a returned value, leaving the VM as it was before
// the call (no unwinding in progress).
func (vm *VM) callCatchingException(op func() (Value, error)) (Value, Value, bool) {
	savedFrameCount := vm.frameCount
	savedRegMark := vm.regDir.mark()
	savedUnwinding := vm.unwinding
	savedCurrentException := vm.currentException
	savedHasException := vm.hasException

	result, err := op()
	if err == nil && !vm.unwinding {
		return result, Undefined, false
	}
	var exceptionVal Value
	if vm.hasException {
		exceptionVal = vm.currentException
	} else if ee, ok := err.(ExceptionError); ok {
		exceptionVal = ee.GetExceptionValue()
	} else if err != nil {
		exceptionVal = NewString(err.Error())
	}
	vm.frameCount = savedFrameCount
	vm.regDir.popTo(savedRegMark)
	vm.unwinding = savedUnwinding
	vm.currentException = savedCurrentException
	vm.hasException = savedHasException
	return Undefined, exceptionVal, true
}

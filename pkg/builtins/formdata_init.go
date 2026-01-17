package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// Priority constant for FormData
const PriorityFormData = 191 // After Blob, before fetch

type FormDataInitializer struct{}

func (f *FormDataInitializer) Name() string {
	return "FormData"
}

func (f *FormDataInitializer) Priority() int {
	return PriorityFormData
}

func (f *FormDataInitializer) InitTypes(ctx *TypeContext) error {
	// FormData type
	formDataType := types.NewObjectType().
		WithProperty("append", types.NewSimpleFunction([]types.Type{types.String, types.Any}, types.Undefined)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{types.String}, types.Undefined)).
		WithProperty("get", types.NewSimpleFunction([]types.Type{types.String}, types.Any)).
		WithProperty("getAll", types.NewSimpleFunction([]types.Type{types.String}, types.Any)).
		WithProperty("has", types.NewSimpleFunction([]types.Type{types.String}, types.Boolean)).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.String, types.Any}, types.Undefined)).
		WithProperty("entries", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("keys", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("values", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{types.Any}, types.Undefined))

	// FormData constructor type
	formDataConstructorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, formDataType). // FormData()
		WithProperty("prototype", formDataType)

	return ctx.DefineGlobal("FormData", formDataConstructorType)
}

func (f *FormDataInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create FormData.prototype
	formDataProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Create FormData constructor
	formDataConstructorFn := func(args []vm.Value) (vm.Value, error) {
		fd := &FormData{
			entries: make([]FormDataEntry, 0),
		}
		return createFormDataObject(vmInstance, fd, formDataProto), nil
	}

	formDataConstructor := vm.NewConstructorWithProps(0, false, "FormData", formDataConstructorFn)
	if ctorProps := formDataConstructor.AsNativeFunctionWithProps(); ctorProps != nil {
		ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(formDataProto))
	}

	formDataProto.SetOwnNonEnumerable("constructor", formDataConstructor)

	return ctx.DefineGlobal("FormData", formDataConstructor)
}

// FormDataEntry represents a single entry in FormData
type FormDataEntry struct {
	name     string
	value    vm.Value
	filename string // For file uploads
}

// FormData represents form data that can be sent with fetch
type FormData struct {
	entries []FormDataEntry
}

func createFormDataObject(vmInstance *vm.VM, fd *FormData, _ *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// append(name, value) or append(name, blob, filename)
	obj.SetOwnNonEnumerable("append", vm.NewNativeFunction(3, false, "append", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, nil
		}
		name := args[0].ToString()
		value := args[1]
		filename := ""
		if len(args) > 2 && args[2].Type() == vm.TypeString {
			filename = args[2].ToString()
		}
		fd.entries = append(fd.entries, FormDataEntry{
			name:     name,
			value:    value,
			filename: filename,
		})
		return vm.Undefined, nil
	}))

	// delete(name)
	obj.SetOwnNonEnumerable("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, nil
		}
		name := args[0].ToString()
		newEntries := make([]FormDataEntry, 0, len(fd.entries))
		for _, entry := range fd.entries {
			if entry.name != name {
				newEntries = append(newEntries, entry)
			}
		}
		fd.entries = newEntries
		return vm.Undefined, nil
	}))

	// get(name) -> value or null
	obj.SetOwnNonEnumerable("get", vm.NewNativeFunction(1, false, "get", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Null, nil
		}
		name := args[0].ToString()
		for _, entry := range fd.entries {
			if entry.name == name {
				return entry.value, nil
			}
		}
		return vm.Null, nil
	}))

	// getAll(name) -> array of values
	obj.SetOwnNonEnumerable("getAll", vm.NewNativeFunction(1, false, "getAll", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.NewArray(), nil
		}
		name := args[0].ToString()
		result := vm.NewArray()
		arr := result.AsArray()
		for _, entry := range fd.entries {
			if entry.name == name {
				arr.Append(entry.value)
			}
		}
		return result, nil
	}))

	// has(name) -> boolean
	obj.SetOwnNonEnumerable("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.False, nil
		}
		name := args[0].ToString()
		for _, entry := range fd.entries {
			if entry.name == name {
				return vm.True, nil
			}
		}
		return vm.False, nil
	}))

	// set(name, value) or set(name, blob, filename)
	obj.SetOwnNonEnumerable("set", vm.NewNativeFunction(3, false, "set", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, nil
		}
		name := args[0].ToString()
		value := args[1]
		filename := ""
		if len(args) > 2 && args[2].Type() == vm.TypeString {
			filename = args[2].ToString()
		}

		// Remove all existing entries with this name
		newEntries := make([]FormDataEntry, 0, len(fd.entries))
		for _, entry := range fd.entries {
			if entry.name != name {
				newEntries = append(newEntries, entry)
			}
		}
		// Add the new entry
		newEntries = append(newEntries, FormDataEntry{
			name:     name,
			value:    value,
			filename: filename,
		})
		fd.entries = newEntries
		return vm.Undefined, nil
	}))

	// entries() -> array of [name, value] pairs
	obj.SetOwnNonEnumerable("entries", vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		result := vm.NewArray()
		arr := result.AsArray()
		for _, entry := range fd.entries {
			pair := vm.NewArray()
			pairArr := pair.AsArray()
			pairArr.Append(vm.NewString(entry.name))
			pairArr.Append(entry.value)
			arr.Append(pair)
		}
		return result, nil
	}))

	// keys() -> array of names
	obj.SetOwnNonEnumerable("keys", vm.NewNativeFunction(0, false, "keys", func(args []vm.Value) (vm.Value, error) {
		result := vm.NewArray()
		arr := result.AsArray()
		for _, entry := range fd.entries {
			arr.Append(vm.NewString(entry.name))
		}
		return result, nil
	}))

	// values() -> array of values
	obj.SetOwnNonEnumerable("values", vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		result := vm.NewArray()
		arr := result.AsArray()
		for _, entry := range fd.entries {
			arr.Append(entry.value)
		}
		return result, nil
	}))

	// forEach(callback)
	obj.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 || !args[0].IsCallable() {
			return vm.Undefined, nil
		}
		callback := args[0]
		for _, entry := range fd.entries {
			_, _ = vmInstance.Call(callback, vm.Undefined, []vm.Value{
				entry.value,
				vm.NewString(entry.name),
				vm.NewValueFromPlainObject(obj),
			})
		}
		return vm.Undefined, nil
	}))

	return vm.NewValueFromPlainObject(obj)
}

// GetEntries returns the form data entries for serialization
func (fd *FormData) GetEntries() []FormDataEntry {
	return fd.entries
}

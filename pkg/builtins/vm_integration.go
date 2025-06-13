package builtins

import (
	"paserati/pkg/vm"
)

// GetStandardInitCallbacks returns the standard set of VM initialization callbacks
// This allows VM constructor to automatically register all standard builtins
func GetStandardInitCallbacks() []vm.VMInitCallback {
	return []vm.VMInitCallback{
		initializeBuiltinsCallback,
	}
}

// initializeBuiltinsCallback sets up all built-in functions and prototypes for a VM
func initializeBuiltinsCallback(vmInstance *vm.VM) error {
	// First, initialize the old registry system
	InitializeRegistry()
	
	// Set up Object prototype methods  
	setupObjectPrototype(vmInstance)
	
	// Set up Object static methods
	setupObjectStaticMethods(vmInstance)
	
	// Set up Function prototype methods with VM isolation
	setupFunctionPrototype(vmInstance)
	
	// Set up other prototypes as needed
	setupArrayPrototype(vmInstance)
	// setupStringPrototype(vmInstance)
	
	return nil
}


// setupObjectPrototype adds Object.prototype methods to the VM
func setupObjectPrototype(vmInstance *vm.VM) {
	objProto := vmInstance.ObjectPrototype.AsPlainObject()
	
	// Object.prototype.hasOwnProperty
	hasOwnPropertyImpl := func(args []vm.Value) vm.Value {
		// 'this' should be the first argument for prototype methods
		if len(args) < 2 {
			return vm.BooleanValue(false)
		}
		
		thisObj := args[0]
		propName := args[1].ToString()
		
		switch thisObj.Type() {
		case vm.TypeObject:
			obj := thisObj.AsPlainObject()
			_, exists := obj.GetOwn(propName)
			return vm.BooleanValue(exists)
		case vm.TypeDictObject:
			dict := thisObj.AsDictObject()
			_, exists := dict.GetOwn(propName)
			return vm.BooleanValue(exists)
		default:
			return vm.BooleanValue(false)
		}
	}
	
	objProto.SetOwn("hasOwnProperty", vm.NewNativeFunction(1, false, "hasOwnProperty", hasOwnPropertyImpl))
	
	// Object.prototype.isPrototypeOf
	isPrototypeOfImpl := func(args []vm.Value) vm.Value {
		if len(args) < 2 {
			return vm.BooleanValue(false)
		}
		
		thisObj := args[0]   // The potential prototype
		targetObj := args[1] // The object to check
		
		if !targetObj.IsObject() {
			return vm.BooleanValue(false)
		}
		
		// Walk up the prototype chain of targetObj
		var current vm.Value
		if targetObj.Type() == vm.TypeObject {
			current = targetObj.AsPlainObject().GetPrototype()
		} else if targetObj.Type() == vm.TypeDictObject {
			current = targetObj.AsDictObject().GetPrototype()
		} else {
			return vm.BooleanValue(false)
		}
		
		for current.Type() != vm.TypeNull && current.Type() != vm.TypeUndefined {
			if current.Equals(thisObj) {
				return vm.BooleanValue(true)
			}
			if current.Type() == vm.TypeObject {
				current = current.AsPlainObject().GetPrototype()
			} else if current.Type() == vm.TypeDictObject {
				current = current.AsDictObject().GetPrototype()
			} else {
				break
			}
		}
		
		return vm.BooleanValue(false)
	}
	
	objProto.SetOwn("isPrototypeOf", vm.NewNativeFunction(1, false, "isPrototypeOf", isPrototypeOfImpl))
}

// setupObjectStaticMethods adds Object static methods
func setupObjectStaticMethods(vmInstance *vm.VM) {
	// For now, let's skip Object static methods setup 
	// This is a temporary approach until we fully integrate with the existing builtins system
	// TODO: Properly integrate with existing Object constructor from builtins.go
}

// Note: Removed global init() function registration
// VM constructor now uses GetStandardInitCallbacks() to get all standard callbacks
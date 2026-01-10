package builtins

import (
	"testing"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

func TestObjectInitializer(t *testing.T) {
	// Test that ObjectInitializer implements the interface correctly
	var initializer BuiltinInitializer = &ObjectInitializer{}

	if initializer.Name() != "Object" {
		t.Errorf("Expected name 'Object', got %s", initializer.Name())
	}

	if initializer.Priority() != PriorityObject {
		t.Errorf("Expected priority %d, got %d", PriorityObject, initializer.Priority())
	}
}

func TestObjectInitTypes(t *testing.T) {
	obj := &ObjectInitializer{}

	// Track what gets defined
	definedGlobals := make(map[string]types.Type)
	definedPrototypes := make(map[string]*types.ObjectType)

	ctx := &TypeContext{
		DefineGlobal: func(name string, typ types.Type) error {
			definedGlobals[name] = typ
			return nil
		},
		GetType: func(name string) (types.Type, bool) {
			typ, exists := definedGlobals[name]
			return typ, exists
		},
		SetPrimitivePrototype: func(primitiveName string, prototypeType *types.ObjectType) {
			definedPrototypes[primitiveName] = prototypeType
		},
	}

	err := obj.InitTypes(ctx)
	if err != nil {
		t.Fatalf("InitTypes failed: %v", err)
	}

	// Check that Object constructor was defined
	objectType, exists := definedGlobals["Object"]
	if !exists {
		t.Fatal("Object constructor not defined globally")
	}

	// Check that it's an ObjectType with call signatures
	objectObjType, ok := objectType.(*types.ObjectType)
	if !ok {
		t.Fatal("Object type is not an ObjectType")
	}

	if !objectObjType.IsCallable() {
		t.Error("Object constructor should be callable")
	}

	// Check that Object.prototype was stored for primitives
	_, exists = definedPrototypes["object"]
	if !exists {
		t.Error("Object prototype not defined for primitive 'object'")
	}
}

func TestObjectInitRuntime(t *testing.T) {
	obj := &ObjectInitializer{}

	// Create a VM instance
	vmInstance := vm.NewVM()

	// Track what gets defined globally
	definedGlobals := make(map[string]vm.Value)

	ctx := &RuntimeContext{
		VM: vmInstance,
		DefineGlobal: func(name string, value vm.Value) error {
			definedGlobals[name] = value
			return nil
		},
		ObjectPrototype:   vm.Undefined,
		FunctionPrototype: vm.Undefined,
		ArrayPrototype:    vm.Undefined,
	}

	err := obj.InitRuntime(ctx)
	if err != nil {
		t.Fatalf("InitRuntime failed: %v", err)
	}

	// Check that Object constructor was defined globally
	objectValue, exists := definedGlobals["Object"]
	if !exists {
		t.Fatal("Object constructor not defined globally in runtime")
	}

	// Check that it's a callable function
	if !objectValue.IsCallable() {
		t.Error("Object constructor should be callable at runtime")
	}

	// Check that VM.ObjectPrototype was set
	if vmInstance.ObjectPrototype.Type() == vm.TypeUndefined {
		t.Error("VM.ObjectPrototype was not set")
	}
}

func TestFunctionInitializer(t *testing.T) {
	// Test that FunctionInitializer implements the interface correctly
	var initializer BuiltinInitializer = &FunctionInitializer{}

	if initializer.Name() != "Function" {
		t.Errorf("Expected name 'Function', got %s", initializer.Name())
	}

	if initializer.Priority() != PriorityFunction {
		t.Errorf("Expected priority %d, got %d", PriorityFunction, initializer.Priority())
	}
}

func TestBothObjectAndFunctionInitializers(t *testing.T) {
	// Test that both initializers work together
	vmInstance := vm.NewVM()

	// Track what gets defined globally
	definedGlobals := make(map[string]vm.Value)
	definedPrototypes := make(map[string]*types.ObjectType)

	// Initialize Object first
	objInit := &ObjectInitializer{}

	typeCtx := &TypeContext{
		DefineGlobal: func(name string, typ types.Type) error {
			return nil // We're focusing on runtime here
		},
		GetType: func(name string) (types.Type, bool) {
			return nil, false
		},
		SetPrimitivePrototype: func(primitiveName string, prototypeType *types.ObjectType) {
			definedPrototypes[primitiveName] = prototypeType
		},
	}

	runtimeCtx := &RuntimeContext{
		VM: vmInstance,
		DefineGlobal: func(name string, value vm.Value) error {
			definedGlobals[name] = value
			return nil
		},
		ObjectPrototype:   vm.Undefined,
		FunctionPrototype: vm.Undefined,
		ArrayPrototype:    vm.Undefined,
	}

	// Initialize Object
	err := objInit.InitTypes(typeCtx)
	if err != nil {
		t.Fatalf("Object InitTypes failed: %v", err)
	}

	err = objInit.InitRuntime(runtimeCtx)
	if err != nil {
		t.Fatalf("Object InitRuntime failed: %v", err)
	}

	// Update context with Object prototype
	runtimeCtx.ObjectPrototype = vmInstance.ObjectPrototype

	// Initialize Function
	funcInit := &FunctionInitializer{}

	err = funcInit.InitTypes(typeCtx)
	if err != nil {
		t.Fatalf("Function InitTypes failed: %v", err)
	}

	err = funcInit.InitRuntime(runtimeCtx)
	if err != nil {
		t.Fatalf("Function InitRuntime failed: %v", err)
	}

	// Check that both Object and Function were defined
	if _, exists := definedGlobals["Object"]; !exists {
		t.Error("Object constructor not defined")
	}

	if _, exists := definedGlobals["Function"]; !exists {
		t.Error("Function constructor not defined")
	}

	// Check that VM prototypes were set
	if vmInstance.ObjectPrototype.Type() == vm.TypeUndefined {
		t.Error("VM.ObjectPrototype was not set")
	}

	if vmInstance.FunctionPrototype.Type() == vm.TypeUndefined {
		t.Error("VM.FunctionPrototype was not set")
	}
}

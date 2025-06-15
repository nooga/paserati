package checker

import (
	"paserati/pkg/types"
	"testing"
)

func TestNewGlobalEnvironmentWithInitializers(t *testing.T) {
	// Create global environment using new system
	env := NewGlobalEnvironment()
	
	// Check that Object constructor was defined
	if objectType, _, found := env.Resolve("Object"); !found {
		t.Fatal("Object constructor not defined in global environment")
	} else if objectType == nil {
		t.Fatal("Object constructor type is nil")
	}
	
	// Check that Function constructor was defined
	if functionType, _, found := env.Resolve("Function"); !found {
		t.Fatal("Function constructor not defined in global environment")
	} else if functionType == nil {
		t.Fatal("Function constructor type is nil")
	}
	
	// Check that primitive prototypes were set up
	if env.primitivePrototypes == nil {
		t.Fatal("Primitive prototypes not initialized")
	}
	
	// Check that object prototype methods are accessible
	toStringType := env.GetPrimitivePrototypeMethodType("object", "toString")
	if toStringType == nil {
		t.Error("Object.prototype.toString method type not found")
	}
	
	hasOwnPropertyType := env.GetPrimitivePrototypeMethodType("object", "hasOwnProperty")
	if hasOwnPropertyType == nil {
		t.Error("Object.prototype.hasOwnProperty method type not found")
	}
	
	// Check that function prototype methods are accessible
	callType := env.GetPrimitivePrototypeMethodType("function", "call")
	if callType == nil {
		t.Error("Function.prototype.call method type not found")
	}
	
	applyType := env.GetPrimitivePrototypeMethodType("function", "apply")
	if applyType == nil {
		t.Error("Function.prototype.apply method type not found")
	}
	
	bindType := env.GetPrimitivePrototypeMethodType("function", "bind")
	if bindType == nil {
		t.Error("Function.prototype.bind method type not found")
	}
}

func TestPrototypeMethodResolver(t *testing.T) {
	// Create global environment to initialize the resolver
	_ = NewGlobalEnvironment()
	
	// Test object property access (we have ObjectInitializer)
	objectType := types.NewObjectType()
	hasOwnType := types.GetPropertyType(objectType, "hasOwnProperty", false)
	if hasOwnType == types.Never {
		t.Error("Object.prototype.hasOwnProperty should be accessible via types package")
	}
	
	// Test that toString is available on objects
	toStringType := types.GetPropertyType(objectType, "toString", false)
	if toStringType == types.Never {
		t.Error("Object.prototype.toString should be accessible via types package")
	}
	
	// Note: String prototype methods are not tested yet since we don't have StringInitializer
	// This is expected and will be addressed when we add more initializers
}
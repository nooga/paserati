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

// --- Type Parameter Tests ---

func TestTypeParameterDefinition(t *testing.T) {
	env := NewEnvironment()
	
	// Create a type parameter
	param := types.NewTypeParameter("T", 0, nil)
	
	// Define it in the environment
	success := env.DefineTypeParameter("T", param)
	if !success {
		t.Error("Should be able to define type parameter T")
	}
	
	// Try to define the same parameter again (should fail)
	success = env.DefineTypeParameter("T", param)
	if success {
		t.Error("Should not be able to redefine type parameter T")
	}
	
	// Define a different parameter (should succeed)
	param2 := types.NewTypeParameter("U", 1, nil)
	success = env.DefineTypeParameter("U", param2)
	if !success {
		t.Error("Should be able to define type parameter U")
	}
}

func TestTypeParameterResolution(t *testing.T) {
	env := NewEnvironment()
	
	// Define some type parameters
	paramT := types.NewTypeParameter("T", 0, nil)
	paramU := types.NewTypeParameter("U", 1, types.String) // constrained to string
	
	env.DefineTypeParameter("T", paramT)
	env.DefineTypeParameter("U", paramU)
	
	// Test resolution
	resolved, found := env.ResolveTypeParameter("T")
	if !found {
		t.Error("Should find type parameter T")
	}
	if resolved != paramT {
		t.Error("Should return the correct TypeParameter instance")
	}
	
	resolved, found = env.ResolveTypeParameter("U")
	if !found {
		t.Error("Should find type parameter U")
	}
	if resolved.Constraint != types.String {
		t.Error("Should preserve type parameter constraint")
	}
	
	// Test non-existent parameter
	_, found = env.ResolveTypeParameter("V")
	if found {
		t.Error("Should not find non-existent type parameter V")
	}
}

func TestTypeParameterScoping(t *testing.T) {
	outerEnv := NewEnvironment()
	
	// Define type parameter in outer scope
	paramT := types.NewTypeParameter("T", 0, nil)
	outerEnv.DefineTypeParameter("T", paramT)
	
	// Create inner scope
	innerEnv := NewEnclosedEnvironment(outerEnv)
	
	// Should be able to resolve T from inner scope
	resolved, found := innerEnv.ResolveTypeParameter("T")
	if !found {
		t.Error("Should find type parameter T from outer scope")
	}
	if resolved != paramT {
		t.Error("Should return the correct TypeParameter from outer scope")
	}
	
	// Define different T in inner scope (shadowing)
	paramTInner := types.NewTypeParameter("T", 0, types.Number)
	innerEnv.DefineTypeParameter("T", paramTInner)
	
	// Should now resolve to inner T
	resolved, found = innerEnv.ResolveTypeParameter("T")
	if !found {
		t.Error("Should find type parameter T from inner scope")
	}
	if resolved != paramTInner {
		t.Error("Should return the inner TypeParameter (shadowing)")
	}
	if resolved.Constraint != types.Number {
		t.Error("Should have the inner parameter's constraint")
	}
	
	// Outer scope should still have original T
	resolved, found = outerEnv.ResolveTypeParameter("T")
	if !found {
		t.Error("Outer scope should still have original T")
	}
	if resolved != paramT {
		t.Error("Outer scope should have original TypeParameter")
	}
}
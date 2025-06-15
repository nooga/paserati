package types

import (
	"testing"
)

func TestTypeParameter(t *testing.T) {
	// Test basic type parameter creation
	param := NewTypeParameter("T", 0, nil)
	if param.Name != "T" {
		t.Errorf("Expected name 'T', got '%s'", param.Name)
	}
	if param.Index != 0 {
		t.Errorf("Expected index 0, got %d", param.Index)
	}
	if param.Constraint != nil {
		t.Errorf("Expected nil constraint, got %v", param.Constraint)
	}
	
	// Test type parameter with constraint
	constrainedParam := NewTypeParameter("U", 1, String)
	if constrainedParam.Constraint != String {
		t.Errorf("Expected String constraint, got %v", constrainedParam.Constraint)
	}
	
	// Test string representation
	if param.String() != "T" {
		t.Errorf("Expected 'T', got '%s'", param.String())
	}
	if constrainedParam.String() != "U extends string" {
		t.Errorf("Expected 'U extends string', got '%s'", constrainedParam.String())
	}
}

func TestTypeParameterType(t *testing.T) {
	param := NewTypeParameter("T", 0, nil)
	paramType := &TypeParameterType{Parameter: param}
	
	// Test string representation
	if paramType.String() != "T" {
		t.Errorf("Expected 'T', got '%s'", paramType.String())
	}
	
	// Test equality
	anotherParamType := &TypeParameterType{Parameter: param}
	if !paramType.Equals(anotherParamType) {
		t.Error("TypeParameterTypes with same parameter should be equal")
	}
	
	differentParam := NewTypeParameter("U", 1, nil)
	differentParamType := &TypeParameterType{Parameter: differentParam}
	if paramType.Equals(differentParamType) {
		t.Error("TypeParameterTypes with different parameters should not be equal")
	}
	
	// Test against different type
	if paramType.Equals(String) {
		t.Error("TypeParameterType should not equal primitive type")
	}
}

func TestGenericType(t *testing.T) {
	// Create a simple generic type: Container<T> = { value: T }
	param := NewTypeParameter("T", 0, nil)
	paramType := &TypeParameterType{Parameter: param}
	
	containerBody := NewObjectType()
	containerBody.WithProperty("value", paramType)
	
	containerGeneric := NewGenericType("Container", []*TypeParameter{param}, containerBody)
	
	// Test string representation
	if containerGeneric.String() != "Container<T>" {
		t.Errorf("Expected 'Container<T>', got '%s'", containerGeneric.String())
	}
	
	// Test equality
	anotherContainer := NewGenericType("Container", []*TypeParameter{param}, containerBody)
	if !containerGeneric.Equals(anotherContainer) {
		t.Error("GenericTypes with same structure should be equal")
	}
}

func TestInstantiatedType(t *testing.T) {
	// Use the built-in Array<T> generic
	arrayString := NewInstantiatedType(ArrayGeneric, []Type{String})
	
	// Test string representation
	if arrayString.String() != "Array<string>" {
		t.Errorf("Expected 'Array<string>', got '%s'", arrayString.String())
	}
	
	// Test equality
	anotherArrayString := NewInstantiatedType(ArrayGeneric, []Type{String})
	if !arrayString.Equals(anotherArrayString) {
		t.Error("InstantiatedTypes with same generic and args should be equal")
	}
	
	arrayNumber := NewInstantiatedType(ArrayGeneric, []Type{Number})
	if arrayString.Equals(arrayNumber) {
		t.Error("InstantiatedTypes with different args should not be equal")
	}
}

func TestSubstitution(t *testing.T) {
	// Test Array<string> substitution
	arrayString := NewInstantiatedType(ArrayGeneric, []Type{String})
	substituted := arrayString.Substitute()
	
	// Should result in ArrayType{ElementType: String}
	if arrayType, ok := substituted.(*ArrayType); ok {
		if !arrayType.ElementType.Equals(String) {
			t.Errorf("Expected ArrayType with String elements, got %s", arrayType.ElementType.String())
		}
	} else {
		t.Errorf("Expected ArrayType, got %T", substituted)
	}
	
	// Test caching - second call should return same result
	substituted2 := arrayString.Substitute()
	if substituted != substituted2 {
		t.Error("Substitution should be cached")
	}
}

func TestComplexSubstitution(t *testing.T) {
	// Create a more complex generic: Pair<T, U> = { first: T, second: U }
	paramT := NewTypeParameter("T", 0, nil)
	paramU := NewTypeParameter("U", 1, nil)
	paramTypeT := &TypeParameterType{Parameter: paramT}
	paramTypeU := &TypeParameterType{Parameter: paramU}
	
	pairBody := NewObjectType()
	pairBody.WithProperty("first", paramTypeT)
	pairBody.WithProperty("second", paramTypeU)
	
	pairGeneric := NewGenericType("Pair", []*TypeParameter{paramT, paramU}, pairBody)
	
	// Instantiate Pair<string, number>
	pairStringNumber := NewInstantiatedType(pairGeneric, []Type{String, Number})
	substituted := pairStringNumber.Substitute()
	
	// Verify the substitution
	if objType, ok := substituted.(*ObjectType); ok {
		firstProp, hasFirst := objType.Properties["first"]
		secondProp, hasSecond := objType.Properties["second"]
		
		if !hasFirst || !hasSecond {
			t.Error("Substituted type should have 'first' and 'second' properties")
		}
		
		if !firstProp.Equals(String) {
			t.Errorf("Expected 'first' to be string, got %s", firstProp.String())
		}
		
		if !secondProp.Equals(Number) {
			t.Errorf("Expected 'second' to be number, got %s", secondProp.String())
		}
	} else {
		t.Errorf("Expected ObjectType, got %T", substituted)
	}
}

func TestNestedGenericSubstitution(t *testing.T) {
	// Test Array<Array<string>>
	// First create the inner Array<string> and substitute it
	innerArrayInst := NewInstantiatedType(ArrayGeneric, []Type{String})
	innerArrayType := innerArrayInst.Substitute()
	
	// Now create Array<ArrayType> where ArrayType is the concrete inner array
	arrayArray := NewInstantiatedType(ArrayGeneric, []Type{innerArrayType})
	substituted := arrayArray.Substitute()
	
	// Should be ArrayType{ElementType: ArrayType{ElementType: String}}
	if outerArray, ok := substituted.(*ArrayType); ok {
		if innerArray, ok := outerArray.ElementType.(*ArrayType); ok {
			if !innerArray.ElementType.Equals(String) {
				t.Errorf("Expected inner element type to be string, got %s", innerArray.ElementType.String())
			}
		} else {
			t.Errorf("Expected inner type to be ArrayType, got %T", outerArray.ElementType)
		}
	} else {
		t.Errorf("Expected outer type to be ArrayType, got %T", substituted)
	}
}

func TestBuiltinGenerics(t *testing.T) {
	// Test that built-in generics are properly initialized
	if ArrayGeneric == nil {
		t.Error("ArrayGeneric should be initialized")
	}
	
	if ArrayGeneric.Name != "Array" {
		t.Errorf("Expected ArrayGeneric name to be 'Array', got '%s'", ArrayGeneric.Name)
	}
	
	if len(ArrayGeneric.TypeParameters) != 1 {
		t.Errorf("Expected ArrayGeneric to have 1 type parameter, got %d", len(ArrayGeneric.TypeParameters))
	}
	
	if ArrayGeneric.TypeParameters[0].Name != "T" {
		t.Errorf("Expected ArrayGeneric parameter to be 'T', got '%s'", ArrayGeneric.TypeParameters[0].Name)
	}
	
	// Test Promise generic
	if PromiseGeneric == nil {
		t.Error("PromiseGeneric should be initialized")
	}
	
	if PromiseGeneric.Name != "Promise" {
		t.Errorf("Expected PromiseGeneric name to be 'Promise', got '%s'", PromiseGeneric.Name)
	}
}
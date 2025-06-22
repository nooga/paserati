package types

import (
	"fmt"
)

// ClassType represents a TypeScript/JavaScript class type
type ClassType struct {
	Name            string      // The class name (e.g., "Animal")
	ConstructorType *ObjectType // Constructor signature as ObjectType with construct signature
	InstanceType    *ObjectType // Shape of instances created by this class
	StaticType      *ObjectType // Static methods and properties (for future use)
	SuperClass      *ClassType  // Parent class for inheritance (for future use)
}

func (ct *ClassType) String() string {
	return fmt.Sprintf("class %s", ct.Name)
}

func (ct *ClassType) typeNode() {}

func (ct *ClassType) Equals(other Type) bool {
	if otherClass, ok := other.(*ClassType); ok {
		// Two class types are equal if they have the same name and equivalent types
		return ct.Name == otherClass.Name &&
			ct.ConstructorType.Equals(otherClass.ConstructorType) &&
			ct.InstanceType.Equals(otherClass.InstanceType)
	}
	return false
}

// NewClassType creates a new class type with the given constructor and instance types
func NewClassType(name string, constructorSig *Signature, instanceType *ObjectType) *ClassType {
	// Create constructor type as an ObjectType with a construct signature
	constructorType := NewConstructorType(constructorSig)
	
	return &ClassType{
		Name:            name,
		ConstructorType: constructorType,
		InstanceType:    instanceType,
		StaticType:      NewObjectType(), // Empty for now, will be populated with static members
		SuperClass:      nil,             // No inheritance for now
	}
}

// NewSimpleClassType creates a class type with a simple constructor signature
func NewSimpleClassType(name string, paramTypes []Type, instanceType *ObjectType) *ClassType {
	// Create a constructor signature that returns the instance type
	constructorSig := &Signature{
		ParameterTypes: paramTypes,
		ReturnType:     instanceType,
		OptionalParams: make([]bool, len(paramTypes)), // All required for now
		IsVariadic:     false,
		RestParameterType: nil,
	}
	
	return NewClassType(name, constructorSig, instanceType)
}

// GetConstructorSignature returns the constructor signature for this class
func (ct *ClassType) GetConstructorSignature() *Signature {
	if ct.ConstructorType != nil && len(ct.ConstructorType.ConstructSignatures) > 0 {
		return ct.ConstructorType.ConstructSignatures[0]
	}
	
	// Return default constructor signature (no parameters, returns instance type)
	return &Signature{
		ParameterTypes:    []Type{},
		ReturnType:        ct.InstanceType,
		OptionalParams:    []bool{},
		IsVariadic:        false,
		RestParameterType: nil,
	}
}

// WithMethod adds a method to the instance type of this class
func (ct *ClassType) WithMethod(name string, methodType Type) *ClassType {
	newInstanceType := ct.InstanceType.WithProperty(name, methodType)
	
	return &ClassType{
		Name:            ct.Name,
		ConstructorType: ct.ConstructorType,
		InstanceType:    newInstanceType,
		StaticType:      ct.StaticType,
		SuperClass:      ct.SuperClass,
	}
}

// WithProperty adds a property to the instance type of this class
func (ct *ClassType) WithProperty(name string, propType Type) *ClassType {
	newInstanceType := ct.InstanceType.WithProperty(name, propType)
	
	return &ClassType{
		Name:            ct.Name,
		ConstructorType: ct.ConstructorType,
		InstanceType:    newInstanceType,
		StaticType:      ct.StaticType,
		SuperClass:      ct.SuperClass,
	}
}

// IsInstanceOf checks if a given type could be an instance of this class
func (ct *ClassType) IsInstanceOf(instanceType Type) bool {
	// For now, we check if the instance type is assignable to our instance type
	// This is a simplified version - a full implementation would use proper assignability checking
	return ct.InstanceType.Equals(instanceType)
}
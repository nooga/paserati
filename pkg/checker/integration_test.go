package checker

import (
	"testing"
)

// Note: Full integration tests with lexer/parser are commented out 
// to focus on testing the core builtin system integration

func TestGetBuiltinType(t *testing.T) {
	// Test that getBuiltinType works with the new system
	checker := NewChecker()
	
	// Test Object constructor type
	objectType := checker.getBuiltinType("Object")
	if objectType == nil {
		t.Error("Object builtin type not found")
	}
	
	// Test Function constructor type
	functionType := checker.getBuiltinType("Function")
	if functionType == nil {
		t.Error("Function builtin type not found")
	}
	
	// Test non-existent builtin
	nonExistent := checker.getBuiltinType("NonExistentBuiltin")
	if nonExistent != nil {
		t.Error("Non-existent builtin should return nil")
	}
}
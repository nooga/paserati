package vm

import (
	"testing"
)

func TestHeap_NewHeap(t *testing.T) {
	heap := NewHeap(10)
	if heap.Size() != 0 {
		t.Errorf("Expected new heap size to be 0, got %d", heap.Size())
	}
	if len(heap.values) != 10 {
		t.Errorf("Expected heap capacity to be 10, got %d", len(heap.values))
	}
}

func TestHeap_SetAndGet(t *testing.T) {
	heap := NewHeap(5)
	
	// Test setting and getting a value
	testValue := NewString("test")
	err := heap.Set(2, testValue)
	if err != nil {
		t.Errorf("Unexpected error setting value: %v", err)
	}
	
	value, exists := heap.Get(2)
	if !exists {
		t.Error("Expected value to exist at index 2")
	}
	if value.Type() != TypeString || value.AsString() != "test" {
		t.Errorf("Expected string 'test', got %v", value.Inspect())
	}
	
	// Check that size was updated
	if heap.Size() != 3 {
		t.Errorf("Expected heap size to be 3, got %d", heap.Size())
	}
}

func TestHeap_AutoResize(t *testing.T) {
	heap := NewHeap(2)
	
	// Set a value beyond initial capacity
	testValue := NumberValue(42)
	err := heap.Set(5, testValue)
	if err != nil {
		t.Errorf("Unexpected error setting value: %v", err)
	}
	
	// Check that heap was resized
	if len(heap.values) <= 5 {
		t.Errorf("Expected heap to be resized to accommodate index 5, capacity is %d", len(heap.values))
	}
	if heap.Size() != 6 {
		t.Errorf("Expected heap size to be 6, got %d", heap.Size())
	}
	
	// Verify the value is retrievable
	value, exists := heap.Get(5)
	if !exists {
		t.Error("Expected value to exist at index 5")
	}
	if value.Type() != TypeFloatNumber || value.AsFloat() != 42 {
		t.Errorf("Expected number 42, got %v", value.Inspect())
	}
}

func TestHeap_GetOutOfBounds(t *testing.T) {
	heap := NewHeap(5)
	heap.Set(2, NewString("test"))
	
	// Test negative index
	_, exists := heap.Get(-1)
	if exists {
		t.Error("Expected negative index to return false")
	}
	
	// Test index beyond size
	_, exists = heap.Get(10)
	if exists {
		t.Error("Expected out-of-bounds index to return false")
	}
}

func TestHeap_SetBuiltinGlobals(t *testing.T) {
	heap := NewHeap(5)
	
	globals := map[string]Value{
		"console": NewString("console"),
		"Object":  NewString("Object"),
		"Array":   NewString("Array"),
	}
	
	indexMap := map[string]int{
		"Array":   0,
		"Object":  1,
		"console": 2,
	}
	
	err := heap.SetBuiltinGlobals(globals, indexMap)
	if err != nil {
		t.Errorf("Unexpected error setting builtin globals: %v", err)
	}
	
	// Verify each global was set at the correct index
	for name, expectedIndex := range indexMap {
		value, exists := heap.Get(expectedIndex)
		if !exists {
			t.Errorf("Expected builtin global '%s' to exist at index %d", name, expectedIndex)
		}
		if value.Type() != TypeString || value.AsString() != name {
			t.Errorf("Expected builtin global '%s' at index %d, got %v", name, expectedIndex, value.Inspect())
		}
	}
	
	// Check that heap size was updated appropriately
	if heap.Size() != 3 {
		t.Errorf("Expected heap size to be 3, got %d", heap.Size())
	}
}

func TestHeap_Resize(t *testing.T) {
	heap := NewHeap(2)
	
	// Set a value first
	heap.Set(1, NewString("test"))
	
	// Resize to larger
	heap.Resize(10)
	if len(heap.values) != 10 {
		t.Errorf("Expected heap capacity to be 10, got %d", len(heap.values))
	}
	if heap.Size() != 10 {
		t.Errorf("Expected heap size to be 10, got %d", heap.Size())
	}
	
	// Verify existing value is preserved
	value, exists := heap.Get(1)
	if !exists || value.Type() != TypeString {
		t.Error("Expected existing value to be preserved after resize")
	}
	
	// Verify new slots are initialized to Undefined
	for i := 2; i < 10; i++ {
		value, exists := heap.Get(i)
		if !exists || value.Type() != TypeUndefined {
			t.Errorf("Expected index %d to be Undefined after resize, got %v", i, value.Type())
		}
	}
}
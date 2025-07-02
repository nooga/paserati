package compiler

import (
	"reflect"
	"testing"
)

func TestHeapAlloc_NewHeapAlloc(t *testing.T) {
	ha := NewHeapAlloc()
	if ha.GetAllocatedSize() != 0 {
		t.Errorf("Expected new HeapAlloc to have size 0, got %d", ha.GetAllocatedSize())
	}
	if len(ha.nameToIndex) != 0 {
		t.Errorf("Expected new HeapAlloc to have empty name map, got %d entries", len(ha.nameToIndex))
	}
}

func TestHeapAlloc_GetOrAssignIndex(t *testing.T) {
	ha := NewHeapAlloc()
	
	// First assignment should get index 0
	index1 := ha.GetOrAssignIndex("global1")
	if index1 != 0 {
		t.Errorf("Expected first index to be 0, got %d", index1)
	}
	
	// Second assignment should get index 1
	index2 := ha.GetOrAssignIndex("global2")
	if index2 != 1 {
		t.Errorf("Expected second index to be 1, got %d", index2)
	}
	
	// Reassigning same name should return same index
	index1Again := ha.GetOrAssignIndex("global1")
	if index1Again != index1 {
		t.Errorf("Expected same name to return same index %d, got %d", index1, index1Again)
	}
	
	// Check that allocated size is correct
	if ha.GetAllocatedSize() != 2 {
		t.Errorf("Expected allocated size to be 2, got %d", ha.GetAllocatedSize())
	}
}

func TestHeapAlloc_GetIndex(t *testing.T) {
	ha := NewHeapAlloc()
	ha.GetOrAssignIndex("test")
	
	// Test existing name
	index, exists := ha.GetIndex("test")
	if !exists {
		t.Error("Expected 'test' to exist in HeapAlloc")
	}
	if index != 0 {
		t.Errorf("Expected 'test' to have index 0, got %d", index)
	}
	
	// Test non-existing name
	_, exists = ha.GetIndex("nonexistent")
	if exists {
		t.Error("Expected 'nonexistent' to not exist in HeapAlloc")
	}
}

func TestHeapAlloc_SetIndex(t *testing.T) {
	ha := NewHeapAlloc()
	
	// Set a specific index
	ha.SetIndex("builtin", 5)
	
	// Check that the index was set correctly
	index, exists := ha.GetIndex("builtin")
	if !exists {
		t.Error("Expected 'builtin' to exist after SetIndex")
	}
	if index != 5 {
		t.Errorf("Expected 'builtin' to have index 5, got %d", index)
	}
	
	// Check that nextIndex was updated
	if ha.GetAllocatedSize() != 6 {
		t.Errorf("Expected allocated size to be 6, got %d", ha.GetAllocatedSize())
	}
	
	// Next assignment should get index 6
	nextIndex := ha.GetOrAssignIndex("next")
	if nextIndex != 6 {
		t.Errorf("Expected next assignment to get index 6, got %d", nextIndex)
	}
}

func TestHeapAlloc_PreallocateBuiltins(t *testing.T) {
	ha := NewHeapAlloc()
	
	builtins := []string{"console", "Array", "Object", "String"}
	ha.PreallocateBuiltins(builtins)
	
	// Check that builtins were allocated in alphabetical order
	expectedOrder := []string{"Array", "Object", "String", "console"}
	for i, name := range expectedOrder {
		index, exists := ha.GetIndex(name)
		if !exists {
			t.Errorf("Expected builtin '%s' to be allocated", name)
		}
		if index != i {
			t.Errorf("Expected builtin '%s' to have index %d, got %d", name, i, index)
		}
	}
	
	// Check that allocated size is correct
	if ha.GetAllocatedSize() != 4 {
		t.Errorf("Expected allocated size to be 4, got %d", ha.GetAllocatedSize())
	}
	
	// Next assignment should get index 4
	nextIndex := ha.GetOrAssignIndex("userGlobal")
	if nextIndex != 4 {
		t.Errorf("Expected next assignment to get index 4, got %d", nextIndex)
	}
}

func TestHeapAlloc_GetAllNames(t *testing.T) {
	ha := NewHeapAlloc()
	ha.GetOrAssignIndex("zebra")
	ha.GetOrAssignIndex("alpha")
	ha.GetOrAssignIndex("beta")
	
	names := ha.GetAllNames()
	expected := []string{"alpha", "beta", "zebra"}
	
	if !reflect.DeepEqual(names, expected) {
		t.Errorf("Expected names %v, got %v", expected, names)
	}
}

func TestHeapAlloc_Clone(t *testing.T) {
	original := NewHeapAlloc()
	original.GetOrAssignIndex("global1")
	original.SetIndex("builtin", 10)
	original.GetOrAssignIndex("global2")
	
	clone := original.Clone()
	
	// Check that clone has same allocations
	if clone.GetAllocatedSize() != original.GetAllocatedSize() {
		t.Errorf("Expected clone size %d, got %d", original.GetAllocatedSize(), clone.GetAllocatedSize())
	}
	
	originalMap := original.GetNameToIndexMap()
	cloneMap := clone.GetNameToIndexMap()
	
	if !reflect.DeepEqual(originalMap, cloneMap) {
		t.Errorf("Expected clone map %v, got %v", originalMap, cloneMap)
	}
	
	// Check that modifications to clone don't affect original
	clone.GetOrAssignIndex("newGlobal")
	
	if original.GetAllocatedSize() == clone.GetAllocatedSize() {
		t.Error("Expected clone and original to have different sizes after clone modification")
	}
	
	_, existsInOriginal := original.GetIndex("newGlobal")
	if existsInOriginal {
		t.Error("Expected new global to not exist in original after adding to clone")
	}
}

func TestHeapAlloc_GetNameToIndexMap(t *testing.T) {
	ha := NewHeapAlloc()
	ha.GetOrAssignIndex("a")
	ha.GetOrAssignIndex("b")
	ha.SetIndex("c", 5)
	
	mapping := ha.GetNameToIndexMap()
	expected := map[string]int{
		"a": 0,
		"b": 1,
		"c": 5,
	}
	
	if !reflect.DeepEqual(mapping, expected) {
		t.Errorf("Expected mapping %v, got %v", expected, mapping)
	}
	
	// Check that returned map is a copy (modifications don't affect original)
	mapping["d"] = 10
	_, exists := ha.GetIndex("d")
	if exists {
		t.Error("Expected modification to returned map to not affect original HeapAlloc")
	}
}
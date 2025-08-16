package vm

import (
	"fmt"
)

// Heap represents a unified global variable storage for the VM.
// This replaces the module-specific global tables with a single shared heap
// that all modules and the main program can access consistently.
type Heap struct {
	values []Value // The actual global values
	size   int     // Current size of the heap
	// optional name -> index map to enable VM.GetGlobal(name)
	nameToIndex map[string]int
}

// NewHeap creates a new heap with the specified initial capacity
func NewHeap(initialCapacity int) *Heap {
	return &Heap{
		values: make([]Value, initialCapacity),
		size:   0,
	}
}

// Resize ensures the heap can accommodate at least the specified size
func (h *Heap) Resize(newSize int) {
	if newSize > len(h.values) {
		// Grow the slice, preserving existing values
		newValues := make([]Value, newSize)
		copy(newValues, h.values)
		// Initialize new slots with Undefined
		for i := len(h.values); i < newSize; i++ {
			newValues[i] = Undefined
		}
		h.values = newValues
	}
	if newSize > h.size {
		h.size = newSize
	}
}

// Get retrieves a value from the heap at the specified index
func (h *Heap) Get(index int) (Value, bool) {
	if index < 0 || index >= h.size {
		return Undefined, false
	}
	return h.values[index], true
}

// Set stores a value in the heap at the specified index
func (h *Heap) Set(index int, value Value) error {
	if index < 0 {
		return fmt.Errorf("heap index cannot be negative: %d", index)
	}

	// Auto-resize if needed
	if index >= len(h.values) {
		h.Resize(index + 1)
	}

	h.values[index] = value
	if index >= h.size {
		h.size = index + 1
	}
	return nil
}

// Size returns the current size of the heap
func (h *Heap) Size() int {
	return h.size
}

// Values returns a copy of all values in the heap (for debugging)
func (h *Heap) Values() []Value {
	result := make([]Value, h.size)
	copy(result, h.values[:h.size])
	return result
}

// SetBuiltinGlobals initializes the heap with builtin global variables
// This replaces the old SetBuiltinGlobals method on VM
func (h *Heap) SetBuiltinGlobals(globals map[string]Value, indexMap map[string]int) error {
	// Find the maximum index to size the heap appropriately
	maxIndex := -1
	for _, index := range indexMap {
		if index > maxIndex {
			maxIndex = index
		}
	}

	if maxIndex >= 0 {
		h.Resize(maxIndex + 1)

		// Set each builtin global at its assigned index
		for name, value := range globals {
			if index, exists := indexMap[name]; exists {
				if err := h.Set(index, value); err != nil {
					return fmt.Errorf("failed to set builtin global '%s' at index %d: %v", name, index, err)
				}
			}
		}
		// Store name->index mapping for lookup by name
		if h.nameToIndex == nil {
			h.nameToIndex = make(map[string]int, len(indexMap))
		}
		for name, idx := range indexMap {
			h.nameToIndex[name] = idx
		}
	}

	return nil
}

// GetNameToIndex returns the current name->index mapping (if available)
func (h *Heap) GetNameToIndex() map[string]int {
	return h.nameToIndex
}

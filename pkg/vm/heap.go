package vm

import (
	"fmt"
)

// Heap represents a unified global variable storage for the VM.
// This replaces the module-specific global tables with a single shared heap
// that all modules and the main program can access consistently.
type Heap struct {
	values       []Value        // The actual global values
	configurable []bool         // Whether each global can be deleted (defaults to true for user vars)
	size         int            // Current size of the heap
	// optional name -> index map to enable VM.GetGlobal(name)
	nameToIndex map[string]int
}

// NewHeap creates a new heap with the specified initial capacity
func NewHeap(initialCapacity int) *Heap {
	return &Heap{
		values:       make([]Value, initialCapacity),
		configurable: make([]bool, initialCapacity),
		size:         0,
	}
}

// Resize ensures the heap can accommodate at least the specified size
func (h *Heap) Resize(newSize int) {
	if newSize > len(h.values) {
		// Grow the values slice, preserving existing values
		newValues := make([]Value, newSize)
		copy(newValues, h.values)
		// Initialize new slots with Undefined
		for i := len(h.values); i < newSize; i++ {
			newValues[i] = Undefined
		}
		h.values = newValues

		// Grow the configurable slice, preserving existing flags
		newConfigurable := make([]bool, newSize)
		copy(newConfigurable, h.configurable)
		// New slots default to true (configurable) for user-defined variables
		for i := len(h.configurable); i < newSize; i++ {
			newConfigurable[i] = true
		}
		h.configurable = newConfigurable
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

// SetConfigurable sets whether a global variable at the specified index can be deleted
func (h *Heap) SetConfigurable(index int, configurable bool) error {
	if index < 0 || index >= h.size {
		return fmt.Errorf("heap index out of bounds: %d", index)
	}
	h.configurable[index] = configurable
	return nil
}

// IsConfigurable returns whether a global variable at the specified index can be deleted
func (h *Heap) IsConfigurable(index int) bool {
	if index < 0 || index >= h.size {
		return false
	}
	return h.configurable[index]
}

// Delete removes a global variable at the specified index if it's configurable
// Returns true if deletion succeeded, false if not configurable or doesn't exist
func (h *Heap) Delete(index int) bool {
	if index < 0 || index >= h.size {
		// Non-existent global: delete returns true per ECMAScript spec
		return true
	}
	if !h.configurable[index] {
		// Cannot delete non-configurable global
		return false
	}
	// Set to undefined (we don't actually remove it from the array to preserve indices)
	h.values[index] = Undefined
	return true
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
	// List of non-configurable built-in globals per ECMAScript spec
	nonConfigurableGlobals := map[string]bool{
		"NaN":       true,
		"Infinity":  true,
		"undefined": true,
	}

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
				// Mark non-configurable globals
				if nonConfigurableGlobals[name] {
					if err := h.SetConfigurable(index, false); err != nil {
						return fmt.Errorf("failed to mark '%s' as non-configurable: %v", name, err)
					}
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

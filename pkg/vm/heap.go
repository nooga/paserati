package vm

import (
	"fmt"
)

// heapEmptySlot marks a global slot that has never been explicitly Set (or was
// deleted). It shares TypeUninitialized with the public TDZ marker but carries a
// distinct payload, so a single read of the value slot distinguishes three
// states without a parallel initialized[] array: a real value (any other type),
// a declared-but-uninitialized let/const in its TDZ (Uninitialized, payload 0),
// and a never-set slot (this marker, payload 1). The empty marker never escapes
// Get - it is reported as (Undefined, false) - so no code outside the heap
// observes its payload.
var heapEmptySlot = Value{typ: TypeUninitialized, payload: 1}

// Heap represents a unified global variable storage for the VM.
// This replaces the module-specific global tables with a single shared heap
// that all modules and the main program can access consistently.
type Heap struct {
	values       []Value        // The actual global values; unset slots hold heapEmptySlot
	configurable []bool         // Whether each global can be deleted (defaults to true for user vars)
	writable     []bool         // Whether each global can be assigned to (defaults to true for user vars)
	size         int            // Current size of the heap
	// optional name -> index map to enable VM.GetGlobal(name)
	nameToIndex map[string]int
	// builtinCount tracks how many globals are builtins (indices 0 to builtinCount-1)
	// Used to preserve builtins during Reset() while clearing user-defined globals
	builtinCount int
}

// fillEmpty sets every slot in s to the empty-slot marker. Called when a values
// backing array is allocated or grown so any slot not yet Set reads as absent.
func fillEmpty(s []Value) {
	for i := range s {
		s[i] = heapEmptySlot
	}
}

// NewHeap creates a new heap with the specified initial capacity
func NewHeap(initialCapacity int) *Heap {
	// Initialize configurable and writable slices to all true (user variables are modifiable by default)
	configurable := make([]bool, initialCapacity)
	writable := make([]bool, initialCapacity)
	for i := range configurable {
		configurable[i] = true
		writable[i] = true
	}
	values := make([]Value, initialCapacity)
	fillEmpty(values)
	return &Heap{
		values:       values,
		configurable: configurable,
		writable:     writable,
		size:         0,
	}
}

// Resize ensures the heap can accommodate at least the specified size
func (h *Heap) Resize(newSize int) {
	if newSize > len(h.values) {
		// Grow the values slice, preserving existing values
		newValues := make([]Value, newSize)
		copy(newValues, h.values)
		// New slots start empty (never Set)
		fillEmpty(newValues[len(h.values):])
		h.values = newValues

		// Grow the configurable slice, preserving existing flags
		newConfigurable := make([]bool, newSize)
		copy(newConfigurable, h.configurable)
		// New slots default to true (configurable) for user-defined variables
		for i := len(h.configurable); i < newSize; i++ {
			newConfigurable[i] = true
		}
		h.configurable = newConfigurable

		// Grow the writable slice, preserving existing flags
		newWritable := make([]bool, newSize)
		copy(newWritable, h.writable)
		// New slots default to true (writable) for user-defined variables
		for i := len(h.writable); i < newSize; i++ {
			newWritable[i] = true
		}
		h.writable = newWritable
	}
	if newSize > h.size {
		h.size = newSize
	}
}

// Get retrieves a value from the heap at the specified index
// Returns (value, true) if the slot exists AND has been initialized
// Returns (Undefined, false) if the slot doesn't exist OR hasn't been initialized
func (h *Heap) Get(index int) (Value, bool) {
	if index < 0 || index >= h.size {
		return Undefined, false
	}
	// One read: an empty slot (never Set / deleted) reports as absent; anything
	// else - including the TDZ marker - is a live value the caller inspects.
	v := h.values[index]
	if v.typ == TypeUninitialized && v.payload == heapEmptySlot.payload {
		return Undefined, false
	}
	return v, true
}

// IsInitialized reports whether the slot has been explicitly Set (to any value,
// including the TDZ marker). Equivalent to the old initialized[] flag.
func (h *Heap) IsInitialized(index int) bool {
	if index < 0 || index >= h.size {
		return false
	}
	v := h.values[index]
	return !(v.typ == TypeUninitialized && v.payload == heapEmptySlot.payload)
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

	h.values[index] = value // any non-empty value marks the slot as set
	if index >= h.size {
		h.size = index + 1
	}
	return nil
}

// Size returns the current size of the heap
func (h *Heap) Size() int {
	return h.size
}

// GetNameByIndex returns the name of a global variable by its heap index
// Returns empty string if the index doesn't have a name mapping
func (h *Heap) GetNameByIndex(index int) string {
	for name, idx := range h.nameToIndex {
		if idx == index {
			return name
		}
	}
	return ""
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

// SetWritable sets whether a global variable at the specified index can be assigned to
func (h *Heap) SetWritable(index int, writable bool) error {
	if index < 0 || index >= h.size {
		return fmt.Errorf("heap index out of bounds: %d", index)
	}
	h.writable[index] = writable
	return nil
}

// IsWritable returns whether a global variable at the specified index can be assigned to
func (h *Heap) IsWritable(index int) bool {
	if index < 0 || index >= h.size {
		return true // Default to writable for non-existent slots
	}
	return h.writable[index]
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
	// Mark the slot empty (we keep the array entry to preserve indices)
	h.values[index] = heapEmptySlot
	return true
}

// Values returns a copy of all values in the heap (for debugging)
func (h *Heap) Values() []Value {
	result := make([]Value, h.size)
	copy(result, h.values[:h.size])
	// Present empty (never-Set) slots as Undefined rather than leaking the
	// internal marker to debug consumers.
	for i, v := range result {
		if v.typ == TypeUninitialized && v.payload == heapEmptySlot.payload {
			result[i] = Undefined
		}
	}
	return result
}

// SetBuiltinGlobals initializes the heap with builtin global variables
// This replaces the old SetBuiltinGlobals method on VM
func (h *Heap) SetBuiltinGlobals(globals map[string]Value, indexMap map[string]int) error {
	// List of non-configurable, non-writable built-in globals per ECMAScript spec
	// NaN, Infinity, undefined are { writable: false, enumerable: false, configurable: false }
	nonWritableGlobals := map[string]bool{
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
				// Set configurable and writable flags based on ECMAScript spec
				if nonWritableGlobals[name] {
					if err := h.SetConfigurable(index, false); err != nil {
						return fmt.Errorf("failed to mark '%s' as non-configurable: %v", name, err)
					}
					if err := h.SetWritable(index, false); err != nil {
						return fmt.Errorf("failed to mark '%s' as non-writable: %v", name, err)
					}
				} else {
					// Most builtins are configurable and writable
					if err := h.SetConfigurable(index, true); err != nil {
						return fmt.Errorf("failed to mark '%s' as configurable: %v", name, err)
					}
					// Default writable is already true from Resize, but be explicit
					if err := h.SetWritable(index, true); err != nil {
						return fmt.Errorf("failed to mark '%s' as writable: %v", name, err)
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

		// Track builtin count - all indices 0 to maxIndex are builtins
		// This allows Reset() to preserve builtins while clearing user-defined globals
		h.builtinCount = maxIndex + 1
	}

	return nil
}

// GetNameToIndex returns the current name->index mapping (if available)
func (h *Heap) GetNameToIndex() map[string]int {
	return h.nameToIndex
}

// UpdateNameToIndex merges new name->index mappings into the heap's mapping
// This is called after compilation to sync user-defined global names from the compiler
func (h *Heap) UpdateNameToIndex(newMappings map[string]int) {
	if h.nameToIndex == nil {
		h.nameToIndex = make(map[string]int, len(newMappings))
	}

	// Find the maximum index and resize the heap if needed
	// This ensures configurable/initialized slices are properly sized
	maxIndex := -1
	for _, idx := range newMappings {
		if idx > maxIndex {
			maxIndex = idx
		}
	}
	if maxIndex >= 0 && maxIndex >= len(h.configurable) {
		h.Resize(maxIndex + 1)
	}

	for name, idx := range newMappings {
		h.nameToIndex[name] = idx
	}
}

// CloneLayout creates a new Heap with the same nameToIndex mapping and size,
// but with all value slots uninitialized. This is used for realm isolation:
// compiled bytecode references globals by index, so the new heap must have the
// same layout. But since values are separate, writes go to the new realm's storage.
func (h *Heap) CloneLayout() *Heap {
	newSize := len(h.values)
	if h.size > newSize {
		newSize = h.size
	}

	configurable := make([]bool, newSize)
	writable := make([]bool, newSize)
	for i := range configurable {
		configurable[i] = true
		writable[i] = true
	}

	// Clone the nameToIndex map
	nameToIndex := make(map[string]int, len(h.nameToIndex))
	for name, idx := range h.nameToIndex {
		nameToIndex[name] = idx
	}

	values := make([]Value, newSize)
	fillEmpty(values)
	return &Heap{
		values:       values,
		configurable: configurable,
		writable:     writable,
		size:         h.size,
		nameToIndex:  nameToIndex,
		builtinCount: h.builtinCount,
	}
}

// ClearUserGlobals resets user-defined globals while preserving builtin globals
// This is used by VM.Reset() to prevent memory leaks without destroying builtins
func (h *Heap) ClearUserGlobals() {
	// Clear all values beyond the builtin range back to empty
	for i := h.builtinCount; i < h.size; i++ {
		h.values[i] = heapEmptySlot
	}
	// Reset size to just the builtins
	h.size = h.builtinCount
}

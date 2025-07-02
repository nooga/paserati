package compiler

import (
	"fmt"
	"sort"
)

// HeapAlloc manages allocation of indices in the unified global heap.
// This replaces the module-specific global index management with a single
// coordinated allocator that all compilers can use consistently.
type HeapAlloc struct {
	nameToIndex map[string]int // Maps global names to their heap indices
	nextIndex   int            // Next available index for allocation
}

// NewHeapAlloc creates a new heap allocator
func NewHeapAlloc() *HeapAlloc {
	return &HeapAlloc{
		nameToIndex: make(map[string]int),
		nextIndex:   0,
	}
}

// GetOrAssignIndex returns the heap index for a global variable name,
// allocating a new index if the name hasn't been seen before
func (ha *HeapAlloc) GetOrAssignIndex(name string) int {
	if index, exists := ha.nameToIndex[name]; exists {
		return index
	}
	
	// Assign new index
	index := ha.nextIndex
	ha.nameToIndex[name] = index
	ha.nextIndex++
	return index
}

// GetIndex returns the heap index for a global variable name if it exists
func (ha *HeapAlloc) GetIndex(name string) (int, bool) {
	index, exists := ha.nameToIndex[name]
	return index, exists
}

// SetIndex explicitly sets the heap index for a global variable name
// This is used when coordinating with pre-allocated builtin indices
func (ha *HeapAlloc) SetIndex(name string, index int) {
	ha.nameToIndex[name] = index
	// Update nextIndex to ensure we don't reuse this index
	if index >= ha.nextIndex {
		ha.nextIndex = index + 1
	}
}

// GetAllNames returns all allocated global names in alphabetical order
func (ha *HeapAlloc) GetAllNames() []string {
	names := make([]string, 0, len(ha.nameToIndex))
	for name := range ha.nameToIndex {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetNameToIndexMap returns a copy of the name-to-index mapping
func (ha *HeapAlloc) GetNameToIndexMap() map[string]int {
	result := make(map[string]int, len(ha.nameToIndex))
	for name, index := range ha.nameToIndex {
		result[name] = index
	}
	return result
}

// GetAllocatedSize returns the number of indices that have been allocated
func (ha *HeapAlloc) GetAllocatedSize() int {
	return ha.nextIndex
}

// PreallocateBuiltins sets up indices for builtin globals in alphabetical order
// This ensures consistent ordering across all compilers
func (ha *HeapAlloc) PreallocateBuiltins(builtinNames []string) {
	// Sort to ensure consistent ordering
	sortedNames := make([]string, len(builtinNames))
	copy(sortedNames, builtinNames)
	sort.Strings(sortedNames)
	
	// Assign indices starting from 0
	for i, name := range sortedNames {
		ha.SetIndex(name, i)
	}
}

// Clone creates a copy of this HeapAlloc for use by module compilers
// This ensures they start with the same builtin allocations
func (ha *HeapAlloc) Clone() *HeapAlloc {
	clone := &HeapAlloc{
		nameToIndex: make(map[string]int, len(ha.nameToIndex)),
		nextIndex:   ha.nextIndex,
	}
	
	// Copy all existing mappings
	for name, index := range ha.nameToIndex {
		clone.nameToIndex[name] = index
	}
	
	return clone
}

// Debug returns a string representation for debugging
func (ha *HeapAlloc) Debug() string {
	names := ha.GetAllNames()
	result := fmt.Sprintf("HeapAlloc(nextIndex=%d):\n", ha.nextIndex)
	for _, name := range names {
		result += fmt.Sprintf("  %s -> %d\n", name, ha.nameToIndex[name])
	}
	return result
}
package compiler

import "fmt"

// Register represents a virtual machine register index.
type Register uint8 // Assuming max 256 registers per function for now

// RegisterAllocator manages the allocation of registers within a function scope.
// This initial implementation uses a simple stack-like allocation.
type RegisterAllocator struct {
	nextReg Register // Index of the next register to allocate
	maxReg  Register // Highest register index allocated so far
	// Could add a free list later for more complex allocation
	freeRegs []Register // Stack of available registers to reuse

	// New fields for tracking logical current/result register
	currentReg Register
	currentSet bool
}

// NewRegisterAllocator creates a new allocator for a scope (e.g., a function).
func NewRegisterAllocator() *RegisterAllocator {
	return &RegisterAllocator{
		nextReg:    0,
		maxReg:     0,
		freeRegs:   make([]Register, 0, 16), // Initialize with some capacity
		currentReg: 0,                       // Initialize
		currentSet: false,                   // Initialize
	}
}

// Alloc allocates the next available register.
func (ra *RegisterAllocator) Alloc() Register {
	var reg Register
	// Check free list first
	if len(ra.freeRegs) > 0 {
		// Pop from free list (stack behavior)
		lastIdx := len(ra.freeRegs) - 1
		reg = ra.freeRegs[lastIdx]
		ra.freeRegs = ra.freeRegs[:lastIdx]
	} else {
		// Allocate new register if free list is empty
		reg = ra.nextReg
		if ra.nextReg < 255 { // Check before incrementing to avoid overflow wrap-around
			ra.nextReg++
		} else {
			// Handle register exhaustion - Panic for now
			panic("Compiler Error: Ran out of registers!")
		}

		if reg > ra.maxReg {
			ra.maxReg = reg
		}
	}

	// Update logical current register on allocation
	ra.currentReg = reg
	ra.currentSet = true

	return reg
}

// Peek returns the index of the next register that *would* be allocated,
// without actually allocating it.
func (ra *RegisterAllocator) Peek() Register {
	return ra.nextReg
}

// Current returns the index of the register holding the most recent logical result.
// Falls back to the highest allocated register if not explicitly set.
func (ra *RegisterAllocator) Current() Register {
	if ra.currentSet {
		return ra.currentReg
	} else if ra.nextReg > 0 {
		// Fallback: return highest allocated if nothing set (matches old behavior)
		return ra.nextReg - 1
	} else {
		// Nothing allocated yet
		return 0 // Or handle as error?
	}
}

// SetCurrent explicitly sets the register considered to hold the current/result value.
func (ra *RegisterAllocator) SetCurrent(reg Register) {
	// Optional: Add check? if reg > ra.maxReg { panic(...) }
	ra.currentReg = reg
	ra.currentSet = true
}

// MaxRegs returns the maximum register index allocated by this allocator + 1
// (representing the number of register slots needed).
func (ra *RegisterAllocator) MaxRegs() Register {
	if ra.nextReg == 0 {
		return 0 // No registers allocated
	}
	return ra.maxReg + 1
}

// Reset prepares the allocator for a new scope (e.g., a new function).
func (ra *RegisterAllocator) Reset() {
	ra.nextReg = 0
	ra.maxReg = 0
	ra.freeRegs = ra.freeRegs[:0] // Clear free list (keeps allocated capacity)
	ra.currentReg = 0
	ra.currentSet = false
}

// Free marks a register as available for reuse.
func (ra *RegisterAllocator) Free(reg Register) {
	// Optional: Could check if reg is already free or out of bounds
	// For simplicity, we assume valid usage for now.
	ra.freeRegs = append(ra.freeRegs, reg)
}

// --- Optional/Future ---

func (r Register) String() string {
	return fmt.Sprintf("R%d", r)
}

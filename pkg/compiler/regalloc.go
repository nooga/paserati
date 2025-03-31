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

	// New fields for tracking logical current/result register
	currentReg Register
	currentSet bool
}

// NewRegisterAllocator creates a new allocator for a scope (e.g., a function).
func NewRegisterAllocator() *RegisterAllocator {
	return &RegisterAllocator{
		nextReg:    0,
		maxReg:     0,
		currentReg: 0,     // Initialize
		currentSet: false, // Initialize
	}
}

// Alloc allocates the next available register.
func (ra *RegisterAllocator) Alloc() Register {
	reg := ra.nextReg
	if ra.nextReg < 255 { // Check before incrementing to avoid overflow wrap-around
		ra.nextReg++
	} else {
		// Handle register exhaustion - Panic for now
		panic("Compiler Error: Ran out of registers!")
	}

	if reg > ra.maxReg {
		ra.maxReg = reg
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
	ra.currentReg = 0
	ra.currentSet = false
}

// --- Optional/Future ---

// Free could be used to mark a register as available again in more complex schemes.
// func (ra *RegisterAllocator) Free(reg Register) {
//     // For stack allocation, maybe decrement nextReg if reg was the last one?
//     if reg == ra.nextReg - 1 {
//         ra.nextReg--
//         // Need to recalculate maxReg if we pop below it
//     }
//     // Or add to a free list
// }

func (r Register) String() string {
	return fmt.Sprintf("R%d", r)
}

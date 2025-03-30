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
}

// NewRegisterAllocator creates a new allocator for a scope (e.g., a function).
func NewRegisterAllocator() *RegisterAllocator {
	return &RegisterAllocator{
		nextReg: 0,
		maxReg:  0,
	}
}

// Alloc allocates the next available register.
func (ra *RegisterAllocator) Alloc() Register {
	reg := ra.nextReg
	if ra.nextReg+1 > 0 { // Check for overflow (uint8)
		ra.nextReg++
	} else {
		// Handle register exhaustion - Panic for now, could return an error
		// or trigger spilling in a more advanced allocator.
		panic("Compiler Error: Ran out of registers!")
	}

	if reg > ra.maxReg {
		ra.maxReg = reg
	}
	return reg
}

// Peek returns the index of the next register that *would* be allocated,
// without actually allocating it.
func (ra *RegisterAllocator) Peek() Register {
	return ra.nextReg
}

// Current returns the index of the most recently allocated register.
// Returns 0 if no registers have been allocated yet (use Peek to check if allocation happened).
// Caution: Use carefully, might be confusing if registers are freed later.
func (ra *RegisterAllocator) Current() Register {
	if ra.nextReg == 0 {
		return 0 // Or maybe return an error/bool?
	}
	return ra.nextReg - 1
}

// MaxRegs returns the maximum register index allocated by this allocator.
// Useful for determining the required register file size for the function frame.
// Returns 0 if no registers were allocated.
func (ra *RegisterAllocator) MaxRegs() Register {
	if ra.nextReg == 0 {
		return 0 // No registers allocated
	}
	// maxReg holds the highest index *used*, so we need maxReg + 1 slots.
	// Example: if only R0 is used, maxReg=0, need 1 slot.
	return ra.maxReg + 1
}

// Reset prepares the allocator for a new scope (e.g., a new function).
func (ra *RegisterAllocator) Reset() {
	ra.nextReg = 0
	ra.maxReg = 0
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

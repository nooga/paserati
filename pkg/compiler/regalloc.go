package compiler

import "fmt"

// Debug flag for register allocation tracing
const debugRegAlloc = true

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

	// Pinning mechanism to prevent important registers from being freed
	pinnedRegs map[Register]bool // Set of pinned registers
}

// NewRegisterAllocator creates a new allocator for a scope (e.g., a function).
func NewRegisterAllocator() *RegisterAllocator {
	return &RegisterAllocator{
		nextReg:    0,
		maxReg:     0,
		freeRegs:   make([]Register, 0, 16), // Initialize with some capacity
		currentReg: 0,                       // Initialize
		currentSet: false,                   // Initialize
		pinnedRegs: make(map[Register]bool), // Initialize pinned registers map
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
		if debugRegAlloc {
			fmt.Printf("[REGALLOC] REUSE R%d (from free list, %d available)\n", reg, len(ra.freeRegs))
		}
	} else {
		// Allocate new register if free list is empty
		reg = ra.nextReg
		if ra.nextReg < 255 { // Check before incrementing to avoid overflow wrap-around
			ra.nextReg++
		} else {
			// Handle register exhaustion - Panic for now
			if debugRegAlloc {
				fmt.Printf("[REGALLOC] EXHAUSTED! Next would be R%d but limit is 255\n", ra.nextReg)
				fmt.Printf("[REGALLOC] Free list has %d registers: %v\n", len(ra.freeRegs), ra.freeRegs)
				fmt.Printf("[REGALLOC] MaxReg so far: R%d\n", ra.maxReg)
			}
			panic("Compiler Error: Ran out of registers!")
		}

		if reg > ra.maxReg {
			ra.maxReg = reg
		}
		if debugRegAlloc {
			fmt.Printf("[REGALLOC] NEW R%d (nextReg now %d, maxReg %d, %d free)\n", reg, ra.nextReg, ra.maxReg, len(ra.freeRegs))
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
	if debugRegAlloc {
		oldReg := ra.currentReg
		oldSet := ra.currentSet
		fmt.Printf("[REGALLOC] SET_CURRENT R%d (was R%d, set=%v)\n", reg, oldReg, oldSet)
	}
	ra.currentReg = reg
	ra.currentSet = true
}

// CurrentSet returns whether a current register has been explicitly set.
func (ra *RegisterAllocator) CurrentSet() bool {
	return ra.currentSet
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
	ra.pinnedRegs = make(map[Register]bool) // Clear pinned registers
}

// Free marks a register as available for reuse, unless it's pinned.
func (ra *RegisterAllocator) Free(reg Register) {
	// Check if register is pinned - if so, don't free it
	if ra.pinnedRegs[reg] {
		if debugRegAlloc {
			fmt.Printf("[REGALLOC] SKIP FREE R%d (pinned, %d pinned total)\n", reg, len(ra.pinnedRegs))
		}
		return
	}

	// Optional: Could check if reg is already free or out of bounds
	// For simplicity, we assume valid usage for now.
	if debugRegAlloc {
		fmt.Printf("[REGALLOC] FREE R%d (free list will have %d registers)\n", reg, len(ra.freeRegs)+1)
	}
	ra.freeRegs = append(ra.freeRegs, reg)
}

// Pin marks a register as pinned, preventing it from being freed.
// This is useful for registers holding local variables that could be captured by upvalues.
func (ra *RegisterAllocator) Pin(reg Register) {
	ra.pinnedRegs[reg] = true
	if debugRegAlloc {
		fmt.Printf("[REGALLOC] PIN R%d (now %d pinned registers)\n", reg, len(ra.pinnedRegs))
	}
}

// Unpin removes the pin from a register, allowing it to be freed again.
func (ra *RegisterAllocator) Unpin(reg Register) {
	delete(ra.pinnedRegs, reg)
	if debugRegAlloc {
		fmt.Printf("[REGALLOC] UNPIN R%d (now %d pinned registers)\n", reg, len(ra.pinnedRegs))
	}
}

// IsPinned checks if a register is currently pinned.
func (ra *RegisterAllocator) IsPinned(reg Register) bool {
	return ra.pinnedRegs[reg]
}

// IsInFreeList checks if a register is already in the free list.
func (ra *RegisterAllocator) IsInFreeList(reg Register) bool {
	for _, freeReg := range ra.freeRegs {
		if freeReg == reg {
			return true
		}
	}
	return false
}

// --- Optional/Future ---

func (r Register) String() string {
	return fmt.Sprintf("R%d", r)
}

package compiler

import "fmt"

// Debug flag for register allocation tracing
const debugRegAlloc = false

// Register represents a virtual machine register index.
type Register uint8 // Assuming max 256 registers per function for now

// NoHint is a sentinel value indicating no register hint is provided
const NoHint Register = 255
const BadRegister Register = 254

// RegisterAllocator manages the allocation of registers within a function scope.
// This initial implementation uses a simple stack-like allocation.
type RegisterAllocator struct {
	nextReg Register // Index of the next register to allocate
	maxReg  Register // Highest register index allocated so far
	// Could add a free list later for more complex allocation
	freeRegs []Register // Stack of available registers to reuse
	// Pinning mechanism to prevent important registers from being freed
	pinnedRegs map[Register]bool // Set of pinned registers
}

// NewRegisterAllocator creates a new allocator for a scope (e.g., a function).
func NewRegisterAllocator() *RegisterAllocator {
	return &RegisterAllocator{
		nextReg:    0,
		maxReg:     0,
		freeRegs:   make([]Register, 0, 16), // Initialize with some capacity
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

	return reg
}

// AllocHinted allocates a register, preferring the hinted register if available.
// If the hint is not available, falls back to normal allocation.
func (ra *RegisterAllocator) AllocHinted(hint Register) Register {
	// If hint is provided and available, use it
	if hint != NoHint && ra.isAvailable(hint) {
		ra.reserve(hint) // Mark as allocated
		if debugRegAlloc {
			fmt.Printf("[REGALLOC] HINT USED R%d\n", hint)
		}
		return hint
	}

	// Otherwise, allocate normally
	reg := ra.Alloc()
	if debugRegAlloc && hint != NoHint {
		fmt.Printf("[REGALLOC] HINT R%d not available, allocated R%d instead\n", hint, reg)
	}
	return reg
}

// AllocContiguous allocates a contiguous block of registers.
// Returns the first register of the block.
func (ra *RegisterAllocator) AllocContiguous(count int) Register {
	if count <= 0 {
		panic("AllocContiguous: count must be positive")
	}
	if count == 1 {
		return ra.Alloc()
	}

	// Find a contiguous block starting from nextReg
	firstReg := ra.nextReg

	// Check if we have enough room
	if int(firstReg)+count > 256 {
		panic("Compiler Error: Not enough registers for contiguous allocation")
	}

	// Allocate the block
	for i := 0; i < count; i++ {
		reg := firstReg + Register(i)
		if reg > ra.maxReg {
			ra.maxReg = reg
		}
	}

	ra.nextReg = firstReg + Register(count)

	if debugRegAlloc {
		fmt.Printf("[REGALLOC] CONTIGUOUS R%d-R%d (%d registers, nextReg now %d)\n",
			firstReg, firstReg+Register(count-1), count, ra.nextReg)
	}

	return firstReg
}

// isAvailable checks if a register is available for allocation.
func (ra *RegisterAllocator) isAvailable(reg Register) bool {
	// Check if register is pinned
	if ra.pinnedRegs[reg] {
		return false
	}

	// Check if it's in the free list
	for _, freeReg := range ra.freeRegs {
		if freeReg == reg {
			return true
		}
	}

	// Check if it's beyond nextReg (unallocated)
	return reg >= ra.nextReg
}

// reserve marks a specific register as allocated.
func (ra *RegisterAllocator) reserve(reg Register) {
	// Remove from free list if present
	for i, freeReg := range ra.freeRegs {
		if freeReg == reg {
			ra.freeRegs = append(ra.freeRegs[:i], ra.freeRegs[i+1:]...)
			if debugRegAlloc {
				fmt.Printf("[REGALLOC] RESERVE R%d (removed from free list)\n", reg)
			}
			return
		}
	}

	// If beyond nextReg, advance nextReg
	if reg >= ra.nextReg {
		ra.nextReg = reg + 1
		if reg > ra.maxReg {
			ra.maxReg = reg
		}
		if debugRegAlloc {
			fmt.Printf("[REGALLOC] RESERVE R%d (advanced nextReg to %d)\n", reg, ra.nextReg)
		}
	}
}

// Peek returns the index of the next register that *would* be allocated,
// without actually allocating it.
func (ra *RegisterAllocator) Peek() Register {
	return ra.nextReg
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
	ra.freeRegs = ra.freeRegs[:0]           // Clear free list (keeps allocated capacity)
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

// --- RegisterGroup for managing register lifetimes ---

// RegisterGroup manages a collection of registers with coordinated lifetime.
// This is useful for complex operations like function calls that need multiple
// registers to be live simultaneously.
type RegisterGroup struct {
	allocator *RegisterAllocator
	registers []Register
	parent    *RegisterGroup
	subgroups []*RegisterGroup
	released  bool
}

// NewGroup creates a new register group associated with this allocator.
func (ra *RegisterAllocator) NewGroup() *RegisterGroup {
	return &RegisterGroup{
		allocator: ra,
		registers: make([]Register, 0),
		parent:    nil,
		subgroups: make([]*RegisterGroup, 0),
		released:  false,
	}
}

// SubGroup creates a child group. When the parent is released,
// all subgroups are automatically released as well.
func (rg *RegisterGroup) SubGroup() *RegisterGroup {
	if rg.released {
		panic("Cannot create subgroup of released group")
	}
	subgroup := &RegisterGroup{
		allocator: rg.allocator,
		registers: make([]Register, 0),
		parent:    rg,
		subgroups: make([]*RegisterGroup, 0),
		released:  false,
	}
	rg.subgroups = append(rg.subgroups, subgroup)
	return subgroup
}

// Add registers a register with this group for lifetime management.
// The register will be freed when Release() is called, unless it's pinned.
func (rg *RegisterGroup) Add(reg Register) {
	if rg.released {
		panic("Cannot add register to released group")
	}
	rg.registers = append(rg.registers, reg)
	if debugRegAlloc {
		fmt.Printf("[REGGROUP] ADD R%d to group (now %d registers)\n", reg, len(rg.registers))
	}
}

// Registers returns a copy of the registers currently in this group.
func (rg *RegisterGroup) Registers() []Register {
	result := make([]Register, len(rg.registers))
	copy(result, rg.registers)
	return result
}

// Count returns the number of registers in this group.
func (rg *RegisterGroup) Count() int {
	return len(rg.registers)
}

// Linearize ensures registers are in a contiguous block and returns the first one.
// This is useful for function calls that require arguments in consecutive registers.
// If registers are already contiguous, no new allocation is needed.
// If not, allocates a new contiguous block that the caller should move values to.
func (rg *RegisterGroup) Linearize() (Register, error) {
	if rg.released {
		return 0, fmt.Errorf("cannot linearize released group")
	}
	if len(rg.registers) == 0 {
		return 0, fmt.Errorf("cannot linearize empty group")
	}
	if len(rg.registers) == 1 {
		return rg.registers[0], nil // Already "linear"
	}

	// Check if registers are already contiguous
	firstReg := rg.registers[0]
	isContiguous := true
	for i := 1; i < len(rg.registers); i++ {
		if rg.registers[i] != firstReg+Register(i) {
			isContiguous = false
			break
		}
	}

	if isContiguous {
		if debugRegAlloc {
			fmt.Printf("[REGGROUP] LINEARIZE OPTIMIZED: %d registers already contiguous starting at R%d\n", len(rg.registers), firstReg)
		}
		return firstReg, nil
	}

	// Not contiguous, allocate new contiguous block
	newFirstReg := rg.allocator.AllocContiguous(len(rg.registers))

	if debugRegAlloc {
		fmt.Printf("[REGGROUP] LINEARIZE %d registers: not contiguous, allocated new block starting at R%d\n", len(rg.registers), newFirstReg)
	}

	// Note: The caller needs to emit move instructions to transfer values:
	// for i, srcReg := range rg.registers {
	//     targetReg := newFirstReg + Register(i)
	//     compiler.emitMove(targetReg, srcReg, line)
	// }

	return newFirstReg, nil
}

// Release frees all registers in this group and all subgroups.
// Registers that are pinned will not be freed.
func (rg *RegisterGroup) Release() {
	if rg.released {
		return // Already released
	}

	if debugRegAlloc {
		fmt.Printf("[REGGROUP] RELEASE group with %d registers and %d subgroups\n", len(rg.registers), len(rg.subgroups))
	}

	// Release all subgroups first
	for _, subgroup := range rg.subgroups {
		subgroup.Release()
	}

	// Free all registers in this group (respecting pinning)
	for _, reg := range rg.registers {
		rg.allocator.Free(reg)
	}

	rg.released = true
}

// IsReleased returns whether this group has been released.
func (rg *RegisterGroup) IsReleased() bool {
	return rg.released
}

// --- Optional/Future ---

func (r Register) String() string {
	return fmt.Sprintf("R%d", r)
}

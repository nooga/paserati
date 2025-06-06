package compiler

import (
	"testing"
)

func TestNewRegisterAllocator(t *testing.T) {
	ra := NewRegisterAllocator()

	if ra.nextReg != 0 {
		t.Errorf("Expected nextReg to be 0, got %d", ra.nextReg)
	}
	if ra.maxReg != 0 {
		t.Errorf("Expected maxReg to be 0, got %d", ra.maxReg)
	}
	if len(ra.freeRegs) != 0 {
		t.Errorf("Expected empty free list, got %v", ra.freeRegs)
	}
	if len(ra.pinnedRegs) != 0 {
		t.Errorf("Expected empty pinned registers, got %v", ra.pinnedRegs)
	}
}

func TestBasicAllocation(t *testing.T) {
	ra := NewRegisterAllocator()

	// Test sequential allocation
	reg1 := ra.Alloc()
	if reg1 != 0 {
		t.Errorf("Expected first register to be 0, got %d", reg1)
	}

	reg2 := ra.Alloc()
	if reg2 != 1 {
		t.Errorf("Expected second register to be 1, got %d", reg2)
	}

	reg3 := ra.Alloc()
	if reg3 != 2 {
		t.Errorf("Expected third register to be 2, got %d", reg3)
	}
}

func TestAllocHinted(t *testing.T) {
	ra := NewRegisterAllocator()

	// Test hint with NoHint (should allocate sequentially)
	reg1 := ra.AllocHinted(NoHint)
	if reg1 != 0 {
		t.Errorf("Expected NoHint to allocate register 0, got %d", reg1)
	}

	// Test hint with available register
	reg2 := ra.AllocHinted(5)
	if reg2 != 5 {
		t.Errorf("Expected hinted register 5, got %d", reg2)
	}

	// Test hint with unavailable register (already allocated)
	reg3 := ra.AllocHinted(5) // R5 is already taken
	if reg3 == 5 {
		t.Errorf("Expected hint 5 to be rejected, but got %d", reg3)
	}
	// Should allocate the next sequential register (R6, since nextReg advanced to 6 after allocating R5)
	if reg3 != 6 {
		t.Errorf("Expected fallback allocation to be 6, got %d", reg3)
	}
}

func TestAllocHintedWithFreeList(t *testing.T) {
	ra := NewRegisterAllocator()

	// Allocate some registers
	_ = ra.Alloc()     // R0
	reg2 := ra.Alloc() // R1
	_ = ra.Alloc()     // R2

	// Free one
	ra.Free(reg2) // R1 goes to free list

	// Test hint for freed register
	reg4 := ra.AllocHinted(1)
	if reg4 != 1 {
		t.Errorf("Expected hinted register 1 from free list, got %d", reg4)
	}

	// Test hint for register beyond nextReg
	reg5 := ra.AllocHinted(10)
	if reg5 != 10 {
		t.Errorf("Expected hinted register 10, got %d", reg5)
	}
}

func TestAllocContiguous(t *testing.T) {
	ra := NewRegisterAllocator()

	// Test single register (should use normal Alloc)
	reg1 := ra.AllocContiguous(1)
	if reg1 != 0 {
		t.Errorf("Expected contiguous(1) to return 0, got %d", reg1)
	}

	// Test multiple contiguous registers
	reg2 := ra.AllocContiguous(3)
	if reg2 != 1 {
		t.Errorf("Expected contiguous(3) to return 1, got %d", reg2)
	}

	// Verify nextReg advanced correctly
	if ra.nextReg != 4 {
		t.Errorf("Expected nextReg to be 4, got %d", ra.nextReg)
	}

	// Test that next allocation continues correctly
	reg3 := ra.Alloc()
	if reg3 != 4 {
		t.Errorf("Expected next allocation to be 4, got %d", reg3)
	}
}

func TestAllocContiguousEdgeCases(t *testing.T) {
	ra := NewRegisterAllocator()

	// Test zero count (should panic)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for AllocContiguous(0)")
		}
	}()
	ra.AllocContiguous(0)
}

func TestPinning(t *testing.T) {
	ra := NewRegisterAllocator()

	reg1 := ra.Alloc()
	reg2 := ra.Alloc()

	// Pin a register
	ra.Pin(reg1)
	if !ra.IsPinned(reg1) {
		t.Errorf("Expected register %d to be pinned", reg1)
	}
	if ra.IsPinned(reg2) {
		t.Errorf("Expected register %d to not be pinned", reg2)
	}

	// Try to free pinned register (should be ignored)
	ra.Free(reg1)
	if ra.IsInFreeList(reg1) {
		t.Errorf("Pinned register %d should not be in free list", reg1)
	}

	// Free unpinned register (should work)
	ra.Free(reg2)
	if !ra.IsInFreeList(reg2) {
		t.Errorf("Unpinned register %d should be in free list", reg2)
	}

	// Unpin and then free
	ra.Unpin(reg1)
	if ra.IsPinned(reg1) {
		t.Errorf("Expected register %d to be unpinned", reg1)
	}

	ra.Free(reg1)
	if !ra.IsInFreeList(reg1) {
		t.Errorf("Unpinned register %d should be in free list after freeing", reg1)
	}
}

func TestReuseFromFreeList(t *testing.T) {
	ra := NewRegisterAllocator()

	// Allocate several registers
	reg1 := ra.Alloc() // R0
	reg2 := ra.Alloc() // R1
	_ = ra.Alloc()     // R2

	// Free some registers
	ra.Free(reg1) // R0 -> free list
	ra.Free(reg2) // R1 -> free list

	// Next allocation should reuse from free list (LIFO order)
	reg4 := ra.Alloc()
	if reg4 != reg2 { // Should get R1 (last freed)
		t.Errorf("Expected to reuse register %d, got %d", reg2, reg4)
	}

	reg5 := ra.Alloc()
	if reg5 != reg1 { // Should get R0 (first freed)
		t.Errorf("Expected to reuse register %d, got %d", reg1, reg5)
	}

	// Next allocation should allocate new register
	reg6 := ra.Alloc()
	if reg6 != 3 {
		t.Errorf("Expected new register 3, got %d", reg6)
	}
}

func TestIsAvailable(t *testing.T) {
	ra := NewRegisterAllocator()

	// Test unallocated register
	if !ra.isAvailable(5) {
		t.Errorf("Expected register 5 to be available")
	}

	// Allocate a register
	reg1 := ra.Alloc() // R0
	if ra.isAvailable(0) {
		t.Errorf("Expected allocated register 0 to be unavailable")
	}

	// Free and check availability
	ra.Free(reg1)
	if !ra.isAvailable(0) {
		t.Errorf("Expected freed register 0 to be available")
	}

	// Pin a register and check availability
	ra.Pin(0)
	if ra.isAvailable(0) {
		t.Errorf("Expected pinned register 0 to be unavailable")
	}

	ra.Unpin(0)
	if !ra.isAvailable(0) {
		t.Errorf("Expected unpinned register 0 to be available")
	}
}

func TestReserve(t *testing.T) {
	ra := NewRegisterAllocator()

	// Reserve a register beyond nextReg
	ra.reserve(5)
	if ra.nextReg != 6 {
		t.Errorf("Expected nextReg to advance to 6, got %d", ra.nextReg)
	}
	if ra.maxReg != 5 {
		t.Errorf("Expected maxReg to be 5, got %d", ra.maxReg)
	}

	// Try to allocate normally - should skip reserved register
	reg1 := ra.Alloc()
	if reg1 != 6 {
		t.Errorf("Expected allocation to skip reserved register, got %d", reg1)
	}

	// Reserve from free list
	ra.Free(reg1)
	ra.reserve(reg1)
	if ra.IsInFreeList(reg1) {
		t.Errorf("Expected reserved register to be removed from free list")
	}
}

func TestMaxRegs(t *testing.T) {
	ra := NewRegisterAllocator()

	// Initially should be 0
	if ra.MaxRegs() != 0 {
		t.Errorf("Expected MaxRegs to be 0 initially, got %d", ra.MaxRegs())
	}

	// Allocate some registers
	_ = ra.Alloc()        // R0
	_ = ra.Alloc()        // R1
	_ = ra.AllocHinted(5) // R5

	// MaxRegs should be highest + 1
	if ra.MaxRegs() != 6 {
		t.Errorf("Expected MaxRegs to be 6, got %d", ra.MaxRegs())
	}
}

func TestReset(t *testing.T) {
	ra := NewRegisterAllocator()

	// Allocate and modify state
	reg1 := ra.Alloc()
	reg2 := ra.Alloc()
	ra.Pin(reg1)
	ra.Free(reg2)

	// Reset
	ra.Reset()

	// Check all state is reset
	if ra.nextReg != 0 {
		t.Errorf("Expected nextReg to be reset to 0, got %d", ra.nextReg)
	}
	if ra.maxReg != 0 {
		t.Errorf("Expected maxReg to be reset to 0, got %d", ra.maxReg)
	}
	if len(ra.freeRegs) != 0 {
		t.Errorf("Expected free list to be empty after reset, got %v", ra.freeRegs)
	}
	if len(ra.pinnedRegs) != 0 {
		t.Errorf("Expected pinned registers to be empty after reset, got %v", ra.pinnedRegs)
	}
}

func TestHintedAllocationWithComplexScenario(t *testing.T) {
	ra := NewRegisterAllocator()

	// Create a complex scenario
	reg1 := ra.Alloc()    // R0
	reg2 := ra.Alloc()    // R1
	_ = ra.AllocHinted(5) // R5
	ra.Free(reg1)         // R0 -> free list
	ra.Pin(reg2)          // R1 pinned

	// Try to hint for pinned register (should fail)
	reg4 := ra.AllocHinted(1)
	if reg4 == 1 {
		t.Errorf("Expected hint for pinned register to fail")
	}
	if reg4 != 0 { // Should get R0 from free list
		t.Errorf("Expected fallback to get R0 from free list, got %d", reg4)
	}

	// Try to hint for register in free list (should succeed)
	ra.Free(reg4) // R0 back to free list
	reg5 := ra.AllocHinted(0)
	if reg5 != 0 {
		t.Errorf("Expected hint for register in free list to succeed, got %d", reg5)
	}

	// Try to hint for register beyond allocated range (should succeed)
	reg6 := ra.AllocHinted(10)
	if reg6 != 10 {
		t.Errorf("Expected hint for unallocated register to succeed, got %d", reg6)
	}
}

// Test the String method for Register
func TestRegisterString(t *testing.T) {
	reg := Register(42)
	expected := "R42"
	if reg.String() != expected {
		t.Errorf("Expected %s, got %s", expected, reg.String())
	}
}

// Benchmark basic allocation
func BenchmarkAlloc(b *testing.B) {
	ra := NewRegisterAllocator()
	regs := make([]Register, 0, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reg := ra.Alloc()
		regs = append(regs, reg)
		if len(regs) >= 10 { // Free every 10 allocations to avoid exhaustion
			for _, r := range regs {
				ra.Free(r)
			}
			regs = regs[:0]
		}
	}
}

// Benchmark hinted allocation
func BenchmarkAllocHinted(b *testing.B) {
	ra := NewRegisterAllocator()
	regs := make([]Register, 0, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hint := Register(i % 50) // Cycle through hints
		reg := ra.AllocHinted(hint)
		regs = append(regs, reg)
		if len(regs) >= 10 { // Free every 10 allocations to avoid exhaustion
			for _, r := range regs {
				ra.Free(r)
			}
			regs = regs[:0]
		}
	}
}

// --- RegisterGroup Tests ---

func TestNewGroup(t *testing.T) {
	ra := NewRegisterAllocator()

	group := ra.NewGroup()
	if group == nil {
		t.Errorf("Expected NewGroup to return a group")
	}
	if group.allocator != ra {
		t.Errorf("Expected group allocator to be the same as ra")
	}
	if group.Count() != 0 {
		t.Errorf("Expected new group to be empty, got %d registers", group.Count())
	}
	if group.IsReleased() {
		t.Errorf("Expected new group to not be released")
	}
}

func TestGroupAddAndRegisters(t *testing.T) {
	ra := NewRegisterAllocator()
	group := ra.NewGroup()

	// Add some registers
	reg1 := ra.Alloc()
	reg2 := ra.Alloc()
	reg3 := ra.Alloc()

	group.Add(reg1)
	group.Add(reg2)
	group.Add(reg3)

	if group.Count() != 3 {
		t.Errorf("Expected group to have 3 registers, got %d", group.Count())
	}

	regs := group.Registers()
	if len(regs) != 3 {
		t.Errorf("Expected 3 registers, got %d", len(regs))
	}

	// Check that we get a copy (modifying the slice doesn't affect the group)
	regs[0] = 99
	if group.Registers()[0] == 99 {
		t.Errorf("Expected Registers() to return a copy")
	}
}

func TestSubGroup(t *testing.T) {
	ra := NewRegisterAllocator()
	parent := ra.NewGroup()

	child1 := parent.SubGroup()
	child2 := parent.SubGroup()

	if child1 == nil || child2 == nil {
		t.Errorf("Expected SubGroup to return valid groups")
	}
	if child1.parent != parent || child2.parent != parent {
		t.Errorf("Expected subgroups to have correct parent")
	}
	if len(parent.subgroups) != 2 {
		t.Errorf("Expected parent to track 2 subgroups, got %d", len(parent.subgroups))
	}
}

func TestGroupRelease(t *testing.T) {
	ra := NewRegisterAllocator()
	group := ra.NewGroup()

	// Add some registers
	reg1 := ra.Alloc()
	reg2 := ra.Alloc()
	group.Add(reg1)
	group.Add(reg2)

	// Verify registers are not in free list initially
	if ra.IsInFreeList(reg1) || ra.IsInFreeList(reg2) {
		t.Errorf("Registers should not be in free list before release")
	}

	// Release the group
	group.Release()

	// Verify group is marked as released
	if !group.IsReleased() {
		t.Errorf("Expected group to be marked as released")
	}

	// Verify registers are now in free list
	if !ra.IsInFreeList(reg1) || !ra.IsInFreeList(reg2) {
		t.Errorf("Registers should be in free list after release")
	}
}

func TestGroupReleaseWithSubgroups(t *testing.T) {
	ra := NewRegisterAllocator()
	parent := ra.NewGroup()
	child1 := parent.SubGroup()
	child2 := parent.SubGroup()

	// Add registers to different groups
	parentReg := ra.Alloc()
	child1Reg := ra.Alloc()
	child2Reg := ra.Alloc()

	parent.Add(parentReg)
	child1.Add(child1Reg)
	child2.Add(child2Reg)

	// Release parent should release all children
	parent.Release()

	// Verify all groups are released
	if !parent.IsReleased() {
		t.Errorf("Expected parent group to be released")
	}
	if !child1.IsReleased() {
		t.Errorf("Expected child1 group to be released")
	}
	if !child2.IsReleased() {
		t.Errorf("Expected child2 group to be released")
	}

	// Verify all registers are freed
	if !ra.IsInFreeList(parentReg) {
		t.Errorf("Parent register should be freed")
	}
	if !ra.IsInFreeList(child1Reg) {
		t.Errorf("Child1 register should be freed")
	}
	if !ra.IsInFreeList(child2Reg) {
		t.Errorf("Child2 register should be freed")
	}
}

func TestGroupReleaseWithPinnedRegisters(t *testing.T) {
	ra := NewRegisterAllocator()
	group := ra.NewGroup()

	reg1 := ra.Alloc()
	reg2 := ra.Alloc()

	// Pin one register
	ra.Pin(reg1)

	group.Add(reg1)
	group.Add(reg2)

	// Release group
	group.Release()

	// Pinned register should not be freed
	if ra.IsInFreeList(reg1) {
		t.Errorf("Pinned register should not be freed")
	}

	// Unpinned register should be freed
	if !ra.IsInFreeList(reg2) {
		t.Errorf("Unpinned register should be freed")
	}

	// Group should still be marked as released
	if !group.IsReleased() {
		t.Errorf("Group should be marked as released")
	}
}

func TestGroupLinearize(t *testing.T) {
	ra := NewRegisterAllocator()
	group := ra.NewGroup()

	// Test empty group
	_, err := group.Linearize()
	if err == nil {
		t.Errorf("Expected error when linearizing empty group")
	}

	// Test single register (should return the same register)
	reg1 := ra.Alloc()
	group.Add(reg1)

	linearReg, err := group.Linearize()
	if err != nil {
		t.Errorf("Unexpected error linearizing single register: %v", err)
	}
	if linearReg != reg1 {
		t.Errorf("Expected linearized register to be %d, got %d", reg1, linearReg)
	}

	// Test multiple registers
	reg2 := ra.Alloc()
	reg3 := ra.Alloc()
	group.Add(reg2)
	group.Add(reg3)

	linearStart, err := group.Linearize()
	if err != nil {
		t.Errorf("Unexpected error linearizing multiple registers: %v", err)
	}

	// Should return a register that starts a contiguous block
	// The exact value depends on allocator state, but it should be valid
	if linearStart > 250 { // Reasonable upper bound
		t.Errorf("Linearized start register seems too high: %d", linearStart)
	}
}

func TestGroupLinearizeReleasedGroup(t *testing.T) {
	ra := NewRegisterAllocator()
	group := ra.NewGroup()

	reg1 := ra.Alloc()
	group.Add(reg1)
	group.Release()

	// Should not be able to linearize released group
	_, err := group.Linearize()
	if err == nil {
		t.Errorf("Expected error when linearizing released group")
	}
}

func TestGroupOperationsOnReleasedGroup(t *testing.T) {
	ra := NewRegisterAllocator()
	group := ra.NewGroup()

	reg1 := ra.Alloc()
	group.Add(reg1)
	group.Release()

	// Should panic when adding to released group
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when adding to released group")
		}
	}()

	reg2 := ra.Alloc()
	group.Add(reg2)
}

func TestSubGroupOnReleasedGroup(t *testing.T) {
	ra := NewRegisterAllocator()
	group := ra.NewGroup()

	group.Release()

	// Should panic when creating subgroup of released group
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when creating subgroup of released group")
		}
	}()

	group.SubGroup()
}

func TestGroupDoubleRelease(t *testing.T) {
	ra := NewRegisterAllocator()
	group := ra.NewGroup()

	reg1 := ra.Alloc()
	group.Add(reg1)

	// First release
	group.Release()
	if !group.IsReleased() {
		t.Errorf("Expected group to be released after first Release()")
	}

	// Second release should be safe (no-op)
	group.Release()
	if !group.IsReleased() {
		t.Errorf("Expected group to still be released after second Release()")
	}
}

// Test a realistic function call scenario
func TestGroupFunctionCallScenario(t *testing.T) {
	ra := NewRegisterAllocator()

	// Simulate compiling: func(arg1, arg2, arg3)
	callGroup := ra.NewGroup()

	// Compile function expression
	funcReg := ra.Alloc()
	callGroup.Add(funcReg)

	// Compile arguments in a subgroup
	argGroup := callGroup.SubGroup()
	arg1Reg := ra.Alloc()
	arg2Reg := ra.Alloc()
	arg3Reg := ra.Alloc()

	argGroup.Add(arg1Reg)
	argGroup.Add(arg2Reg)
	argGroup.Add(arg3Reg)

	// Linearize arguments for the call
	firstArgReg, err := argGroup.Linearize()
	if err != nil {
		t.Errorf("Failed to linearize arguments: %v", err)
	}

	// Allocate result register
	resultReg := ra.Alloc()

	// Simulate emitting the call instruction
	// (In real code: c.emitCall(resultReg, funcReg, firstArgReg, 3))
	// For testing, just verify the registers are valid
	if firstArgReg > 250 || resultReg > 250 {
		t.Errorf("Register values seem invalid: firstArg=%d, result=%d", firstArgReg, resultReg)
	}

	// Clean up
	callGroup.Release()

	// Verify cleanup
	if !callGroup.IsReleased() || !argGroup.IsReleased() {
		t.Errorf("Expected all groups to be released")
	}

	// The original registers should be freed (if not pinned)
	if !ra.IsInFreeList(funcReg) {
		t.Errorf("Function register should be freed")
	}
}

// Benchmark group operations
func BenchmarkGroupOperations(b *testing.B) {
	ra := NewRegisterAllocator()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		group := ra.NewGroup()

		// Add a few registers
		for j := 0; j < 5; j++ {
			reg := ra.AllocHinted(NoHint)
			group.Add(reg)
		}

		// Create a subgroup
		subgroup := group.SubGroup()
		reg := ra.AllocHinted(NoHint)
		subgroup.Add(reg)

		// Release everything
		group.Release()
	}
}

package vm

import "testing"

func TestRegisterDirectoryBasicPushPop(t *testing.T) {
	d := newRegisterDirectory(10)

	win, m0, _, ok := d.push(5)
	if !ok {
		t.Fatalf("push(5) failed")
	}
	if m0 != (registerMark{block: 0, offset: 0}) {
		t.Fatalf("expected first push at (0,0), got %+v", m0)
	}
	if len(win) != 5 {
		t.Fatalf("expected window of 5, got %d", len(win))
	}

	win2, m1, _, ok := d.push(3)
	if !ok {
		t.Fatalf("push(3) failed")
	}
	if m1 != (registerMark{block: 0, offset: 5}) {
		t.Fatalf("expected second push at (0,5), got %+v", m1)
	}
	if len(win2) != 3 {
		t.Fatalf("expected window of 3, got %d", len(win2))
	}

	// LIFO pop of the second window must restore the mark exactly.
	d.popTo(m1)
	if d.mark() != m1 {
		t.Fatalf("expected cursor at %+v after popTo, got %+v", m1, d.mark())
	}
	d.popTo(m0)
	if d.mark() != m0 {
		t.Fatalf("expected cursor at %+v after popTo, got %+v", m0, d.mark())
	}
}

func TestRegisterDirectoryTailWasteAcrossBlockBoundary(t *testing.T) {
	d := newRegisterDirectory(10)

	// Fill block 0 to offset 200, leaving 56 slots in its tail.
	_, _, _, ok := d.push(200)
	if !ok {
		t.Fatalf("push(200) failed")
	}

	// A window of 100 doesn't fit in the remaining 56 - must skip to block 1,
	// wasting the 56-slot tail rather than splitting across blocks.
	win, start, _, ok := d.push(100)
	if !ok {
		t.Fatalf("push(100) failed")
	}
	if start.block != 1 || start.offset != 0 {
		t.Fatalf("expected skip to (1,0), got %+v", start)
	}
	if len(win) != 100 {
		t.Fatalf("expected window of 100, got %d", len(win))
	}
	if len(d.blocks) != 2 {
		t.Fatalf("expected 2 blocks allocated, got %d", len(d.blocks))
	}

	// Popping back below block 0's mark must make its wasted tail available
	// again: the very next push that fits should land back in block 0.
	d.popTo(registerMark{block: 0, offset: 200})
	win2, start2, _, ok := d.push(50)
	if !ok {
		t.Fatalf("push(50) after popTo failed")
	}
	if start2.block != 0 || start2.offset != 200 {
		t.Fatalf("expected reuse of block 0's wasted tail at (0,200), got %+v", start2)
	}
	if len(win2) != 50 {
		t.Fatalf("expected window of 50, got %d", len(win2))
	}
}

// TestRegisterDirectoryPushReturnsDistinctReleaseAndWindowStartAcrossSkip
// pins down the exact bug the B4 VM wiring hit on first attempt: when a push
// skips to a fresh block, the window's own location (`start`) and the mark
// that must be restored on release (`release`) are DIFFERENT, and a caller
// that conflates them (using `start` for both, as CallFrame.regSlotBeforePush
// originally did) never recovers the wasted tail slots - every return would
// leave the cursor sitting at the block boundary instead of back where the
// caller's own cursor was before the call, permanently stranding blocks over
// a long call chain. See push's own doc comment for the full story.
func TestRegisterDirectoryPushReturnsDistinctReleaseAndWindowStartAcrossSkip(t *testing.T) {
	d := newRegisterDirectory(10)

	// Fill block 0 to offset 254, leaving only 2 slots in its tail.
	_, _, _, ok := d.push(254)
	if !ok {
		t.Fatalf("push(254) failed")
	}
	preSkipMark := d.mark() // {block:0, offset:254} - what a caller's cursor looks like right before the next push

	// A window of 18 doesn't fit in the remaining 2 - must skip to block 1.
	_, start, release, ok := d.push(18)
	if !ok {
		t.Fatalf("push(18) failed")
	}
	if start == release {
		t.Fatalf("expected start and release to diverge across a block skip, both were %+v", start)
	}
	if release != preSkipMark {
		t.Fatalf("expected release to equal the pre-push cursor %+v, got %+v", preSkipMark, release)
	}
	if start != (registerMark{block: 1, offset: 0}) {
		t.Fatalf("expected the window itself to live at (1,0), got %+v", start)
	}

	// Releasing with `release` (as CallFrame.regSlotBeforePush does) must
	// restore the pre-skip position, recovering the wasted tail slots.
	d.popTo(release)
	if d.mark() != preSkipMark {
		t.Fatalf("popTo(release) should restore %+v, got %+v", preSkipMark, d.mark())
	}
	// The 2 wasted slots at the end of block 0 must be reusable again now.
	_, reuseStart, _, ok := d.push(2)
	if !ok {
		t.Fatalf("push(2) after popTo(release) failed")
	}
	if reuseStart != preSkipMark {
		t.Fatalf("expected the wasted tail slots to be reused at %+v, got %+v", preSkipMark, reuseStart)
	}
}

func TestRegisterDirectoryWindowNeverSpansBlocks(t *testing.T) {
	d := newRegisterDirectory(10)
	_, _, _, _ = d.push(250) // leaves 6 slots in block 0's tail

	_, start, _, ok := d.push(10) // doesn't fit in 6 remaining - must move to block 1 entirely
	if !ok {
		t.Fatalf("push(10) failed")
	}
	if start.block != 1 {
		t.Fatalf("expected window to move entirely into block 1, got block %d", start.block)
	}
}

func TestRegisterDirectoryOverflowAtMaxBlocks(t *testing.T) {
	d := newRegisterDirectory(2) // only 2 blocks allowed

	_, _, _, ok := d.push(registerBlockSize) // fills block 0 exactly
	if !ok {
		t.Fatalf("push(registerBlockSize) failed")
	}
	_, _, _, ok = d.push(registerBlockSize) // fills block 1 exactly
	if !ok {
		t.Fatalf("second push(registerBlockSize) failed")
	}
	_, _, _, ok = d.push(1) // no room for a 3rd block
	if ok {
		t.Fatalf("expected overflow at maxBlocks=2, but push succeeded")
	}
}

func TestRegisterDirectoryPushExceedingBlockSizePanics(t *testing.T) {
	d := newRegisterDirectory(10)
	defer func() {
		if recover() == nil {
			t.Fatalf("expected push(registerBlockSize+1) to panic")
		}
	}()
	d.push(registerBlockSize + 1)
}

func TestRegisterDirectoryTrimReleasesDeepExcursion(t *testing.T) {
	d := newRegisterDirectory(1000)

	// Simulate a deep excursion: push+pop repeatedly to build up many blocks,
	// then return to shallow depth in one popTo.
	var marks []registerMark
	for i := 0; i < 50; i++ {
		_, m, _, ok := d.push(registerBlockSize) // one full block per push
		if !ok {
			t.Fatalf("push %d failed", i)
		}
		marks = append(marks, m)
	}
	if len(d.blocks) < 50 {
		t.Fatalf("expected at least 50 blocks allocated during excursion, got %d", len(d.blocks))
	}

	// Pop all the way back to the start.
	d.popTo(registerMark{block: 0, offset: 0})

	maxKept := 1 + registerDirectoryReserveBlocks
	if len(d.blocks) > maxKept {
		t.Fatalf("expected trim to release blocks beyond the reserve (%d), got %d blocks retained", maxKept, len(d.blocks))
	}
}

func TestRegisterDirectoryTryExpandInPlace(t *testing.T) {
	d := newRegisterDirectory(10)
	win, start, _, ok := d.push(10)
	if !ok {
		t.Fatalf("push(10) failed")
	}
	for i := range win {
		win[i] = IntegerValue(int32(i))
	}

	expanded, ok := d.tryExpand(start, 10, 20)
	if !ok {
		t.Fatalf("tryExpand in place failed")
	}
	if len(expanded) != 20 {
		t.Fatalf("expected expanded window of 20, got %d", len(expanded))
	}
	// Original contents must be preserved (same backing array, no copy needed).
	for i := 0; i < 10; i++ {
		if expanded[i].AsInteger() != int32(i) {
			t.Fatalf("expected expanded[%d]=%d, got %v", i, i, expanded[i])
		}
	}
	if d.mark() != (registerMark{block: 0, offset: 20}) {
		t.Fatalf("expected cursor advanced to (0,20), got %+v", d.mark())
	}
}

func TestRegisterDirectoryTryExpandFailsNearBlockEnd(t *testing.T) {
	d := newRegisterDirectory(10)
	_, first, _, _ := d.push(240) // leaves 16 slots in block 0
	// first isn't the topmost window anymore once we push again, but here
	// we test tryExpand directly on the still-topmost window.
	expanded, ok := d.tryExpand(first, 240, 250) // needs 10 more; only 16 left - fits
	if !ok {
		t.Fatalf("expected expand to 250 (needs 10 more of 16 remaining) to succeed")
	}
	if len(expanded) != 250 {
		t.Fatalf("expected window of 250, got %d", len(expanded))
	}

	_, ok = d.tryExpand(first, 250, 256+1)
	if ok {
		t.Fatalf("expected expand past registerBlockSize to fail")
	}
}

func TestRegisterDirectoryMoveRelocatesAndCopies(t *testing.T) {
	d := newRegisterDirectory(10)
	_, _, _, _ = d.push(10) // padding, so the frame's window starts at a non-zero offset
	win, start, _, ok := d.push(240)
	if !ok {
		t.Fatalf("push(240) failed")
	}
	if start.offset != 10 {
		t.Fatalf("expected frame window to start at offset 10, got %d", start.offset)
	}
	for i := range win {
		win[i] = IntegerValue(int32(i))
	}

	// tryExpand must fail here: offset 10 + 250 = 260 > registerBlockSize, so
	// it doesn't fit in place even though 250 itself is a legal window size -
	// move should relocate to a fresh block and preserve contents.
	if _, ok := d.tryExpand(start, 240, 250); ok {
		t.Fatalf("expected tryExpand to fail so move is exercised")
	}

	moved, newStart, ok := d.move(start, 240, 250)
	if !ok {
		t.Fatalf("move failed")
	}
	if newStart.block == start.block {
		t.Fatalf("expected move to relocate to a different block, stayed at block %d", newStart.block)
	}
	if len(moved) != 250 {
		t.Fatalf("expected moved window of 250, got %d", len(moved))
	}
	for i := 0; i < 240; i++ {
		if moved[i].AsInteger() != int32(i) {
			t.Fatalf("expected moved[%d]=%d after copy, got %v", i, i, moved[i])
		}
	}
}

func TestRegisterDirectoryMoveOverflowLeavesOldWindowIntact(t *testing.T) {
	d := newRegisterDirectory(1) // only 1 block available - no room to move into
	_, _, _, _ = d.push(10)         // padding, so the frame's window starts at offset 10
	win, start, _, ok := d.push(240)
	if !ok {
		t.Fatalf("push(240) failed")
	}
	win[0] = IntegerValue(42)

	// offset 10 + 250 = 260 > registerBlockSize, so this can't stay in place,
	// and with maxBlocks=1 there's no second block to relocate into either.
	_, _, ok = d.move(start, 240, 250)
	if ok {
		t.Fatalf("expected move to fail (no second block available)")
	}
	// Old window must remain exactly as it was - readable at its original
	// location with its original contents, and the cursor unchanged.
	still := d.window(start, 240)
	if still[0].AsInteger() != 42 {
		t.Fatalf("expected old window contents preserved after failed move")
	}
	if d.mark() != (registerMark{block: start.block, offset: start.offset + 240}) {
		t.Fatalf("expected cursor restored to just past the untouched old window")
	}
}

package vm

import "fmt"

// This file implements the B4 register-stack redesign
// (docs/runtime-production-roadmap.md#b4): a directory of lazily allocated,
// fixed-capacity blocks in place of one eagerly allocated flat
// `RegFileSize * MaxFrames` array. See registerDirectory's own doc comment
// for the design; this header covers why it's shaped the way it is.
//
// Two invariants make this safe:
//
//  1. Every frame's register window fits in exactly one block. The compiler
//     caps a function's RegisterSize at 255 (pkg/compiler/regalloc.go's
//     registerLimit; Register is a uint8 and Alloc/AllocContiguous panic into
//     a proper compile error - "register exhaustion: expression too deeply
//     nested" - well before that, turning what would otherwise be silent
//     truncation into a normal, catchable failure). TCO's in-place expansion
//     (OpTailCall) only ever grows a frame's allocatedRegSize to
//     max(oldSize, calleeFunc.RegisterSize) across a tail-call chain, never a
//     sum. So no window this VM can ever produce exceeds 255 registers, and
//     registerBlockSize (256) always has room to spare. The "oversized
//     window needs a dedicated block" case the roadmap anticipates is
//     therefore provably unreachable - push() panics if it's ever hit, which
//     would mean one of the two facts above stopped being true.
//
//  2. A block, once allocated, is never resized or moved. Only the
//     directory's own slice of block *pointers* grows or shrinks; each
//     block's backing array is a fixed-size field of the heap-allocated
//     registerBlock struct it belongs to. So a raw *Value taken into a
//     block's data (an open upvalue) stays valid for as long as that block
//     is retained, independent of how much the directory itself grows,
//     shrinks, or reallocates its outer slice - exactly the pointer-stability
//     guarantee the flat array gave open upvalues before, and the same
//     guarantee VM.frames already has for frame records (it's allocated once
//     at len(MaxFrames) and never reallocated - so this file only needs to
//     solve the register-storage half of "frame records also need stable
//     storage where pointers to them survive calls").

// registerBlockSize is the fixed capacity, in Values, of every block. See
// invariant 1 above for why this is always enough for any single frame's
// window.
const registerBlockSize = RegFileSize

// registerDirectoryReserveBlocks is how many trailing empty blocks popTo
// keeps allocated (rather than releasing to the GC) after the cursor
// retreats past them. Zero would make every pop-then-push crossing a block
// boundary alloc/free a block; a small fixed slack absorbs that oscillation
// (e.g. a loop that recurses just deep enough to cross a boundary and
// returns) without unboundedly retaining blocks from one deep, one-off
// excursion (the roadmap's "keep only a bounded reserve of empty segments
// after a deep request").
const registerDirectoryReserveBlocks = 4

// registerMark identifies a position in the register directory: a block
// index plus an offset within that block. It is the B4 replacement for a
// bare `nextRegSlot int` - the value recorded before a push
// (CallFrame.regSlotBeforePush) and restored on pop.
type registerMark struct {
	block  int
	offset int
}

// registerBlock is one fixed-capacity chunk of the register directory. See
// invariant 2 above for why its address is stable for as long as it's
// retained.
type registerBlock struct {
	data [registerBlockSize]Value
}

// registerDirectory replaces VM.registerStack ([]Value) + VM.nextRegSlot
// (int) with a growable list of fixed-capacity blocks and a single logical
// cursor (`cur`) that moves forward on push and backward on pop - the same
// shape as nextRegSlot, just expressed as (block, offset) instead of one
// flat int.
//
// Push and pop are strictly LIFO, matching the call stack's own discipline:
// a frame's window is always released before any frame pushed after it (the
// same assumption the old nextRegSlot design already depended on). That
// means pop never needs arithmetic - popTo just restores a previously
// recorded mark - which is what eliminates the entire class of bug B1 fixed
// (reclaiming the wrong *amount* of registers): there is no amount to get
// wrong anymore, only a mark to restore.
//
// A push that doesn't fit in the current block's tail moves to the next
// block, wasting the unused tail slots for as long as the frame that forced
// the move (or anything above it) stays active. That waste is bounded
// (< registerBlockSize per skip) and self-healing: popping back below the
// block that has room makes those slots available again. This is the
// standard tradeoff of packing many small frames into fixed-size blocks
// instead of giving every frame its own precisely-sized allocation - it's
// where B4's memory savings come from, and the reason a frame's window
// "must remain contiguous within one block" per the roadmap rather than
// spanning two.
type registerDirectory struct {
	blocks []*registerBlock
	cur    registerMark
	// maxBlocks is the hard cap on the number of blocks in use (cur.block),
	// the B4 equivalent of the old flat array's fixed length
	// (RegFileSize*MaxFrames). It is set to MaxFrames at construction: the
	// worst case - every active frame forcing its own block skip, zero
	// packing benefit - needs exactly one block per frame, so this preserves
	// the exact same absolute worst-case register-space ceiling the flat
	// array had, while typical workloads (many frames packed per block) use
	// far less.
	maxBlocks int
}

// newRegisterDirectory creates an empty directory with no blocks allocated
// yet (the first push lazily allocates the first block) and a hard cap of
// maxBlocks blocks in use at once.
func newRegisterDirectory(maxBlocks int) *registerDirectory {
	return &registerDirectory{
		blocks:    make([]*registerBlock, 0, 8),
		cur:       registerMark{block: 0, offset: 0},
		maxBlocks: maxBlocks,
	}
}

// reset clears every Value in every block currently in use (releasing any
// object/closure/etc. references they hold, so Reset() doesn't leak large
// retained values across VM reuse) and returns the cursor to the very
// start, trimming blocks back to the reserve exactly as popTo would from a
// popTo(registerMark{0,0}).
func (d *registerDirectory) reset() {
	for i := 0; i <= d.cur.block && i < len(d.blocks); i++ {
		blk := d.blocks[i]
		for j := range blk.data {
			blk.data[j] = Undefined
		}
	}
	d.popTo(registerMark{block: 0, offset: 0})
}

// mark returns the directory's current cursor - the position the next push
// will start at, and the value to record as a frame's regSlotBeforePush
// immediately before pushing its window.
func (d *registerDirectory) mark() registerMark {
	return d.cur
}

// push allocates a contiguous window of n registers. It returns:
//
//   - window: the register slice itself (for CallFrame.registers).
//   - start: the mark identifying where the window actually lives (for
//     CallFrame.regWindowStart - what TCO's tryExpand/move, reclaimUnwoundRegisters,
//     and checkRegWindowRelease need to reason about this window's own bounds).
//   - release: the directory's cursor as it was *before* this push (for
//     CallFrame.regSlotBeforePush - what popTo needs to correctly restore
//     when this window is released).
//   - ok=false if the register-space budget (maxBlocks) is exhausted, the
//     direct analog of the old `nextRegSlot+n > len(registerStack)` check.
//
// start and release are equal exactly when the window fit in the current
// block's tail (the common case - most pushes). They diverge when the push
// had to skip to a fresh block, wasting the old block's remaining tail
// slots: start then points into the new block, while release still points
// at the old cursor position, just before those wasted slots. Popping with
// release (not start) is what makes that waste self-healing - see popTo and
// registerDirectory's own doc comment. A caller that used `start` for both
// roles would never recover the wasted slots: every return would leave the
// cursor sitting at the block boundary instead of back where the caller's
// own cursor was before making the call, permanently stranding a growing
// number of blocks over a long call chain that crosses many block
// boundaries. This was exactly the first (and only) real bug the B4 wiring
// hit - caught immediately by the smoke suite once StrictRegWindowChecks
// was on, rather than shipping as a slow, hard-to-attribute memory leak.
func (d *registerDirectory) push(n int) (window []Value, start registerMark, release registerMark, ok bool) {
	release = d.cur

	if n > registerBlockSize {
		// See invariant 1 in the file header: no real function/TCO chain can
		// produce a window this large. Panicking (instead of silently
		// handling it) keeps that assumption honest if it's ever violated.
		panic(fmt.Sprintf(
			"registerDirectory.push: requested window of %d registers exceeds block size %d - "+
				"impossible for any window this VM can produce (compiler caps a function's "+
				"RegisterSize at 255, and TCO only ever takes the max across a tail-call chain, "+
				"never a sum); this means that invariant was violated elsewhere",
			n, registerBlockSize))
	}

	if d.cur.offset+n > registerBlockSize {
		// Doesn't fit in the current block's tail: move to a fresh block.
		if d.cur.block+1 >= d.maxBlocks {
			return nil, registerMark{}, registerMark{}, false
		}
		d.cur = registerMark{block: d.cur.block + 1, offset: 0}
	}

	if d.cur.block >= len(d.blocks) {
		d.blocks = append(d.blocks, &registerBlock{})
	}

	start = d.cur
	window = d.blocks[d.cur.block].data[d.cur.offset : d.cur.offset+n]
	d.cur.offset += n
	return window, start, release, true
}

// window returns the slice for an already-allocated (start, n) window,
// without allocating anything - used when a frame's mark/size is already
// known (e.g. a suspended generator/async frame resuming into the window it
// was given when its body first started, or TCO recomputing the current
// frame's slice after expanding it).
func (d *registerDirectory) window(start registerMark, n int) []Value {
	return d.blocks[start.block].data[start.offset : start.offset+n]
}

// popTo restores the directory's cursor to a previously recorded mark,
// releasing every register allocated since (LIFO, so this is always exactly
// "everything the popped frame and anything it called are done with") and
// opportunistically trims trailing now-empty blocks back to a bounded
// reserve. Because push/pop are strictly LIFO, this alone is always
// correct - unlike the old `nextRegSlot -= amount`, there is no amount that
// can be wrong.
func (d *registerDirectory) popTo(mark registerMark) {
	d.cur = mark
	d.trim()
}

// trim releases blocks beyond a small fixed reserve past the current
// cursor, letting the GC reclaim them. See registerDirectoryReserveBlocks
// for why a reserve is kept rather than trimming to exactly cur.block.
func (d *registerDirectory) trim() {
	keep := d.cur.block + 1 + registerDirectoryReserveBlocks
	if len(d.blocks) > keep {
		d.blocks = d.blocks[:keep]
	}
}

// wouldFit reports whether a push of n registers would currently succeed,
// without allocating or mutating anything. Used at call sites that want to
// report a clean overflow error *before* doing unrelated setup work (e.g.
// resolving a constructor's prototype) that would otherwise be wasted -
// the same early-bailout role the old `nextRegSlot+n > len(registerStack)`
// check played, kept separate from the actual push done later at the point
// where the frame's window is really allocated.
func (d *registerDirectory) wouldFit(n int) bool {
	if n > registerBlockSize {
		return false
	}
	if d.cur.offset+n <= registerBlockSize {
		return true
	}
	return d.cur.block+1 < d.maxBlocks
}

// wouldFitExpansion reports, without allocating or mutating anything,
// whether expanding the topmost window (starting at `start`) to newSize
// would succeed - either in place, or via a move to a fresh block. Used by
// TCO (OpTailCall) to decide whether it can proceed *before* closing the
// current frame's upvalues, which must happen before an actual move (so the
// move doesn't leave a stale open upvalue pointing at contents that have
// been copied elsewhere) - tryExpand's in-place case doesn't have that
// ordering requirement, but checking both cases here up front keeps the
// caller's decide-then-commit structure simple.
func (d *registerDirectory) wouldFitExpansion(start registerMark, newSize int) bool {
	if start.offset+newSize <= registerBlockSize {
		return true // Fits in place.
	}
	// Doesn't fit in place: a move would need a fresh block. Any single
	// block always has room for newSize (invariant 1: no real window
	// exceeds 255 registers), so this is just asking whether one more block
	// is available at all.
	return start.block+1 < d.maxBlocks
}

// tryExpand grows an in-place window from oldSize to newSize without moving
// it, when the window's block has room right after it. Used by TCO
// (OpTailCall), which reuses the *current, topmost* frame's window and must
// never shrink it - `start` must be the most recently pushed window (i.e.
// sit exactly at the cursor before this call), which is always true for TCO
// since it only ever expands the frame currently on top of the stack.
//
// ok=false means the window's block doesn't have room; the caller should
// fall back to move(), which relocates the window instead.
func (d *registerDirectory) tryExpand(start registerMark, oldSize, newSize int) (window []Value, ok bool) {
	if newSize <= oldSize {
		return d.window(start, oldSize), true // Never shrinks; nothing to do.
	}
	if start.offset+newSize > registerBlockSize {
		return nil, false
	}
	d.cur = registerMark{block: start.block, offset: start.offset + newSize}
	return d.blocks[start.block].data[start.offset : start.offset+newSize], true
}

// move relocates a window to a fresh push and copies its live contents,
// used by TCO as the fallback when tryExpand can't satisfy the new size in
// place (the window's current block doesn't have enough room after it).
// Unlike an ordinary call - which always allocates a brand new window - TCO
// reuses an existing one, so growing it can require an actual move+copy
// that the old flat-array design never needed (there, "expand" was always
// just extending the same contiguous slice).
//
// This is only safe because the caller has already closed the frame's open
// upvalues (closeFrameUpvalues) before calling move: closing converts any
// raw *Value pointing into the window's old slots into a value copied
// elsewhere, so nothing is left holding a pointer into the slots being
// abandoned here.
//
// ok=false (register-space exhausted) leaves the directory untouched and
// `start`'s window still valid at its old location/size, so the caller can
// still report a clean overflow rather than being left with a corrupted
// window.
func (d *registerDirectory) move(oldStart registerMark, oldSize, newSize int) (window []Value, newStart registerMark, ok bool) {
	// The old window is the current topmost allocation (same precondition as
	// tryExpand): releasing it first lets push reuse its space if possible.
	saved := d.cur
	d.cur = oldStart
	newWindow, start, _, ok := d.push(newSize)
	if !ok {
		d.cur = saved // Restore: the old window is still there, unchanged.
		return nil, registerMark{}, false
	}
	oldWindow := d.blocks[oldStart.block].data[oldStart.offset : oldStart.offset+oldSize]
	copy(newWindow, oldWindow)
	return newWindow, start, true
}

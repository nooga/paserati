package vm

import (
	"math"
	"testing"
)

// TestAddConstantFloatCacheDistinguishesSignedZeroAndNaN covers the other
// half of the A5 acceptance list: numeric constant dedup must not let host
// map equality stand in for JavaScript constant equivalence. Go's float64
// equality (and so a map keyed directly by float64) treats +0.0 == -0.0 as
// true and NaN == NaN as always false - the former would silently alias a
// -0 constant onto an existing +0 entry (Object.is/1÷x can observe the
// difference), the latter would make every NaN literal uncacheable and so
// needlessly spend pool slots once AddConstant enforces a hard cap.
func TestAddConstantFloatCacheDistinguishesSignedZeroAndNaN(t *testing.T) {
	c := NewChunk()

	posZeroIdx := c.AddConstant(NumberValue(0))
	negZeroIdx := c.AddConstant(NumberValue(math.Copysign(0, -1)))
	if posZeroIdx == negZeroIdx {
		t.Fatalf("+0 and -0 got the same constant index %d; they are observably distinct values (1/+0 vs 1/-0, Object.is)", posZeroIdx)
	}
	// Re-adding +0 must still hit the original entry, not grow the pool.
	if again := c.AddConstant(NumberValue(0)); again != posZeroIdx {
		t.Fatalf("re-adding +0 got index %d, want the original %d", again, posZeroIdx)
	}

	nan1Idx := c.AddConstant(NaN)
	nan2Idx := c.AddConstant(NaN)
	if nan1Idx != nan2Idx {
		t.Fatalf("two identical NaN constants got different indices (%d, %d): NaN literals must dedup like any other bit-identical constant", nan1Idx, nan2Idx)
	}
}

// TestAddConstantCapacityBoundary covers the paserati A5 finding: AddConstant
// used to narrow the pool index to uint16 without checking it fit, so a
// 65,537th distinct constant silently wrapped and returned an existing
// constant's index instead of failing. These tests drive a chunk to exactly
// constantPoolCapacity entries and check the boundary from both sides.
func TestAddConstantCapacityBoundary(t *testing.T) {
	c := NewChunk()

	// Fill the pool with exactly constantPoolCapacity distinct constants.
	// Every index returned along the way must match the insertion order -
	// there is no capacity check to trigger yet.
	for i := 0; i < constantPoolCapacity; i++ {
		idx := c.AddConstant(IntegerValue(int32(i)))
		if int(idx) != i {
			t.Fatalf("AddConstant(%d): got index %d, want %d", i, idx, i)
		}
	}
	if len(c.Constants) != constantPoolCapacity {
		t.Fatalf("pool has %d entries, want exactly %d", len(c.Constants), constantPoolCapacity)
	}

	// A duplicate of an already-present constant must still resolve from
	// cache once the pool is completely full - filling the pool must not
	// break code that only re-references constants it already emitted.
	for _, probe := range []int32{0, 1, int32(constantPoolCapacity - 1)} {
		idx := c.AddConstant(IntegerValue(probe))
		if int(idx) != int(probe) {
			t.Fatalf("duplicate AddConstant(%d) at capacity: got index %d, want %d (cache lookup must not be gated by the capacity check)", probe, idx, probe)
		}
	}
	if len(c.Constants) != constantPoolCapacity {
		t.Fatalf("duplicate lookups grew the pool to %d entries, want unchanged %d", len(c.Constants), constantPoolCapacity)
	}

	// A genuinely new constant at capacity must panic with the typed value,
	// not silently narrow its index into range 0..65535 and return the
	// wrong existing constant (the original A5 bug), and must not corrupt
	// the pool by appending anyway.
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("AddConstant of a new value at capacity should panic instead of silently wrapping")
			}
			p, ok := r.(ConstantPoolExhaustionPanic)
			if !ok {
				t.Fatalf("expected ConstantPoolExhaustionPanic, got %T: %v", r, r)
			}
			if p.Capacity != constantPoolCapacity {
				t.Fatalf("panic reported Capacity=%d, want %d", p.Capacity, constantPoolCapacity)
			}
		}()
		c.AddConstant(IntegerValue(int32(constantPoolCapacity))) // one past every existing entry
	}()
	if len(c.Constants) != constantPoolCapacity {
		t.Fatalf("failed AddConstant left the pool at %d entries, want unchanged %d (must not append then panic)", len(c.Constants), constantPoolCapacity)
	}

	// One below capacity must still succeed normally: the fix must not have
	// shaved a legitimate slot off the top along with the buggy one.
	c2 := NewChunk()
	for i := 0; i < constantPoolCapacity-1; i++ {
		c2.AddConstant(IntegerValue(int32(i)))
	}
	idx := c2.AddConstant(IntegerValue(int32(constantPoolCapacity - 1)))
	if int(idx) != constantPoolCapacity-1 {
		t.Fatalf("the last legitimately representable constant got index %d, want %d", idx, constantPoolCapacity-1)
	}
}

// TestAddConstantCapacityBoundaryStrings covers the same boundary through the
// string fast path specifically (the one the A5 appendix repro exercises),
// since each Value-kind branch in AddConstant has its own capacity check.
func TestAddConstantCapacityBoundaryStrings(t *testing.T) {
	c := NewChunk()
	for i := 0; i < constantPoolCapacity; i++ {
		c.AddConstant(NewString(intToDigits(i)))
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("AddConstant of a new string at capacity should panic instead of silently wrapping")
		}
		if _, ok := r.(ConstantPoolExhaustionPanic); !ok {
			t.Fatalf("expected ConstantPoolExhaustionPanic, got %T: %v", r, r)
		}
	}()
	c.AddConstant(NewString(intToDigits(constantPoolCapacity)))
}

// TestChunkGetColumn covers #153's Columns table directly: it must recover
// the column of the latest entry at or before a given offset (binary search
// over a table built by successive MarkColumn calls, as the compiler makes
// them during codegen - always in non-decreasing offset order, since offset
// is always len(chunk.Code) at the time of the call and Code only grows by
// appending), and return 0 - not some other entry's column, not a panic -
// when offset precedes every entry or the table is empty.
func TestChunkGetColumn(t *testing.T) {
	c := NewChunk()
	if got := c.GetColumn(0); got != 0 {
		t.Errorf("empty table: GetColumn(0) = %d, want 0", got)
	}

	c.MarkColumn(0, 3)
	c.MarkColumn(4, 15)
	c.MarkColumn(10, 1)

	cases := []struct {
		offset int
		want   int
	}{
		{-1, 0},  // before every entry
		{0, 3},   // exactly the first entry
		{1, 3},   // between entry 0 and entry 1
		{3, 3},   // still before entry 1
		{4, 15},  // exactly the second entry
		{9, 15},  // between entry 1 and entry 2
		{10, 1},  // exactly the third entry
		{100, 1}, // past every entry
	}
	for _, tc := range cases {
		if got := c.GetColumn(tc.offset); got != tc.want {
			t.Errorf("GetColumn(%d) = %d, want %d", tc.offset, got, tc.want)
		}
	}

	// A second MarkColumn call at the *current last* offset overwrites that
	// entry in place rather than appending a sibling one - the case the
	// compiler actually hits when the line pointer changes twice with no
	// code emitted in between (e.g. two statements compiled back to back
	// with a no-op in between). MarkColumn is only ever called with
	// len(chunk.Code) as the offset, and Code only grows by appending, so
	// offsets across calls are non-decreasing and a repeat can only ever
	// match the most recent entry - never an earlier one, which is why the
	// overwrite check only looks at the table's last entry.
	c.MarkColumn(10, 99)
	if got := c.GetColumn(10); got != 99 {
		t.Errorf("after overwrite: GetColumn(10) = %d, want 99", got)
	}
	if n := len(c.Columns); n != 3 {
		t.Errorf("overwrite at the last offset changed the entry count to %d, want 3", n)
	}
}

func intToDigits(i int) string {
	// Minimal decimal formatter to avoid pulling in strconv just for test data.
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

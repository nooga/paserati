package vm

import "testing"

func TestChunkLinesRunLengthEncoded(t *testing.T) {
	c := NewChunk()
	c.WriteOpCode(OpNop, 3)
	c.EmitByte(1)
	c.WriteUint16(7)
	c.WriteOpCode(OpNop, 3)
	c.WriteOpCode(OpNop, 9)
	c.WriteUint32(1)
	c.WriteOpCode(OpNop, 4)
	want := []int{3, 3, 3, 3, 3, 9, 9, 9, 9, 9, 4}
	if c.LineBytes() != len(want) || len(c.Code) != len(want) {
		t.Fatalf("LineBytes=%d code=%d, want %d", c.LineBytes(), len(c.Code), len(want))
	}
	for i, w := range want {
		if got := c.GetLine(i); got != w {
			t.Errorf("GetLine(%d)=%d, want %d", i, got, w)
		}
	}
	if c.GetLine(-1) != 0 || c.GetLine(len(want)) != 0 {
		t.Error("out-of-range offsets must report line 0")
	}
	if len(c.lineRuns) != 3 {
		t.Errorf("%d runs, want 3", len(c.lineRuns))
	}
	c.Compact()
	if cap(c.Code) != len(c.Code) || c.GetLine(5) != 9 {
		t.Error("Compact changed the chunk")
	}
}

package driver

import (
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// TestRecursionDepthLimitIsFrameCountNotRegisterSpace pins down which of the
// VM's two independent overflow guards (pkg/vm/call.go's "frame limit" check
// on vm.frameCount, and its separate "register stack space" check on
// vm.nextRegSlot) is what actually bounds ordinary deep recursion today.
//
// docs/runtime-production-roadmap.md#b4 wants the eager `RegFileSize *
// MaxFrames` register reservation replaced with on-demand segments. Once
// registers grow on demand, the register-space check stops being able to
// fire at all, and vm.frameCount becomes the *only* thing bounding
// recursion depth - so the "Maximum call stack size exceeded" RangeError a
// JS program can already catch today must, even now, actually be coming
// from the frame-count check for ordinary small-register recursion, not
// incidentally from register exhaustion (which - because the register file
// is sized as exactly RegFileSize*MaxFrames - would only ever bind at the
// same depth or later for any single call needing at most RegFileSize
// registers, never sooner). If that assumption is wrong, or something
// changes it later, B4 would silently change recursion-depth semantics
// instead of only removing a redundant backstop.
//
// This shrinks MaxFrames to a small number via vm.SetMaxFrames so the test
// is fast and the expected depth is unambiguous, then counts exactly how
// many recursive calls happened before the overflow was caught. A
// small-register recursive function run against a VM whose frames array
// was sized for `wantMaxFrames` must overflow at (very close to) that
// depth - many orders of magnitude short of what register exhaustion alone
// would allow (RegFileSize*wantMaxFrames calls' worth of headroom for a
// handful of registers per frame) - which is only possible if frameCount,
// not register space, is the guard that actually fired.
//
// NOTE: this deliberately does NOT assert `e instanceof RangeError`. Today
// ordinary function-call recursion overflow (unlike the OpNew-family
// constructor-call path) throws a generic Error with a non-spec message
// ("Stack overflow") instead of a catchable RangeError("Maximum call stack
// size exceeded") - see the issue filed for that gap. This test only pins
// the frame-count-is-the-guard property that B4 must preserve; it is not
// about the exception's type or message.
func TestRecursionDepthLimitIsFrameCountNotRegisterSpace(t *testing.T) {
	origMaxFrames := vm.MaxFrames
	defer vm.SetMaxFrames(origMaxFrames)

	const wantMaxFrames = 80 // vm.SetMaxFrames clamps below 64, so this stays above that floor
	vm.SetMaxFrames(wantMaxFrames)

	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	src := `
let depth = 0;
function recurse() {
  depth++;
  recurse();
}
let caught = false;
try {
  recurse();
} catch (e) {
  caught = true;
}
JSON.stringify({ caught: caught, depth: depth });
`
	result, errs := p.RunString(src)
	if len(errs) > 0 {
		t.Fatalf("RunString failed: %v", errs[0])
	}

	out := result.ToString()
	if !strings.Contains(out, `"caught":true`) {
		t.Fatalf("expected the overflow to be caught, got: %s", out)
	}

	// Extract the depth value with a tiny manual scan rather than pulling in
	// an encoding/json dependency for one integer.
	const marker = `"depth":`
	_, depthStr, found := strings.Cut(out, marker)
	if !found {
		t.Fatalf("could not find depth in result: %s", out)
	}
	if end := strings.IndexAny(depthStr, "}, "); end >= 0 {
		depthStr = depthStr[:end]
	}

	// The bound to check against: how deep register exhaustion ALONE could
	// have gone for this tiny-register-footprint function, if frameCount
	// were not the actual guard. recurse() only ever needs a couple of
	// registers, so this is generous on purpose - any depth anywhere near
	// this bound (as opposed to anywhere near wantMaxFrames) would mean the
	// frame-count check was not what fired.
	registerExhaustionFloor := vm.RegFileSize * wantMaxFrames / 4 // /4: still far short of the true register-only bound, comfortably above any plausible frame-count-driven depth

	depth := parseSmallInt(depthStr)
	if depth <= 0 {
		t.Fatalf("expected recurse() to run at least once before overflowing, got depth=%d", depth)
	}
	if depth >= registerExhaustionFloor {
		t.Fatalf("recursion reached depth=%d before overflowing (MaxFrames=%d) - "+
			"that's deep enough that register exhaustion, not the frame-count check, "+
			"looks like what actually fired; expected depth close to MaxFrames",
			depth, wantMaxFrames)
	}
}

// parseSmallInt is a tiny, dependency-free decimal parser sufficient for the
// small non-negative integers this test extracts from a JSON fragment.
func parseSmallInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

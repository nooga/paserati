package driver

import (
	"strings"
	"testing"
)

// TestUncaughtToPrimitiveErrorDoesNotPanic covers #156: an *uncaught* TypeError
// raised by ToPrimitive from inside a native call panicked the VM with
// "index out of range [-2]".
//
// handleUncaughtException reports the exception and zeroes frameCount to stop
// execution, but the interpreter's post-unwind paths did `continue` - which does
// not exit the dispatch loop, it re-enters the body with frame/code/ip still
// cached from the frame that was just discarded. The loop then decoded the dead
// frame's bytecode and ran its OpReturn, taking frameCount to -1, and the next
// read of vm.frames[frameCount-1] panicked.
//
// This has to be a Go test rather than a tests/scripts case: the bug only
// appears when the exception goes *uncaught* to the top, and the script harness
// can only assert on a value or a matched error, not on "the VM did not crash on
// the way to reporting this".
func TestUncaughtToPrimitiveErrorDoesNotPanic(t *testing.T) {
	// An object ToPrimitive cannot convert: neither method is callable, so
	// OrdinaryToPrimitive's final step throws via vm.ThrowTypeError - the native
	// then returns normally with the VM unwinding, which is the path that broke.
	const unconvertible = `const o = { valueOf: null, toString: null };`

	cases := map[string]string{
		"call":            `String(o);`,
		"construct":       `new String(o);`,
		"constructNumber": `new Number(o);`,
		"constructDate":   `new Date(o);`,
		// Getter form: the methods exist as accessors returning undefined.
		"accessors": `const g = { get valueOf() { return undefined; }, get toString() { return undefined; } }; String(g);`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewPaserati()
			p.SetSkipTypeCheck(true)
			_, errs := p.RunCode(unconvertible+"\n"+body, RunOptions{})

			if len(errs) == 0 {
				t.Fatal("expected an uncaught-exception diagnostic, got none")
			}

			var reported []string
			for _, e := range errs {
				reported = append(reported, e.Error())
			}
			all := strings.Join(reported, "\n")

			// The panic surfaced as an extra "Internal VM Error" diagnostic,
			// and (because runtimeError itself then panicked on the negative
			// frameCount) buried the real one.
			if strings.Contains(all, "Internal VM Error") {
				t.Errorf("VM panicked while reporting an uncaught exception:\n%s", all)
			}
			if !strings.Contains(all, "Cannot convert object to primitive value") {
				t.Errorf("real TypeError diagnostic missing:\n%s", all)
			}
		})
	}
}

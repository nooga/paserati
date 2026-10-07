package driver

import (
	"strings"
	"testing"
	"time"
)

// CancelVM must take effect inside native builtins that loop over a large
// input, not only at the next bytecode instruction (#621).
func TestCancelInsideLongBuiltins(t *testing.T) {
	cases := []string{
		`new Array(3e7).fill(1).length`,
		`new Uint8Array(1e8).fill(7).length`,
		`Array.from({length: 2e7}).length`,
		`JSON.parse("[" + "1,".repeat(2e7) + "1]").length`,
		`"ab".repeat(2e7).split("").length`,
		`try { new Array(3e7).fill(1) } catch (e) { while (true) {} }`,
	}
	for _, src := range cases {
		p := NewPaserati()
		p.SetSkipTypeCheck(true)
		go func() {
			time.Sleep(50 * time.Millisecond)
			p.CancelVM()
		}()
		start := time.Now()
		_, errs := p.RunCode(src, RunOptions{Script: true})
		elapsed := time.Since(start)
		if len(errs) == 0 {
			t.Errorf("%s: finished without being cancelled", src)
			continue
		}
		if msg := errs[0].Error(); !strings.Contains(msg, "VM execution cancelled") || strings.Contains(msg, "Uncaught") {
			t.Errorf("%s: unexpected error %q", src, msg)
		}
		if elapsed > time.Second {
			t.Errorf("%s: cancellation took %v", src, elapsed)
		}
	}
}

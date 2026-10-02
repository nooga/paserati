package driver

import (
	"reflect"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// runCollectingRejections runs source with an unhandled-rejection hook
// installed and returns the displayed reasons it was called with (#120).
func runCollectingRejections(t *testing.T, source string, install bool) []string {
	t.Helper()
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	var got []string
	if install {
		machine := p.GetVM()
		machine.SetUnhandledRejectionHandler(func(reason vm.Value, _ vm.Value) {
			got = append(got, machine.FormatUnhandledRejection(reason))
		})
	}
	if _, errs := p.RunCode(source, RunOptions{}); len(errs) > 0 {
		t.Fatalf("run failed: %v", errs[0])
	}
	return got
}

func TestUnhandledRejectionReported(t *testing.T) {
	cases := []struct {
		name, source string
		want         []string
	}{
		{"async function call", "async function f() { throw new Error('x'); }\nf();", []string{"Uncaught (in promise): Error: x"}},
		{"Promise.reject", "Promise.reject(new TypeError('t'));", []string{"Uncaught (in promise): TypeError: t"}},
		{"derived promise only", "Promise.reject(1).then(v => v).catch(() => {}); Promise.resolve().then(() => { throw new RangeError('r'); }).then(v => v);", []string{"Uncaught (in promise): RangeError: r"}},
		{"non-error reason", "Promise.reject(7);", []string{"Uncaught (in promise): 7"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runCollectingRejections(t, tc.source, true)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d reports %q, want %q", len(got), got, tc.want)
			}
			for i := range got {
				// Only the first line: the rest is a stack trace.
				first := got[i]
				for j := 0; j < len(first); j++ {
					if first[j] == '\n' {
						first = first[:j]
						break
					}
				}
				if first != tc.want[i] {
					t.Errorf("report %d = %q, want %q", i, first, tc.want[i])
				}
			}
		})
	}
}

func TestHandledRejectionNotReported(t *testing.T) {
	source := `
async function f() { throw new Error('late'); }
const p = f();
p.catch(() => {});
const q = Promise.reject(new Error('q'));
Promise.resolve().then(() => q.catch(() => {}));
(async () => { try { await Promise.reject(new Error('a')); } catch (e) {} })();
Promise.all([Promise.reject(new Error('all'))]).catch(() => {});
Promise.race([Promise.reject(new Error('race')), new Promise(() => {})]).catch(() => {});
Promise.allSettled([Promise.reject(1)]);
new Promise((_, rej) => rej(1)).then(null, () => {});
`
	if got := runCollectingRejections(t, source, true); len(got) != 0 {
		t.Errorf("handled rejections were reported: %q", got)
	}
}

// Without a hook nothing is tracked or reported: embedders and the test262
// runner see no behaviour change.
func TestUnhandledRejectionOptIn(t *testing.T) {
	got := runCollectingRejections(t, "Promise.reject(new Error('x'));", false)
	if !reflect.DeepEqual(got, []string(nil)) {
		t.Errorf("reports without a hook: %q", got)
	}
}

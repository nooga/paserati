package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/test262"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// faultInitializer injects failures the runner must never mistake for a
// JavaScript outcome: a Go panic inside the VM, and native code that
// ignores cancellation.
type faultInitializer struct{}

func (faultInitializer) Name() string  { return "Test262RunnerFaults" }
func (faultInitializer) Priority() int { return 1001 }
func (faultInitializer) InitTypes(ctx *builtins.TypeContext) error {
	if err := ctx.DefineGlobal("$paseratiPanic", types.Any); err != nil {
		return err
	}
	return ctx.DefineGlobal("$paseratiBlock", types.Any)
}
func (faultInitializer) InitRuntime(ctx *builtins.RuntimeContext) error {
	if err := ctx.DefineGlobal("$paseratiPanic", vm.NewNativeFunction(0, false, "$paseratiPanic", func([]vm.Value) (vm.Value, error) {
		panic("injected runner-test panic")
	})); err != nil {
		return err
	}
	return ctx.DefineGlobal("$paseratiBlock", vm.NewNativeFunction(0, false, "$paseratiBlock", func([]vm.Value) (vm.Value, error) {
		time.Sleep(600 * time.Millisecond)
		return vm.Undefined, nil
	}))
}

func fixtureOptions(t *testing.T) runOptions {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "corpus"))
	if err != nil {
		t.Fatal(err)
	}
	return runOptions{
		testRoot:          root,
		testDir:           filepath.Join(root, "test"),
		timeout:           2 * time.Second,
		stopGrace:         2 * time.Second,
		extraInitializers: func() []builtins.BuiltinInitializer { return []builtins.BuiltinInitializer{faultInitializer{}} },
	}
}

type variantWant struct {
	variant string
	status  test262.Status
}

// TestRunnerFixtures pins the verdict of every variant of every fixture.
// The first six fixtures are the ones the September 2026 audit found
// reported as passing; each has a control that must pass, so a runner that
// rejects everything cannot satisfy this test.
func TestRunnerFixtures(t *testing.T) {
	opts := fixtureOptions(t)
	pass, fail := test262.StatusPass, test262.StatusFail
	cases := []struct {
		file    string
		want    []variantWant
		file262 test262.Status
		diag    string // substring of the first non-passing diagnostic
	}{
		// Audit fixtures.
		{"negative-runtime-completes.js", []variantWant{{"non-strict", fail}, {"strict", fail}}, fail, "completed normally"},
		{"negative-runtime-wrong-type.js", []variantWant{{"non-strict", fail}, {"strict", fail}}, fail, "got RangeError"},
		{"negative-parse-wrong-phase.js", []variantWant{{"non-strict", fail}, {"strict", fail}}, fail, "got runtime SyntaxError"},
		{"async-no-done.js", []variantWant{{"non-strict", fail}, {"strict", fail}}, fail, "did not call $DONE"},
		{"async-done-failure.js", []variantWant{{"non-strict", fail}, {"strict", fail}}, fail, "async failure Error: FAIL"},
		{"strict-variant-fails.js", []variantWant{{"non-strict", pass}, {"strict", fail}}, fail, "Test262Error"},
		// Controls.
		{"negative-runtime-control.js", []variantWant{{"non-strict", pass}, {"strict", pass}}, pass, ""},
		{"negative-parse-control.js", []variantWant{{"non-strict", pass}, {"strict", pass}}, pass, ""},
		{"negative-parse-inline.js", []variantWant{{"non-strict", pass}, {"strict", pass}}, pass, ""},
		{"async-control.js", []variantWant{{"non-strict", pass}, {"strict", pass}}, pass, ""},
		{"strict-variant-control.js", []variantWant{{"non-strict", pass}, {"strict", pass}}, pass, ""},
		{"positive-control.js", []variantWant{{"non-strict", pass}, {"strict", pass}}, pass, ""},
		// Metadata handling.
		{"only-strict.js", []variantWant{{"strict", pass}}, pass, ""},
		{"no-strict.js", []variantWant{{"non-strict", pass}}, pass, ""},
		{"raw.js", []variantWant{{"raw", pass}}, pass, ""},
		{"includes-block-list.js", []variantWant{{"non-strict", pass}, {"strict", pass}}, pass, ""},
		// Async failure wins over an earlier completion.
		{"async-failure-after-complete.js", []variantWant{{"non-strict", fail}, {"strict", fail}}, fail, "late failure"},
		// Internal failures never pass, negative or not.
		{"negative-runtime-panic.js", []variantWant{{"non-strict", fail}, {"strict", fail}}, fail, "internal error"},
		{"panic.js", []variantWant{{"non-strict", fail}}, fail, "internal error"},
		// Infrastructure errors are their own status.
		{"missing-include.js", []variantWant{{"non-strict", test262.StatusInfra}}, test262.StatusInfra, "does-not-exist.js"},
		{"bad-negative.js", []variantWant{{"", test262.StatusInfra}}, test262.StatusInfra, "invalid phase"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			r := runFile(filepath.Join(opts.testDir, tc.file), opts)
			var got []variantWant
			for _, v := range r.Variants {
				got = append(got, variantWant{v.Variant, v.Status})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("variants = %v, want %v (%+v)", got, tc.want, r.Variants)
			}
			if r.Status != tc.file262 {
				t.Fatalf("file status = %s, want %s", r.Status, tc.file262)
			}
			if tc.diag != "" && !strings.Contains(r.Error, tc.diag) {
				t.Fatalf("diagnostic %q does not mention %q", r.Error, tc.diag)
			}
			for _, v := range r.Variants {
				if tc.want[0].variant != "" && v.Key != tc.file+"#"+v.Variant {
					t.Fatalf("key = %q", v.Key)
				}
			}
		})
	}
}

// TestRunnerTimeoutStopsWorker checks that a timed-out variant is reported
// only after its worker stopped, and that a worker that can't be stopped
// is an infrastructure error rather than an ordinary timeout.
func TestRunnerTimeoutStopsWorker(t *testing.T) {
	opts := fixtureOptions(t)
	opts.timeout = 100 * time.Millisecond
	r := runFile(filepath.Join(opts.testDir, "loop.js"), opts)
	if r.Status != test262.StatusTimeout {
		t.Fatalf("loop.js: status %s (%s), want timeout", r.Status, r.Error)
	}
	r = runFile(filepath.Join(opts.testDir, "job-loop.js"), opts)
	if r.Status != test262.StatusTimeout {
		t.Fatalf("job-loop.js: status %s (%s), want timeout", r.Status, r.Error)
	}
	opts.stopGrace = 50 * time.Millisecond
	r = runFile(filepath.Join(opts.testDir, "stuck-worker.js"), opts)
	if r.Status != test262.StatusInfra || !strings.Contains(r.Error, "did not stop") {
		t.Fatalf("stuck-worker.js: status %s (%s), want infra-error", r.Status, r.Error)
	}
	time.Sleep(700 * time.Millisecond) // let the blocked worker finish
}

// TestAppendedThrowFlipsVerdict is the audit's execution probe: a throw
// appended to a passing test must make it fail, or the body was not run
// to completion.
func TestAppendedThrowFlipsVerdict(t *testing.T) {
	opts := fixtureOptions(t)
	src, err := os.ReadFile(filepath.Join(opts.testDir, "positive-control.js"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	opts.testDir = dir
	probe := filepath.Join(dir, "probe.js")
	if err := os.WriteFile(probe, append(src, []byte("\nthrow new Test262Error(\"appended\");\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runFile(probe, opts)
	if r.Status != test262.StatusFail {
		t.Fatalf("status %s, want fail", r.Status)
	}
	for _, v := range r.Variants {
		if v.Phase != "runtime" || v.ErrorType != "Test262Error" {
			t.Fatalf("%s: observed %s %s, want runtime Test262Error", v.Variant, v.Phase, v.ErrorType)
		}
	}
}

func TestParseMeta(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want testMeta
		err  string
	}{
		{"none", "var x;", testMeta{Flags: map[string]bool{}}, ""},
		{"flow lists", "/*---\nflags: [onlyStrict, async]\nincludes: [a.js, 'b.js']\nfeatures: [Symbol]\n---*/",
			testMeta{Flags: map[string]bool{"onlyStrict": true, "async": true}, Includes: []string{"a.js", "b.js"}, Features: []string{"Symbol"}}, ""},
		{"block lists", "/*---\nincludes:\n  - a.js\n  - b.js\nflags:\n  - module\n---*/",
			testMeta{Flags: map[string]bool{"module": true}, Includes: []string{"a.js", "b.js"}}, ""},
		{"negative block", "/*---\ndescription: |\n  negative: not this one\nnegative:\n    phase: resolution\n    type: SyntaxError\n---*/",
			testMeta{Flags: map[string]bool{}, Negative: &negativeMeta{Phase: "resolution", Type: "SyntaxError"}}, ""},
		{"negative flow", "/*---\nnegative: {phase: runtime, type: TypeError}\n---*/",
			testMeta{Flags: map[string]bool{}, Negative: &negativeMeta{Phase: "runtime", Type: "TypeError"}}, ""},
		{"description mentioning negative is not negative", "/*---\ndescription: >\n  a SyntaxError is expected here\n---*/",
			testMeta{Flags: map[string]bool{}}, ""},
		{"bare CR line endings", "/*---\rincludes: [a.js]\rflags: [noStrict]\r---*/",
			testMeta{Flags: map[string]bool{"noStrict": true}, Includes: []string{"a.js"}}, ""},
		{"bad phase", "/*---\nnegative:\n  phase: early\n  type: SyntaxError\n---*/", testMeta{}, "invalid phase"},
		{"missing type", "/*---\nnegative:\n  phase: parse\n---*/", testMeta{}, "no type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMeta(tc.src)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestVariantsFor(t *testing.T) {
	m := func(flags ...string) testMeta {
		meta := testMeta{Flags: map[string]bool{}}
		for _, f := range flags {
			meta.Flags[f] = true
		}
		return meta
	}
	cases := []struct {
		meta       testMeta
		strictOnly bool
		want       []string
	}{
		{m(), false, []string{"non-strict", "strict"}},
		{m(), true, []string{"strict"}},
		{m("onlyStrict"), false, []string{"strict"}},
		{m("noStrict"), false, []string{"non-strict"}},
		{m("noStrict"), true, nil},
		{m("module"), false, []string{"module"}},
		{m("raw"), false, []string{"raw"}},
		{m("raw", "noStrict"), false, []string{"raw"}},
	}
	for _, tc := range cases {
		if got := variantsFor(tc.meta, tc.strictOnly); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("variantsFor(%v, %v) = %v, want %v", tc.meta.Flags, tc.strictOnly, got, tc.want)
		}
	}
}

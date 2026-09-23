package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/driver"
	errorsPkg "github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/test262"
	"github.com/nooga/paserati/pkg/vm"
)

// runOptions configures how test files are executed.
type runOptions struct {
	testRoot   string // test262 checkout: holds harness/ and test/
	testDir    string // testRoot/test, the base of result keys
	timeout    time.Duration
	strictOnly bool
	verbose    bool
	jsonMode   bool
	// stopGrace is how long a timed-out worker gets to stop after
	// cancellation before the run reports an infrastructure error.
	stopGrace time.Duration
	// extraInitializers adds globals (Go tests inject failure hooks).
	extraInitializers func() []builtins.BuiltinInitializer
}

// outcome is what one execution variant observably did, before the
// negative/async policy in judge turns it into a status.
type outcome struct {
	completed bool   // ran to the end without an uncaught error
	phase     string // parse, resolution or runtime, when !completed
	errType   string // constructor name of the error, when known
	internal  bool   // a runtime or compiler failure (recovered panic), never a JS error
	infra     string // the runner could not set the test up or stop it
	timedOut  bool
	async     asyncResult
	diag      string
}

// asyncResult is what an async test reported through $DONE (which the
// harness routes through print).
type asyncResult struct {
	completed bool
	failure   string // first Test262:AsyncTestFailure message
}

// printSink collects a test's print() output so the async completion
// protocol is read at the print interface, not from the process stdout.
type printSink struct {
	mu    sync.Mutex
	lines []string
	echo  bool
}

func (s *printSink) print(line string) {
	s.mu.Lock()
	s.lines = append(s.lines, line)
	s.mu.Unlock()
	if s.echo {
		fmt.Println(line)
	}
}

func (s *printSink) async() asyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r asyncResult
	for _, l := range s.lines {
		if l == "Test262:AsyncTestComplete" {
			r.completed = true
		} else if msg, ok := strings.CutPrefix(l, "Test262:AsyncTestFailure:"); ok && r.failure == "" {
			r.failure = msg
		}
	}
	return r
}

// runFile executes every variant testFile requires and combines them.
func runFile(testFile string, opts runOptions) test262.Result {
	start := time.Now()
	result := test262.Result{Path: testFile}
	rel, err := filepath.Rel(opts.testDir, testFile)
	if err != nil {
		rel = testFile
	}
	content, err := os.ReadFile(testFile)
	if err != nil {
		result.Variants = []test262.VariantResult{{Key: rel, Status: test262.StatusInfra, Diagnostic: fmt.Sprintf("read test: %v", err)}}
		return finishResult(result, start)
	}
	src := string(content)
	meta, err := parseMeta(src)
	if err != nil {
		result.Variants = []test262.VariantResult{{Key: rel, Status: test262.StatusInfra, Diagnostic: fmt.Sprintf("metadata: %v", err)}}
		return finishResult(result, start)
	}
	for _, variant := range variantsFor(meta, opts.strictOnly) {
		vstart := time.Now()
		out := runVariant(testFile, src, meta, variant, opts)
		vr := judge(meta, out)
		vr.Key = rel + "#" + variant
		vr.Variant = variant
		vr.Duration = time.Since(vstart)
		result.Variants = append(result.Variants, vr)
	}
	return finishResult(result, start)
}

// finishResult derives the file status from its variants: an
// infrastructure error makes the whole file unreliable, then any failure,
// then any timeout; no variants means skipped.
func finishResult(r test262.Result, start time.Time) test262.Result {
	r.Duration = time.Since(start)
	r.Status = test262.StatusSkip
	if len(r.Variants) > 0 {
		r.Status = test262.StatusPass
	}
	rank := map[test262.Status]int{test262.StatusPass: 0, test262.StatusTimeout: 1, test262.StatusFail: 2, test262.StatusInfra: 3}
	for _, v := range r.Variants {
		if rank[v.Status] > rank[r.Status] || r.Status == test262.StatusSkip {
			r.Status = v.Status
		}
		if v.Status != test262.StatusPass && r.Error == "" {
			r.Error = v.Variant + ": " + v.Diagnostic
		}
	}
	r.Passed = r.Status == test262.StatusPass
	r.Failed = r.Status == test262.StatusFail
	r.TimedOut = r.Status == test262.StatusTimeout
	r.Infra = r.Status == test262.StatusInfra
	r.Skipped = r.Status == test262.StatusSkip
	return r
}

// judge applies the interpretation rules to one variant's outcome.
func judge(meta testMeta, out outcome) test262.VariantResult {
	vr := test262.VariantResult{Phase: out.phase, ErrorType: out.errType, Diagnostic: out.diag}
	if out.async.failure != "" {
		vr.Async = "failure"
	} else if out.async.completed {
		vr.Async = "complete"
	}
	fail := func(format string, args ...any) test262.VariantResult {
		vr.Status = test262.StatusFail
		msg := fmt.Sprintf(format, args...)
		if out.diag != "" {
			msg += ": " + out.diag
		}
		vr.Diagnostic = msg
		return vr
	}
	switch {
	case out.infra != "":
		vr.Status = test262.StatusInfra
		vr.Diagnostic = out.infra
		return vr
	case out.timedOut:
		vr.Status = test262.StatusTimeout
		return vr
	case out.internal:
		return fail("internal error")
	}
	if neg := meta.Negative; neg != nil {
		switch {
		case out.completed:
			return fail("expected %s %s, completed normally", neg.Phase, neg.Type)
		case out.phase != neg.Phase:
			return fail("expected %s %s, got %s %s", neg.Phase, neg.Type, out.phase, orUnknown(out.errType))
		case out.errType != neg.Type:
			return fail("expected %s %s, got %s", neg.Phase, neg.Type, orUnknown(out.errType))
		}
		vr.Status = test262.StatusPass
		return vr
	}
	if !out.completed {
		return fail("%s error %s", out.phase, orUnknown(out.errType))
	}
	if meta.flag("async") {
		switch {
		case out.async.failure != "":
			return fail("async failure %s", out.async.failure)
		case !out.async.completed:
			return fail("async test did not call $DONE")
		}
	}
	vr.Status = test262.StatusPass
	return vr
}

func orUnknown(s string) string {
	if s == "" {
		return "(unknown type)"
	}
	return s
}

// runVariant executes one variant in a fresh runtime on a worker goroutine,
// bounded by opts.timeout. A timed-out worker is cancelled and must stop
// within opts.stopGrace, so it cannot keep consuming CPU during later
// tests; one that doesn't is reported as an infrastructure error.
func runVariant(testFile, src string, meta testMeta, variant string, opts runOptions) outcome {
	sink := &printSink{echo: opts.verbose && !opts.jsonMode}
	pas := newTest262Paserati(testFile, sink, opts)
	done := make(chan outcome, 1)
	go func() {
		var out outcome
		defer func() {
			if r := recover(); r != nil {
				if opts.verbose {
					debug.PrintStack()
				}
				out = outcome{internal: true, diag: fmt.Sprintf("runner goroutine panicked: %v", r)}
			}
			// Cleanup runs only here: the timeout path must not tear down a
			// runtime this goroutine may still be using.
			pas.Cleanup()
			done <- out
		}()
		out = executeVariant(pas, sink, testFile, src, meta, variant, opts)
	}()

	timer := time.NewTimer(opts.timeout)
	defer timer.Stop()
	select {
	case out := <-done:
		return out
	case <-timer.C:
	}
	pas.CancelVM()
	grace := opts.stopGrace
	if grace == 0 {
		grace = 2 * time.Second
	}
	select {
	case <-done:
		return outcome{timedOut: true, diag: fmt.Sprintf("timed out after %v", opts.timeout)}
	case <-time.After(grace):
		return outcome{timedOut: true, infra: fmt.Sprintf("timed out after %v and the worker did not stop within %v of cancellation", opts.timeout, grace)}
	}
}

// executeVariant builds the variant's source and runs it through parse,
// compile and execution, recording the stage of the first error.
func executeVariant(pas *driver.Paserati, sink *printSink, testFile, src string, meta testMeta, variant string, opts runOptions) outcome {
	var harness strings.Builder
	if variant == variantStrict {
		harness.WriteString("\"use strict\";\n")
	}
	if variant != variantRaw {
		includes := []string{"sta.js", "assert.js"}
		if meta.flag("async") {
			includes = append(includes, "doneprintHandle.js")
		}
		includes = append(includes, meta.Includes...)
		for _, inc := range includes {
			b, err := os.ReadFile(filepath.Join(opts.testRoot, "harness", inc))
			if err != nil {
				return outcome{infra: fmt.Sprintf("read include %s: %v", inc, err)}
			}
			harness.WriteString("\n// [included] " + inc + "\n")
			harness.Write(b)
			harness.WriteString("\n")
		}
	}

	var out outcome
	if variant == variantModule {
		// The harness runs as a script; the test body is the module.
		if h := harness.String(); h != "" {
			if o, ok := runStage(pas, h, ""); !ok {
				o.infra = "harness failed: " + o.diag
				return o
			}
		}
		pas.EnableModuleMode(testFile)
		out, _ = runStage(pas, src, variant)
	} else {
		full := src
		if variant != variantRaw {
			full = harness.String() + "\n// [test body]\n" + src
		}
		out, _ = runStage(pas, full, variant)
	}
	out.async = sink.async()
	if !out.completed && !opts.jsonMode && opts.verbose {
		fmt.Printf("  %s [%s]: %s\n", testFile, variant, out.diag)
	}
	return out
}

// runStage parses, compiles and runs source, reporting where it stopped.
func runStage(pas *driver.Paserati, source, variant string) (outcome, bool) {
	prog, parseErrs := parser.NewParser(lexer.NewLexer(source)).ParseProgram()
	if len(parseErrs) > 0 {
		return outcome{phase: "parse", errType: "SyntaxError", diag: parseErrs[0].Error()}, false
	}
	chunk, compileErrs := pas.CompileProgram(prog)
	if len(compileErrs) > 0 {
		return classifyCompileError(compileErrs[0], variant), false
	}
	pas.SyncGlobalNamesFromCompiler()
	_, runtimeErrs := pas.InterpretChunk(chunk)
	if len(runtimeErrs) > 0 {
		return classifyRuntimeError(pas.GetVM(), runtimeErrs[0]), false
	}
	// Run the job queue to completion, as a host does after a script: an
	// async test reports through $DONE from a promise job.
	pas.GetVM().DrainUntilIdle()
	return outcome{completed: true}, true
}

// compileLimitMarkers identify compile errors that are implementation
// limits or compiler bugs rather than ECMAScript early errors. Compile
// errors carry no structured kind yet, so this list is the one place the
// runner reads error text; everything else the compiler reports is an
// early error, which ECMAScript defines as a SyntaxError.
var compileLimitMarkers = []string{"internal error", "internal compiler error", "compiler internal", "compiler panic", "register exhaustion", "too many", "capacity"}

func classifyCompileError(err errorsPkg.PaseratiError, variant string) outcome {
	msg := err.Error()
	lower := strings.ToLower(msg)
	for _, m := range compileLimitMarkers {
		if strings.Contains(lower, m) {
			return outcome{internal: true, diag: msg}
		}
	}
	phase := "parse"
	if ce, ok := err.(*errorsPkg.CompileError); ok && ce.Resolution {
		phase = "resolution"
	}
	return outcome{phase: phase, errType: "SyntaxError", diag: msg}
}

func classifyRuntimeError(v *vm.VM, err errorsPkg.PaseratiError) outcome {
	out := outcome{phase: "runtime", diag: err.Error()}
	re, ok := err.(*errorsPkg.RuntimeError)
	if !ok {
		return out
	}
	if re.Internal {
		out.internal = true
		return out
	}
	if re.Resolution {
		out.phase, out.errType = "resolution", "SyntaxError"
		return out
	}
	if thrown, ok := re.Thrown.(vm.Value); ok {
		out.errType = errorTypeName(v, thrown)
	}
	return out
}

// errorTypeName is the name of the thrown value's constructor, which is
// what negative.type names (Test262Error has no own "name").
func errorTypeName(v *vm.VM, thrown vm.Value) string {
	if v == nil || !thrown.IsObject() {
		return ""
	}
	ctor, err := v.GetProperty(thrown, "constructor")
	if err != nil || !ctor.IsCallable() {
		return ""
	}
	name, err := v.GetProperty(ctor, "name")
	if err != nil {
		return ""
	}
	return name.ToString()
}

// newTest262Paserati creates a runtime with the Test262 globals, resolving
// modules relative to the test file.
func newTest262Paserati(testFile string, sink *printSink, opts runOptions) *driver.Paserati {
	inits := append(builtins.GetStandardInitializers(), &Test262Initializer{sink: sink})
	if opts.extraInitializers != nil {
		inits = append(inits, opts.extraInitializers()...)
	}
	pas := driver.NewPaseratiWithInitializersAndBaseDir(inits, filepath.Dir(testFile))
	pas.SetSkipTypeCheck(true)
	return pas
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/driver"
	errorsPkg "github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
)

// workerReq is one job sent to a worker process.
type workerReq struct {
	Rel    string            `json:"rel"`              // path relative to the conformance dir
	Params map[string]string `json:"params,omitempty"` // the variant
}

// workerResp is what a worker reports back. Scoring happens in the parent.
type workerResp struct {
	Actual      []Diag `json:"actual"`
	ParseErrors int    `json:"parseErrors"`
	// CheckRan is true when the checker ran. After parse errors it is false only
	// if the checker panicked on the partial AST (the "parse-only" category).
	CheckRan   bool   `json:"checkRan"`
	CheckPanic string `json:"checkPanic,omitempty"`
	Panic      string `json:"panic,omitempty"` // panic outside the parse-error recovery path: the test is a failure
	Err        string `json:"err,omitempty"`   // harness-level error (unreadable file, ...)
	Ready      bool   `json:"ready,omitempty"` // handshake sent once by a freshly started worker, before any job
	DurNs      int64  `json:"durNs"`
}

// toDiag converts one of our diagnostics. Columns: Paserati's Position.Column is
// a 1-based rune index within the line; TypeScript's are 1-based UTF-16 code
// units. They coincide except for astral-plane characters (emoji and friends)
// earlier on the same line, which are rare enough in the conformance corpus
// that we do not convert (the full-exact tier is informational anyway).
func toDiag(e errorsPkg.PaseratiError, phase string) Diag {
	code := errorsPkg.TSCode(e)
	if code == "" {
		code = e.Code()
	}
	p := e.Pos()
	return Diag{Line: p.Line, Col: p.Column, Code: code, Msg: e.Message(), Phase: phase}
}

// runJob executes one (test, variant) in this process. It must be called from a
// worker (or a test): a stack overflow here is fatal to the process.
func runJob(conformanceDir string, req workerReq) (resp workerResp) {
	start := time.Now()
	defer func() { resp.DurNs = time.Since(start).Nanoseconds() }()

	content, err := os.ReadFile(filepath.Join(conformanceDir, req.Rel))
	if err != nil {
		resp.Err = fmt.Sprintf("read error: %v", err)
		return
	}
	source := strings.TrimPrefix(string(content), "\xEF\xBB\xBF")
	st := stripDirectives(source)
	opts := effectiveOptions(st.Options, Variant(req.Params))

	pas := createTscPaserati(opts)
	defer pas.Cleanup()

	func() {
		defer func() {
			if r := recover(); r != nil {
				resp.Panic = fmt.Sprintf("panic: %v", r)
			}
		}()
		lx := lexer.NewLexer(st.Source)
		p := parser.NewParser(lx)
		prog, parseErrs := p.ParseProgram()
		for _, e := range parseErrs {
			resp.Actual = append(resp.Actual, toDiag(e, "parse"))
		}
		resp.ParseErrors = len(parseErrs)
		if prog == nil {
			return
		}
		pas.SetIsModule(programIsModule(prog))

		// tsc type-checks files with syntax errors and reports both kinds, so we
		// run the checker on the (error-recovered) AST as well. If the checker
		// cannot cope with a partial AST we keep just the parse errors and say so.
		var checkErrs []errorsPkg.PaseratiError
		checkOK := func() (ok bool) {
			defer func() {
				if r := recover(); r != nil {
					if len(parseErrs) == 0 {
						panic(r) // a plain checker panic is a test failure, not a category
					}
					resp.CheckPanic = fmt.Sprintf("%v", r)
					ok = false
				}
			}()
			_, checkErrs = pas.CompileProgram(prog)
			return true
		}()
		resp.CheckRan = checkOK
		for _, e := range checkErrs {
			phase := "check"
			if e.Kind() == "Compile" {
				// Bytecode generation only runs when the checker found nothing. On a
				// recovered AST (parse errors) it chokes on nil nodes and reports
				// "compilation not implemented for <nil>"; that is noise, not a
				// diagnostic tsc would give, so it is dropped there.
				if len(parseErrs) > 0 {
					continue
				}
				phase = "compile"
			}
			resp.Actual = append(resp.Actual, toDiag(e, phase))
		}
	}()
	return
}

func programIsModule(prog *parser.Program) bool {
	for _, s := range prog.Statements {
		switch s.(type) {
		case *parser.ImportDeclaration, *parser.ExportNamedDeclaration,
			*parser.ExportDefaultDeclaration, *parser.ExportAllDeclaration:
			return true
		}
	}
	return false
}

// createTscPaserati creates a Paserati instance configured for one test variant.
// Every setter the driver exposes for TypeScript compiler options is wired here;
// directives with no setter are reported through unsupportedOptionsFor.
func createTscPaserati(opts Options) *driver.Paserati {
	initializers := builtins.GetStandardInitializers()
	pas := driver.NewPaseratiWithInitializers(initializers)
	pas.SetAllowTopLevelReturn(false)
	// Type checking is ON - that's what we're testing.
	strictNull := strictNullChecksEnabled(opts)
	pas.SetStrictNullChecks(strictNull)
	pas.SetSkipDefiniteAssignment(!strictNull)
	pas.SetStrictFunctionTypes(strictFunctionTypesEnabled(opts))
	pas.SetSkipStrictPropertyInit(!strictPropertyInitEnabled(opts))
	pas.SetNoImplicitOverride(noImplicitOverrideEnabled(opts))
	pas.SetNoImplicitAny(opts.strictFamily("noimplicitany"))
	pas.SetAllowUnreachableCode(allowUnreachableCodeEnabled(opts))
	pas.SetTscCompatibleDiagnostics(true)
	pas.SetAlwaysStrict(alwaysStrictEnabled(opts))
	return pas
}

// strictPropertyInitEnabled gates TS2564. An explicit
// `// @strictPropertyInitialization` directive wins, then `// @strict`; the
// TS 6.0 default is on. TypeScript only reports TS2564 when strictNullChecks is
// also on (it is an error, TS5052, to ask for one without the other).
func strictPropertyInitEnabled(o Options) bool {
	return o.strictFamily("strictpropertyinitialization") && strictNullChecksEnabled(o)
}

// strictNullChecksEnabled gates TS2454 and TS18050. Explicit directive, then `strict`, default on.
func strictNullChecksEnabled(o Options) bool { return o.strictFamily("strictnullchecks") }

func noImplicitOverrideEnabled(o Options) bool {
	v, _ := o.boolOpt("noimplicitoverride")
	return v
}

// allowUnreachableCodeEnabled gates TS2695. Off unless a directive sets it.
func allowUnreachableCodeEnabled(o Options) bool {
	v, _ := o.boolOpt("allowunreachablecode")
	return v
}

// strictFunctionTypesEnabled gates contravariant parameter comparison.
// Explicit directive, then `strict`, default on.
func strictFunctionTypesEnabled(o Options) bool { return o.strictFamily("strictfunctiontypes") }

// alwaysStrictEnabled gates TS1212. Explicit directive, then `strict`, default on.
func alwaysStrictEnabled(o Options) bool { return o.strictFamily("alwaysstrict") }

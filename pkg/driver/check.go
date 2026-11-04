package driver

import (
	"fmt"
	"github.com/nooga/paserati/pkg/builtins"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/checker"
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/source"
)

// FileDiagnostics is the type-checking result of one file.
type FileDiagnostics struct {
	Path        string
	Diagnostics []errors.PaseratiError
	// LoadError is set when the file could not be loaded or parsed.
	LoadError error
}

// CheckOptions are the compiler options a check honors (tsc's names). The
// zero value of each field is tsc's default when that option is not set,
// except the strict family, which DefaultCheckOptions turns on.
type CheckOptions struct {
	StrictNullChecks             bool
	StrictFunctionTypes          bool
	StrictPropertyInitialization bool
	StrictBindCallApply          bool
	NoImplicitAny                bool
	NoImplicitThis               bool
	UseUnknownInCatchVariables   bool
	NoImplicitOverride           bool
	AlwaysStrict                 bool
	AllowUnreachableCode         bool
	ExperimentalDecorators       bool
	// Target is --target as a year (3 and 5 for ES3 and ES5, 2015 for ES6,
	// 9999 for ESNext); 0 is tsc's default, ES2025.
	Target int
	// Module is --module, lower case ("" when unset).
	Module string
	// UseDefineForClassFields is --useDefineForClassFields; nil is tsc's
	// default, on for a target of ES2022 or later.
	UseDefineForClassFields *bool
	// SkipLibCheck skips validating library files' declarations and
	// reporting their errors (tsc's skipLibCheck); their syntax errors are
	// still reported.
	SkipLibCheck bool
	// Types are the type packages whose global declarations every file
	// sees (tsc's "types": "node" for @types/node, "vitest/globals"),
	// loaded before the files. They resolve from TypesFrom (a directory).
	Types     []string
	TypesFrom string

	// Initializers are the builtins every file sees (default: the
	// standard ones).
	Initializers []builtins.BuiltinInitializer
	// Configure, when set, configures each file's checker after the
	// options above (for options without a field here, such as target).
	Configure func(*checker.Checker)
	// ConfigureParser, when set, configures each file's parser.
	ConfigureParser func(*parser.Parser)
	// CheckAfterParseErrors type-checks files with syntax errors too, as
	// tsc does, reporting both kinds of diagnostic.
	CheckAfterParseErrors bool
	// ScriptsFirst checks the files that are scripts (no import or export)
	// before the modules, in the order given: tsc binds every file's
	// globals before it checks any, so a module sees the globals of a
	// script listed after it.
	ScriptsFirst bool
}

// DefaultCheckOptions are tsc's defaults (TypeScript 6: the strict family on).
func DefaultCheckOptions() CheckOptions {
	return CheckOptions{
		StrictNullChecks:             true,
		StrictFunctionTypes:          true,
		StrictPropertyInitialization: true,
		StrictBindCallApply:          true,
		NoImplicitAny:                true,
		NoImplicitThis:               true,
		UseUnknownInCatchVariables:   true,
		AlwaysStrict:                 true,
		SkipLibCheck:                 true,
	}
}

// target is the --target year, tsc's default (ES2025) when unset.
func (o CheckOptions) target() int {
	if o.Target == 0 {
		return 2025
	}
	return o.Target
}

// useDefineForClassFields resolves --useDefineForClassFields against its
// target-dependent default.
func (o CheckOptions) useDefineForClassFields() bool {
	if o.UseDefineForClassFields != nil {
		return *o.UseDefineForClassFields
	}
	return o.target() >= 2022
}

// apply configures a checker for these options.
func (o CheckOptions) apply(c *checker.Checker) {
	c.SetStrictNullChecks(o.StrictNullChecks)
	c.SetSkipDefiniteAssignment(!o.StrictNullChecks)
	c.SetStrictFunctionTypes(o.StrictFunctionTypes)
	c.SetSkipStrictPropertyInit(!o.StrictPropertyInitialization || !o.StrictNullChecks)
	c.SetStrictBindCallApply(o.StrictBindCallApply)
	c.SetNoImplicitAny(o.NoImplicitAny)
	c.SetNoImplicitThis(o.NoImplicitThis)
	c.SetUseUnknownInCatchVariables(o.UseUnknownInCatchVariables)
	c.SetNoImplicitOverride(o.NoImplicitOverride)
	c.SetAlwaysStrict(o.AlwaysStrict)
	c.SetAllowUnreachableCode(o.AllowUnreachableCode)
	c.SetExperimentalDecorators(o.ExperimentalDecorators)
	c.SetTarget(o.target())
	c.SetModuleKind(o.Module)
	c.SetLegacyModuleFormat(LegacyModuleFormat(o.Module))
	c.SetTopLevelAwaitDisallowed(TopLevelAwaitDisallowed(o.Module, o.target()))
	c.SetUseDefineForClassFields(o.useDefineForClassFields())
	c.SetEmitStandardClassFields(o.useDefineForClassFields() && o.target() >= 2022)
	c.SetTscCompatibleDiagnostics(true)
	c.SetSkipLibCheck(o.SkipLibCheck)
}

// configureParser sets up a parser for a check under these options: tsc's
// diagnostics and recovery, its strictness, target and decorator flavour.
func (o CheckOptions) configureParser(ps *parser.Parser) {
	ps.SetTscDiagnostics(true)
	ps.SetTarget(o.target())
	ps.SetExperimentalDecorators(o.ExperimentalDecorators)
	if o.AlwaysStrict {
		ps.SetStrictMode(true)
	}
}

// LegacyModuleFormat is tsc's name for a pre-ES2015 --module (given lower
// case), "" for any other.
func LegacyModuleFormat(module string) string {
	switch module {
	case "commonjs":
		return "CommonJS"
	case "amd":
		return "AMD"
	case "umd":
		return "UMD"
	case "system":
		return "System"
	}
	return ""
}

// TopLevelAwaitDisallowed reports whether an explicit --module (lower case)
// and the target rule out top-level await: it needs module es2022, esnext,
// system, node16-20, nodenext or preserve, and target es2017 or later.
func TopLevelAwaitDisallowed(module string, target int) bool {
	switch module {
	case "":
		return false
	case "es2022", "esnext", "system", "node16", "node18", "node20", "nodenext", "preserve":
		return target < 2017
	}
	return true
}

// CheckFiles type-checks files with tsc's default options; see
// CheckFilesWithOptions.
func (p *Paserati) CheckFiles(files []string) []FileDiagnostics {
	return p.CheckFilesWithOptions(files, DefaultCheckOptions())
}

// CheckFilesWithOptions type-checks files and every module they import without
// compiling or running anything (`paserati --check`, tsc --noEmit). It
// returns the results of the files checked, sorted by path, leaving out
// library files as tsc's skipLibCheck does: declaration files and modules
// under node_modules. Bare package imports resolve only if the host has
// registered a resolver for them (pkg/nodemodules in Types mode).
func (p *Paserati) CheckFilesWithOptions(files []string, opts CheckOptions) []FileDiagnostics {
	p.moduleLoader.SetCompilerFactory(nil)
	p.moduleLoader.SetParserOptions(opts.configureParser)
	defer p.moduleLoader.SetParserOptions(nil)
	loader := p.moduleLoader
	globals := checker.NewGlobalDeclarations()
	initializers := opts.Initializers
	if initializers == nil {
		initializers = builtins.GetStandardInitializers()
	}
	builtinEnv := checker.NewBuiltinEnvironment(initializers)
	p.moduleLoader.SetCheckerFactory(func() modules.TypeChecker {
		c := checker.NewCheckerWithBuiltins(builtinEnv)
		c.EnableModuleMode("", loader)
		opts.apply(c)
		if opts.Configure != nil {
			opts.Configure(c)
		}
		c.SetSharedGlobals(globals)
		return c
	})
	p.moduleLoader.SetParseOptions(opts.ConfigureParser, opts.CheckAfterParseErrors)
	defer p.moduleLoader.SetParseOptions(nil, false)
	// A type error in one module must not hide the types it exports from
	// the modules importing it: every module's diagnostics are kept on its
	// record and reported below.
	p.moduleLoader.SetIgnoreTypeErrors(true)
	p.moduleLoader.SetAmbientModules(globals.AmbientExports)
	defer p.moduleLoader.SetAmbientModules(nil)

	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	from := filepath.Join(wd, "<check>")
	// Parse every file the check will reach on all cores ahead of the
	// loader, which still loads and checks them one at a time in order.
	p.moduleLoader.EnableParseAhead()
	var loadErrors []FileDiagnostics
	typesFrom := from
	if opts.TypesFrom != "" {
		typesFrom = filepath.Join(opts.TypesFrom, "<check>")
	}
	type root struct{ spec, abs string }
	var roots []root
	for _, file := range files {
		abs, err := filepath.Abs(file)
		if err != nil {
			loadErrors = append(loadErrors, FileDiagnostics{Path: file, LoadError: err})
			continue
		}
		rel, err := filepath.Rel(wd, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = abs
		} else {
			rel = "./" + filepath.ToSlash(rel)
		}
		roots = append(roots, root{rel, abs})
	}
	for _, spec := range opts.Types {
		p.moduleLoader.ParseAhead(spec, typesFrom)
	}
	for _, r := range roots {
		p.moduleLoader.ParseAhead(r.spec, from)
	}

	// Globals declared by any module (`declare global`) are in place before
	// the modules that use them without importing their declarer are
	// checked, as tsc binds every file's globals first.
	for _, path := range p.moduleLoader.GlobalAugmentingModules() {
		_, _ = p.moduleLoader.LoadModule(path, from)
	}
	for _, spec := range opts.Types {
		if _, err := p.moduleLoader.LoadModule(spec, typesFrom); err != nil {
			loadErrors = append(loadErrors, FileDiagnostics{Path: spec, LoadError: fmt.Errorf("cannot find type definition file for '%s': %w", spec, err)})
		}
	}
	if opts.ScriptsFirst {
		var scripts, rest []root
		for _, r := range roots {
			if isScriptFile(r.abs) {
				scripts = append(scripts, r)
			} else {
				rest = append(rest, r)
			}
		}
		roots = append(scripts, rest...)
	}
	for _, r := range roots {
		if _, err := p.moduleLoader.LoadModule(r.spec, from); err != nil {
			loadErrors = append(loadErrors, FileDiagnostics{Path: r.abs, LoadError: err})
		}
	}

	results := loadErrors
	for _, rec := range p.moduleLoader.Modules() {
		if rec.ResolvedPath == "" {
			continue
		}
		if rec.Error != nil {
			// A file that does not parse is reported, library or not (tsc
			// reports syntax errors under skipLibCheck too).
			res := FileDiagnostics{Path: rec.ResolvedPath, Diagnostics: rec.Diagnostics}
			if len(rec.Diagnostics) == 0 {
				res.LoadError = rec.Error
			}
			results = append(results, res)
			continue
		}
		if opts.SkipLibCheck && isLibraryFile(rec.ResolvedPath) {
			continue
		}
		results = append(results, FileDiagnostics{Path: rec.ResolvedPath, Diagnostics: rec.Diagnostics})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Path < results[j].Path })
	for _, res := range results {
		sortByPosition(res.Diagnostics)
	}
	return results
}

// isScriptFile reports whether the file at path parses as a script (no
// import or export).
func isScriptFile(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	src := &source.SourceFile{Name: path, Path: path, Content: string(content)}
	prog, _ := parser.NewParser(lexer.NewLexerWithSource(src)).ParseProgram()
	return prog != nil && !checker.IsModuleProgram(prog)
}

// sortByPosition orders a file's diagnostics by where they start, as tsc's
// sortAndDeduplicateDiagnostics does; diagnostics at the same position keep
// the order they were reported in.
func sortByPosition(diags []errors.PaseratiError) {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := diags[i].Pos(), diags[j].Pos()
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
}

// isLibraryFile reports whether path is a declaration file or lives under
// node_modules, the files tsc's skipLibCheck leaves unchecked.
func isLibraryFile(path string) bool {
	slashed := filepath.ToSlash(path)
	if strings.Contains(slashed, "/node_modules/") {
		return true
	}
	for _, ext := range []string{".d.ts", ".d.mts", ".d.cts"} {
		if strings.HasSuffix(slashed, ext) {
			return true
		}
	}
	return false
}

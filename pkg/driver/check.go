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
	"github.com/nooga/paserati/pkg/modules"
)

// FileDiagnostics is the type-checking result of one file.
type FileDiagnostics struct {
	Path        string
	Diagnostics []errors.PaseratiError
	// LoadError is set when the file could not be loaded or parsed.
	LoadError error
}

// CheckOptions are the compiler options a check honors (tsc's names).
type CheckOptions struct {
	StrictNullChecks             bool
	StrictFunctionTypes          bool
	StrictPropertyInitialization bool
	NoImplicitAny                bool
	NoImplicitOverride           bool
	AlwaysStrict                 bool
	AllowUnreachableCode         bool
	// SkipLibCheck skips validating library files' declarations and
	// reporting their errors (tsc's skipLibCheck); their syntax errors are
	// still reported.
	SkipLibCheck bool
	// Types are the type packages whose global declarations every file
	// sees (tsc's "types": "node" for @types/node, "vitest/globals"),
	// loaded before the files. They resolve from TypesFrom (a directory).
	Types     []string
	TypesFrom string
}

// DefaultCheckOptions are tsc's defaults (TypeScript 6: the strict family on).
func DefaultCheckOptions() CheckOptions {
	return CheckOptions{
		StrictNullChecks:             true,
		StrictFunctionTypes:          true,
		StrictPropertyInitialization: true,
		NoImplicitAny:                true,
		AlwaysStrict:                 true,
		SkipLibCheck:                 true,
	}
}

// apply configures a checker for these options.
func (o CheckOptions) apply(c *checker.Checker) {
	c.SetStrictNullChecks(o.StrictNullChecks)
	c.SetSkipDefiniteAssignment(!o.StrictNullChecks)
	c.SetStrictFunctionTypes(o.StrictFunctionTypes)
	c.SetSkipStrictPropertyInit(!o.StrictPropertyInitialization || !o.StrictNullChecks)
	c.SetNoImplicitAny(o.NoImplicitAny)
	c.SetNoImplicitOverride(o.NoImplicitOverride)
	c.SetAlwaysStrict(o.AlwaysStrict)
	c.SetAllowUnreachableCode(o.AllowUnreachableCode)
	c.SetTscCompatibleDiagnostics(true)
	c.SetSkipLibCheck(o.SkipLibCheck)
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
	loader := p.moduleLoader
	globals := checker.NewGlobalDeclarations()
	builtinEnv := checker.NewBuiltinEnvironment(builtins.GetStandardInitializers())
	p.moduleLoader.SetCheckerFactory(func() modules.TypeChecker {
		c := checker.NewCheckerWithBuiltins(builtinEnv)
		c.EnableModuleMode("", loader)
		opts.apply(c)
		c.SetSharedGlobals(globals)
		return c
	})
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
	return results
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

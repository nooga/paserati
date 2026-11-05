package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/checker"
	"github.com/nooga/paserati/pkg/driver"
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/nodemodules"
	"github.com/nooga/paserati/pkg/parser"
)

// prepareMulti expands a multi-file test into its variants, like
// prepareCases does for a single-file one.
func prepareMulti(rel, base, source string, index *baselineIndex, skipList map[string]bool) []testCase {
	units, hasLinks := splitUnits(source, base)
	settings := parseDirectives(source)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	variants, tooMany := expandVariants(settings)
	keys := unitKeySet(units)
	var cases []testCase
	for _, v := range variants {
		id := rel + v.Suffix()
		if skipList[id] {
			continue
		}
		tc := testCase{ID: id, Rel: rel, Variant: v, Job: -1}
		if len(v) == 0 {
			tc.Variant = nil
		}
		if tooMany {
			tc.Skip = "too-many-variants"
			cases = append(cases, tc)
			continue
		}
		eff := effectiveOptions(settings, v)
		file, unmatched := index.lookup(name, v)
		tc.BaselineFile, tc.Unmatched = file, unmatched
		if reason := multiSkipReason(units, eff, hasLinks); reason != "" {
			tc.Skip = reason
			cases = append(cases, tc)
			continue
		}
		bl, err := index.loadMulti(file, keys)
		if err != nil {
			tc.Skip = "baseline-read-error"
			cases = append(cases, tc)
			continue
		}
		tc.Expected = bl
		tc.Unsupported = unsupportedOptionsFor(eff)
		cases = append(cases, tc)
	}
	return cases
}

// isMultiTest reports whether a test runs through the multi-file path.
func isMultiTest(rel string, st StripResult) bool {
	return st.HasFilename || strings.HasSuffix(strings.ToLower(rel), ".tsx")
}

// tscInitializers are the builtins tsc's lib has: no Paserati namespace or
// global Type interface.
func tscInitializers() []builtins.BuiltinInitializer {
	var out []builtins.BuiltinInitializer
	for _, init := range builtins.GetStandardInitializers() {
		if _, paserati := init.(*builtins.PaseratiInitializer); !paserati {
			out = append(out, init)
		}
	}
	return out
}

// configureTscChecker applies the options createTscPaserati sets on the
// single-file driver that CheckOptions has no field for.
func configureTscChecker(c *checker.Checker, opts Options) {
	c.SetAllowTopLevelReturn(false)
	c.SetStrictBindCallApply(opts.strictFamily("strictbindcallapply"))
	c.SetNoImplicitThis(opts.strictFamily("noimplicitthis"))
	c.SetTopLevelAwaitDisallowed(topLevelAwaitDisallowed(opts))
	c.SetUseDefineForClassFields(useDefineForClassFieldsEnabled(opts))
	c.SetTarget(targetLevel(opts))
	c.SetLegacyModuleFormat(legacyModuleFormat(opts))
	c.SetEmitStandardClassFields(useDefineForClassFieldsEnabled(opts) && targetLevel(opts) >= 2022)
	c.SetModuleKind(strings.ToLower(strings.TrimSpace(opts["module"])))
	experimental, _ := opts.boolOpt("experimentaldecorators")
	c.SetExperimentalDecorators(experimental)
}

// tscCheckOptions are the driver's check options for a test variant.
func tscCheckOptions(opts Options) driver.CheckOptions {
	skipLib, _ := opts.boolOpt("skiplibcheck")
	experimental, _ := opts.boolOpt("experimentaldecorators")
	return driver.CheckOptions{
		StrictNullChecks:             strictNullChecksEnabled(opts),
		StrictFunctionTypes:          strictFunctionTypesEnabled(opts),
		StrictPropertyInitialization: strictPropertyInitEnabled(opts),
		NoImplicitAny:                opts.strictFamily("noimplicitany"),
		NoImplicitOverride:           noImplicitOverrideEnabled(opts),
		AlwaysStrict:                 alwaysStrictEnabled(opts),
		AllowUnreachableCode:         allowUnreachableCodeEnabled(opts),
		SkipLibCheck:                 skipLib,
		Initializers:                 tscInitializers(),
		Configure:                    func(c *checker.Checker) { configureTscChecker(c, opts) },
		ConfigureParser: func(p *parser.Parser) {
			p.SetTscDiagnostics(true)
			p.SetStrictMode(alwaysStrictEnabled(opts))
			p.SetTarget(targetLevel(opts))
			p.SetExperimentalDecorators(experimental)
		},
		CheckAfterParseErrors: true,
		ScriptsFirst:          true,
	}
}

// libDir is TypeScript's tests/lib, which the harness mounts at /.lib.
func libDir(conformanceDir string) string {
	return filepath.Join(conformanceDir, "..", "..", "lib")
}

// runMultiJob type-checks one variant of a multi-file test: its units are
// written to a scratch directory laid out like the harness's virtual file
// system (relative names under /.src, /.lib mounted) and checked from the
// root files the compiler runner would pass.
func runMultiJob(conformanceDir string, req workerReq, source string, resp *workerResp) {
	units, _ := splitUnits(source, filepath.Base(req.Rel))
	settings := parseDirectives(source)
	opts := effectiveOptions(settings, Variant(req.Params))

	root, err := os.MkdirTemp("", "testtsc-")
	if err != nil {
		resp.Err = fmt.Sprintf("scratch dir: %v", err)
		return
	}
	defer os.RemoveAll(root)
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if lib, err := filepath.Abs(libDir(conformanceDir)); err == nil {
		_ = os.Symlink(lib, filepath.Join(root, ".lib"))
	}
	disk := func(vpath string) string { return filepath.Join(root, filepath.FromSlash(vpath)) }
	for _, u := range units {
		p := disk(vfsPath(u.Name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			resp.Err = fmt.Sprintf("scratch dir: %v", err)
			return
		}
		if err := os.WriteFile(p, []byte(u.Content), 0o644); err != nil {
			resp.Err = fmt.Sprintf("scratch dir: %v", err)
			return
		}
	}
	var files []string
	allowJS, _ := opts.boolOpt("allowjs")
	if v, _ := opts.boolOpt("checkjs"); v {
		allowJS = true
	}
	for _, i := range selectRoots(units, opts) {
		if isCodeUnit(units[i].Name) || allowJS && isJSUnit(units[i].Name) {
			files = append(files, disk(vfsPath(units[i].Name)))
		}
	}
	keys := unitKeySet(units)
	// FIXME: JavaScript files are checked as TypeScript (no JSDoc types);
	// an unchecked one (no checkJs) reports only its syntax errors.
	syntaxOnly := map[string]bool{}
	for _, u := range units {
		if isJSUnit(u.Name) && !jsChecked(u, opts) {
			syntaxOnly[fileKey(u.Name)] = true
		}
	}

	pas := driver.NewPaseratiWithInitializersAndBaseDir(tscInitializers(), disk("/.src"))
	defer pas.Cleanup()
	pas.AddResolver(nodemodules.NewResolver(nodemodules.Options{Mode: nodemodules.Types, ImporterFormat: nodeFormatResolution(opts), NoHashSlashImports: opts.moduleIs("node16", "node18")}))
	pas.AddResolver(&rootedResolver{root: root, fs: modules.NewOSFileSystemResolver(root)})

	defer func() {
		if r := recover(); r != nil {
			resp.Panic = fmt.Sprintf("panic: %v", r)
		}
	}()
	results := pas.CheckFilesWithOptions(files, tscCheckOptions(opts))
	for _, res := range results {
		rel, err := filepath.Rel(root, res.Path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		key := strings.ToLower(displayName(path.Clean("/" + filepath.ToSlash(rel))))
		if !keys[key] {
			continue // a /.lib file or one the test did not declare
		}
		for _, e := range res.Diagnostics {
			phase := "check"
			if e.Kind() == "Syntax" {
				phase = "parse"
				resp.ParseErrors++
			}
			if e.Kind() == "Compile" || syntaxOnly[key] && phase != "parse" {
				continue
			}
			d := toDiag(e, phase)
			d.File = key
			resp.Actual = append(resp.Actual, d)
		}
	}
	resp.Actual = dedupeDiags(resp.Actual)
}

// rootedResolver resolves rooted specifiers (`/// <reference
// path="/.lib/react.d.ts" />`, `import "/a"`) in the scratch directory that
// stands for the harness's virtual file system root.
type rootedResolver struct {
	root string
	fs   *modules.FileSystemResolver
}

func (r *rootedResolver) Name() string  { return "TestRoot" }
func (r *rootedResolver) Priority() int { return 10 }
func (r *rootedResolver) CanResolve(specifier string) bool {
	return strings.HasPrefix(specifier, "/") && !strings.HasPrefix(specifier, r.root+"/")
}
func (r *rootedResolver) Resolve(specifier, fromPath string) (*modules.ResolvedModule, error) {
	return r.fs.Resolve(filepath.Join(r.root, filepath.FromSlash(specifier)), fromPath)
}

// moduleIs reports whether --module is one of kinds.
func (o Options) moduleIs(kinds ...string) bool {
	m := strings.ToLower(strings.TrimSpace(o["module"]))
	for _, k := range kinds {
		if m == k {
			return true
		}
	}
	return false
}

// nodeFormatResolution reports whether imports resolve per importing file's
// format: node16/nodenext resolution, which module=node16..nodenext implies.
func nodeFormatResolution(o Options) bool {
	res := strings.ToLower(strings.TrimSpace(o["moduleresolution"]))
	if res != "" {
		return res == "node16" || res == "nodenext"
	}
	return o.moduleIs("node16", "node18", "node20", "nodenext")
}

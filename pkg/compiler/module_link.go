package compiler

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
)

// Module linking (ECMA-262 16.2.1.6.4 InitializeEnvironment, the part that
// can fail): every requested module must load, and every named import and
// indirect export must resolve, through ResolveExport, to exactly one
// binding. A failure is a SyntaxError in the resolution phase, reported
// before any module in the graph runs.
//
// Each module checks its own imports and indirect exports when it is
// compiled; a dependency that failed its own check carries that failure in
// its module record, which surfaces here as a load failure.

// linkModule holds the import and export entries of one module.
type linkModule struct {
	key      string // canonical path, the module's identity
	fromPath string // what its specifiers resolve against
	opaque   bool   // no source entries (native or JSON module)

	local    map[string]bool         // export names bound in the module itself
	indirect map[string]linkIndirect // export name -> imported binding
	stars    []string                // `export * from` specifiers
	imports  []linkImport            // named and default imports
	requests []linkRequest           // every requested module, in order
}

type linkIndirect struct {
	spec       string
	importName string // "*" for `export * as ns from`
	node       parser.Node
}

// sourcePhaseImport marks an `import source x` entry.
const sourcePhaseImport = "<source>"

type linkImport struct {
	spec       string
	importName string
	node       parser.Node
}

type linkRequest struct {
	spec string
	node parser.Node
}

// linkBinding is a resolved export: a binding name in a module ("*" names
// the module's namespace object).
type linkBinding struct {
	module string
	name   string
}

type moduleLinker struct {
	c       *Compiler
	root    *linkModule
	modules map[string]*linkModule
}

// checkModuleLinking reports the first linking error of the module being
// compiled.
func (c *Compiler) checkModuleLinking(program *parser.Program) {
	if c.moduleLoader == nil || c.moduleBindings == nil {
		return
	}
	fromPath := c.linkFromPath()
	l := &moduleLinker{c: c, modules: make(map[string]*linkModule)}
	l.root = newLinkModule(canonicalLinkKey(fromPath), fromPath, program)
	l.modules[l.root.key] = l.root

	// Loading the whole graph comes before linking any of it, so a module
	// that cannot be loaded is reported ahead of a dependency's link error.
	var linkFailure error
	var linkFailureNode parser.Node
	for _, req := range l.root.requests {
		_, err := l.load(l.root, req.spec)
		if err == nil {
			continue
		}
		if isLinkFailure(err) {
			if linkFailure == nil {
				linkFailure, linkFailureNode = err, req.node
			}
			continue
		}
		l.failLoad(req.node, err)
		return
	}
	if linkFailure != nil {
		l.failLoad(linkFailureNode, linkFailure)
		return
	}
	for _, imp := range l.root.imports {
		target, err := l.load(l.root, imp.spec)
		if err != nil {
			l.failLoad(imp.node, err)
			return
		}
		if imp.importName == sourcePhaseImport {
			// GetModuleSource: a Source Text Module Record has no source
			// representation to import.
			if !target.opaque {
				l.fail(imp.node, "Source phase import is not available for module '%s'", imp.spec)
				return
			}
			continue
		}
		if !l.checkResolves(target, imp.importName, imp.spec, imp.node) {
			return
		}
	}
	for name, ind := range l.root.indirect {
		if ind.importName == "*" {
			continue
		}
		if !l.checkResolves(l.root, name, ind.spec, ind.node) {
			return
		}
	}
}

// checkResolves reports an error unless name resolves in m; spec names the
// requested module for the message.
func (l *moduleLinker) checkResolves(m *linkModule, name, spec string, node parser.Node) bool {
	res, ambiguous, err := l.resolveExport(m, name, map[linkBinding]bool{})
	switch {
	case err != nil:
		l.failLoad(node, err)
	case ambiguous:
		l.fail(node, "The requested module '%s' contains conflicting star exports for name '%s'", spec, name)
	case res == nil:
		l.fail(node, "The requested module '%s' does not provide an export named '%s'", spec, name)
	default:
		return true
	}
	return false
}

func (l *moduleLinker) fail(node parser.Node, format string, args ...interface{}) {
	err := NewCompileError(node, "SyntaxError: "+fmt.Sprintf(format, args...))
	l.c.errors = append(l.c.errors, resolutionError(err))
}

// failLoad reports a module that failed to load. The diagnostic points at the
// failure inside that module when it has a position (#148).
func (l *moduleLinker) failLoad(node parser.Node, err error) {
	msg := err.Error()
	if le, ok := err.(*linkLoadError); ok {
		if _, isExport := node.(parser.ExportDeclaration); isExport {
			msg = fmt.Sprintf("Failed to load module '%s' for re-export: %v", le.spec, le.cause)
		}
	}
	compileErr := NewCompileError(node, msg)
	compileErr.Cause = err
	if pos, ok := errors.PositionOf(err); ok {
		compileErr.Position = pos
	}
	l.c.errors = append(l.c.errors, resolutionError(compileErr))
}

// resolveExport is ResolveExport (16.2.1.6.3): nil for a name that does not
// resolve (missing or circular), ambiguous for conflicting star exports.
func (l *moduleLinker) resolveExport(m *linkModule, name string, resolveSet map[linkBinding]bool) (*linkBinding, bool, error) {
	if m.opaque {
		return &linkBinding{m.key, name}, false, nil
	}
	visit := linkBinding{m.key, name}
	if resolveSet[visit] {
		return nil, false, nil // circular import request
	}
	resolveSet[visit] = true

	if m.local[name] {
		return &linkBinding{m.key, name}, false, nil
	}
	if ind, ok := m.indirect[name]; ok {
		target, err := l.load(m, ind.spec)
		if err != nil {
			return nil, false, err
		}
		if ind.importName == "*" {
			return &linkBinding{target.key, "*"}, false, nil
		}
		return l.resolveExport(target, ind.importName, resolveSet)
	}
	if name == "default" {
		return nil, false, nil // a star export never provides default
	}
	var starResolution *linkBinding
	for _, spec := range m.stars {
		target, err := l.load(m, spec)
		if err != nil {
			return nil, false, err
		}
		res, ambiguous, err := l.resolveExport(target, name, resolveSet)
		if err != nil || ambiguous {
			return nil, ambiguous, err
		}
		if res == nil {
			continue
		}
		if starResolution == nil {
			starResolution = res
		} else if *starResolution != *res {
			return nil, true, nil
		}
	}
	return starResolution, false, nil
}

// linkLoadError is a requested module that could not be loaded, parsed or
// compiled (including failing its own linking).
type linkLoadError struct {
	spec  string
	cause error
}

func (e *linkLoadError) Error() string {
	return fmt.Sprintf("Failed to load module '%s': %v", e.spec, e.cause)
}

func (e *linkLoadError) Unwrap() error { return e.cause }

// load returns the entries of the module spec names, as requested from m.
func (l *moduleLinker) load(m *linkModule, spec string) (*linkModule, error) {
	rec, err := l.c.moduleLoader.LoadModule(spec, m.fromPath)
	if err != nil {
		return nil, &linkLoadError{spec, err}
	}
	record, ok := rec.(*modules.ModuleRecord)
	if !ok || record == nil {
		return nil, &linkLoadError{spec, fmt.Errorf("module loader returned no record")}
	}
	if record.Error != nil {
		return nil, &linkLoadError{spec, record.Error}
	}
	key := canonicalLinkKey(record.ResolvedPath)
	if lm, ok := l.modules[key]; ok {
		return lm, nil
	}
	var lm *linkModule
	switch {
	case record.IsJSON:
		lm = &linkModule{key: key, local: map[string]bool{"default": true}}
	case record.AST == nil || record.IsNativeModule():
		lm = &linkModule{key: key, opaque: true}
	default:
		lm = newLinkModule(key, record.ResolvedPath, record.AST)
	}
	l.modules[key] = lm
	return lm, nil
}

// isLinkFailure reports whether a dependency failed to load only because
// its own linking failed: the innermost cause is a resolution CompileError.
func isLinkFailure(err error) bool {
	for err != nil {
		u, ok := err.(interface{ Unwrap() error })
		if !ok || u.Unwrap() == nil {
			break
		}
		err = u.Unwrap()
	}
	ce, ok := err.(*errors.CompileError)
	return ok && ce.Resolution
}

// linkFromPath is the path this module's specifiers resolve against. A
// module file named by a relative path is made absolute: the file system
// resolver joins a relative importer path onto its own base directory, which
// already locates the importer, so a relative one would be applied twice.
func (c *Compiler) linkFromPath() string {
	path := c.moduleBindings.ModulePath
	if path == "" {
		return "."
	}
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
				return abs
			}
		}
	}
	return path
}

func canonicalLinkKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// newLinkModule collects a module's import and export entries. TypeScript
// type-only imports are skipped, but type exports count as exports, since a
// plain `import { T }` may name a type.
func newLinkModule(key, fromPath string, program *parser.Program) *linkModule {
	m := &linkModule{
		key:      key,
		fromPath: fromPath,
		local:    make(map[string]bool),
		indirect: make(map[string]linkIndirect),
	}
	type importBinding struct{ spec, importName string }
	importedLocals := make(map[string]importBinding)
	request := func(src *parser.StringLiteral, node parser.Node) {
		if src != nil {
			m.requests = append(m.requests, linkRequest{src.Value, node})
		}
	}

	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *parser.ImportDeclaration:
			if s.Source == nil || s.Attributes["type"] == "json" {
				continue
			}
			spec := s.Source.Value
			if !s.IsTypeOnly {
				request(s.Source, s)
			}
			for _, sp := range s.Specifiers {
				switch is := sp.(type) {
				case *parser.ImportDefaultSpecifier:
					if s.IsSource {
						m.imports = append(m.imports, linkImport{spec, sourcePhaseImport, is})
						continue
					}
					importedLocals[is.Local.Value] = importBinding{spec, "default"}
					if !s.IsTypeOnly {
						m.imports = append(m.imports, linkImport{spec, "default", is})
					}
				case *parser.ImportNamedSpecifier:
					importedLocals[is.Local.Value] = importBinding{spec, is.Imported.Value}
					if !s.IsTypeOnly && !is.IsTypeOnly {
						m.imports = append(m.imports, linkImport{spec, is.Imported.Value, is})
					}
				case *parser.ImportNamespaceSpecifier:
					importedLocals[is.Local.Value] = importBinding{spec, "*"}
				}
			}
		case *parser.ExportDefaultDeclaration:
			m.local["default"] = true
		case *parser.ExportAllDeclaration:
			if s.Source == nil {
				continue
			}
			if !s.IsTypeOnly {
				request(s.Source, s)
			}
			if s.Exported != nil {
				m.indirect[moduleExportNameValue(s.Exported)] = linkIndirect{s.Source.Value, "*", s}
			} else {
				m.stars = append(m.stars, s.Source.Value)
			}
		case *parser.ExportNamedDeclaration:
			if s.Declaration != nil {
				for name := range exportedLocalNames(s.Declaration) {
					m.local[name] = true
				}
				continue
			}
			if s.Source != nil && !s.IsTypeOnly {
				request(s.Source, s)
			}
			for _, spec := range s.Specifiers {
				es, ok := spec.(*parser.ExportNamedSpecifier)
				if !ok {
					continue
				}
				exported := moduleExportNameValue(es.Exported)
				local := moduleExportNameValue(es.Local)
				switch {
				case s.Source != nil:
					if s.IsTypeOnly {
						m.local[exported] = true
					} else {
						m.indirect[exported] = linkIndirect{s.Source.Value, local, es}
					}
				default:
					m.local[exported] = true
				}
			}
		case *parser.ExpressionStatement:
			// TypeScript `export = x` replaces the module's export shape.
			if s.Token != nil && s.Token.Literal == "export" {
				m.opaque = true
			}
		}
	}

	// `import { x } from 'm'; export { x }` re-exports m's binding: it is an
	// indirect export, not a local one (16.2.1.7.1 ParseModule). Re-exporting
	// `import * as ns` likewise resolves to m's namespace, so two modules that
	// do so for the same m are not ambiguous.
	for _, stmt := range program.Statements {
		s, ok := stmt.(*parser.ExportNamedDeclaration)
		if !ok || s.Source != nil || s.Declaration != nil || s.IsTypeOnly {
			continue
		}
		for _, spec := range s.Specifiers {
			es, ok := spec.(*parser.ExportNamedSpecifier)
			if !ok {
				continue
			}
			if ib, isImport := importedLocals[moduleExportNameValue(es.Local)]; isImport {
				exported := moduleExportNameValue(es.Exported)
				delete(m.local, exported)
				m.indirect[exported] = linkIndirect{ib.spec, ib.importName, es}
			}
		}
	}
	return m
}

// exportedLocalNames lists the names an `export <declaration>` binds,
// TypeScript declarations included.
func exportedLocalNames(decl parser.Statement) map[string]bool {
	names := make(map[string]bool)
	for _, d := range exportedDeclarationNames(decl) {
		names[d.name] = true
	}
	collectTypeScriptDeclaredNames(decl, names)
	return names
}

// asyncModulesOfDeferredImport is GatherAsynchronousTransitiveDependencies
// for `import defer` of spec: the modules with top-level await reachable
// from it through synchronous modules, as canonical paths in evaluation
// order. A deferred module that itself uses top-level await is its own
// (only) entry.
func (c *Compiler) asyncModulesOfDeferredImport(spec string) []string {
	if c.moduleLoader == nil || c.moduleBindings == nil {
		return nil
	}
	fromPath := c.linkFromPath()
	var out []string
	seen := map[string]bool{}
	var visit func(spec, from string)
	visit = func(spec, from string) {
		rec, err := c.moduleLoader.LoadModule(spec, from)
		if err != nil {
			return
		}
		record, ok := rec.(*modules.ModuleRecord)
		if !ok || record == nil || record.AST == nil || seen[record.ResolvedPath] {
			return
		}
		seen[record.ResolvedPath] = true
		if containsTopLevelAwait(record.AST) {
			out = append(out, record.ResolvedPath)
			return
		}
		for _, req := range newLinkModule(record.ResolvedPath, record.ResolvedPath, record.AST).requests {
			visit(req.spec, record.ResolvedPath)
		}
	}
	visit(spec, fromPath)
	return out
}

// containsTopLevelAwait reports whether a module's top level (outside any
// function) uses await: an await expression, for await, or await using.
func containsTopLevelAwait(program *parser.Program) bool {
	found := false
	var walk func(n parser.Node)
	walk = func(n parser.Node) {
		if found || isNilASTNode(n) {
			return
		}
		switch x := n.(type) {
		case *parser.AwaitExpression:
			found = true
			return
		case *parser.ForOfStatement:
			if x.IsAsync {
				found = true
				return
			}
		case *parser.LetStatement:
			if x.IsAwaitUsing {
				found = true
				return
			}
		case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral, *parser.ShorthandMethod,
			*parser.ClassBody:
			return
		}
		walkChildren(n, walk)
	}
	for _, stmt := range program.Statements {
		walk(stmt)
	}
	return found
}

package modules

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"

	paseratiErrors "github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/source"
)

// parseAhead parses modules on other cores before the sequential loader asks
// for them. Loading is depth-first (parse a module, load its imports, check
// it), so a module's imports are known long before they are loaded: once a
// module is parsed, its imports and triple-slash references are resolved,
// read and parsed in the background, and so on down the graph. The loader
// then takes the finished parse instead of parsing itself; which modules
// load, and in what order they are checked, does not change.
type parseAhead struct {
	ml *moduleLoader

	mu     sync.Mutex
	parses map[string]*aheadParse // by resolved path

	// slots bounds the parses running at once.
	slots chan struct{}

	// pending counts the parses started and not yet finished, with the
	// imports they start.
	pending sync.WaitGroup
}

// aheadParse is one module's background parse; its fields are set before
// done is closed.
type aheadParse struct {
	done    chan struct{}
	source  *source.SourceFile
	program *parser.Program
	errs    []paseratiErrors.PaseratiError
	readErr error
}

func newParseAhead(ml *moduleLoader) *parseAhead {
	return &parseAhead{
		ml:     ml,
		parses: make(map[string]*aheadParse),
		slots:  make(chan struct{}, runtime.GOMAXPROCS(0)),
	}
}

// EnableParseAhead turns on background parsing of the modules a load will
// reach (see parseAhead). Resolvers are then called from more than one
// goroutine, one call at a time.
func (ml *moduleLoader) EnableParseAhead() {
	if ml.ahead == nil {
		ml.ahead = newParseAhead(ml)
	}
}

// ParseAhead starts parsing the module specifier names from fromPath, and
// what it imports, in the background. It does nothing unless parse-ahead is
// enabled.
func (ml *moduleLoader) ParseAhead(specifier, fromPath string) {
	if ml.ahead != nil {
		ml.ahead.pending.Add(1)
		go ml.ahead.start(specifier, fromPath)
	}
}

// GlobalAugmentingModules waits for every parse ahead to finish and returns
// the paths, sorted, of the parsed modules that declare a top-level
// `declare global { ... }`. tsc binds the globals of every file before it
// checks any; loading these first gives the modules checked before them
// the same view.
func (ml *moduleLoader) GlobalAugmentingModules() []string {
	if ml.ahead == nil {
		return nil
	}
	pa := ml.ahead
	pa.pending.Wait()
	pa.mu.Lock()
	defer pa.mu.Unlock()
	var paths []string
	for path, p := range pa.parses {
		if p.readErr == nil && len(p.errs) == 0 && declaresGlobal(p.program) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

// declaresGlobal reports whether program has a top-level `declare global`.
func declaresGlobal(program *parser.Program) bool {
	if program == nil {
		return false
	}
	for _, stmt := range program.Statements {
		var node parser.Node = stmt
		if es, ok := stmt.(*parser.ExpressionStatement); ok {
			node = es.Expression
		}
		if ns, ok := node.(*parser.NamespaceDeclaration); ok && ns.AmbientModule && ns.Name != nil && ns.Name.Value == "global" {
			return true
		}
	}
	return false
}

// start resolves specifier and parses the file it names, unless that file
// is already parsed or being parsed. A module that would not load the
// ordinary way (a native module, an unresolvable specifier) is left to the
// loader.
func (pa *parseAhead) start(specifier, fromPath string) {
	defer pa.pending.Done()
	resolved, err := pa.ml.resolveModule(specifier, fromPath)
	if err != nil || resolved.ResolvedPath == "" || resolved.Source == nil {
		return
	}
	if _, native := resolved.Source.(NativeModuleSource); native {
		_ = resolved.Source.Close()
		return
	}
	pa.mu.Lock()
	if _, seen := pa.parses[resolved.ResolvedPath]; seen {
		pa.mu.Unlock()
		_ = resolved.Source.Close()
		return
	}
	p := &aheadParse{done: make(chan struct{})}
	pa.parses[resolved.ResolvedPath] = p
	pa.mu.Unlock()

	pa.slots <- struct{}{}
	p.parse(resolved)
	<-pa.slots
	close(p.done)

	// The loader does not load the imports of a module that fails to parse.
	if p.readErr != nil || len(p.errs) > 0 {
		return
	}
	pa.startImports(p.source, p.program, resolved.ResolvedPath)
}

// startImports parses ahead what a parsed module will load next: its
// triple-slash references and its imports, as loadModuleSequential does.
func (pa *parseAhead) startImports(src *source.SourceFile, program *parser.Program, path string) {
	if src != nil {
		for _, ref := range TripleSlashReferences(src.Content) {
			pa.pending.Add(1)
			go pa.start(ref, path)
		}
	}
	if program == nil {
		return
	}
	for _, spec := range extractImportSpecs(program) {
		if spec.Attributes != nil && spec.Attributes["type"] == "json" {
			continue
		}
		pa.pending.Add(1)
		go pa.start(spec.ModulePath, path)
	}
}

func (p *aheadParse) parse(resolved *ResolvedModule) {
	defer resolved.Source.Close()
	content, err := io.ReadAll(resolved.Source)
	if err != nil {
		p.readErr = fmt.Errorf("failed to read source: %w", err)
		return
	}
	p.source = &source.SourceFile{
		Name:    resolved.ResolvedPath,
		Path:    resolved.ResolvedPath,
		Content: string(content),
	}
	p.program, p.errs = parser.NewParser(lexer.NewLexerWithSource(p.source)).ParseProgram()
}

// take returns the background parse of the file at path, waiting for it to
// finish, or nil when there is none.
func (pa *parseAhead) take(path string) *aheadParse {
	pa.mu.Lock()
	p := pa.parses[path]
	pa.mu.Unlock()
	if p == nil {
		return nil
	}
	<-p.done
	return p
}

// startImportsOf parses ahead the imports of a module the loader parsed
// itself (a root file, or one reached before parse-ahead got to it).
func (pa *parseAhead) startImportsOf(record *ModuleRecord) {
	go pa.startImports(record.Source, record.AST, record.ResolvedPath)
}

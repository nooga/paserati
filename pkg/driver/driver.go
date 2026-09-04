package driver

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/checker"
	"github.com/nooga/paserati/pkg/compiler"
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/source"
	"github.com/nooga/paserati/pkg/vm"
)

const debugDriver = false

func debugPrintf(format string, args ...interface{}) {
	if debugDriver {
		fmt.Printf(format, args...)
	}
}

// compilerAdapter adapts compiler.Compiler to modules.Compiler interface
type compilerAdapter struct {
	*compiler.Compiler
}

// Compile adapts the return type from *vm.Chunk to interface{}
func (ca *compilerAdapter) Compile(node parser.Node) (interface{}, []errors.PaseratiError) {
	chunk, errs := ca.Compiler.Compile(node)
	return chunk, errs
}

// SetChecker adapts the parameter type from modules.TypeChecker to *checker.Checker
func (ca *compilerAdapter) SetChecker(tc modules.TypeChecker) {
	// Type assert to get the concrete checker
	if concreteChecker, ok := tc.(*checker.Checker); ok {
		ca.Compiler.SetChecker(concreteChecker)
	}
}

// Paserati represents a persistent interpreter session.
// It maintains state between separate code evaluations,
// allowing variables and functions defined in one evaluation
// to be used in subsequent ones.
type Paserati struct {
	vmInstance       *vm.VM
	checker          *checker.Checker
	compiler         *compiler.Compiler
	moduleLoader     modules.ModuleLoader
	heapAlloc        *compiler.HeapAlloc   // Unified global heap allocator
	builtinGlobals   []string              // Builtin names indexed by their unified heap slot
	preparedChunk    *vm.Chunk             // Most recently checked chunk (avoids repeat validation without retaining every chunk)
	nativeResolver   *NativeModuleResolver // *NativeModuleResolver - defined in native_module.go to avoid import cycles
	ignoreTypeErrors bool                  // When true, type checking errors are ignored and compilation continues
	skipTypeCheck    bool                  // When true, type checker is not run at all (for pure JS mode)
}

// SetIgnoreTypeErrors sets whether type checking errors should be ignored
func (p *Paserati) SetIgnoreTypeErrors(ignore bool) {
	p.ignoreTypeErrors = ignore
	// Also propagate to module loader so imported modules respect this setting
	if p.moduleLoader != nil {
		p.moduleLoader.SetIgnoreTypeErrors(ignore)
	}
}

// SetSkipTypeCheck sets whether to completely skip type checking
// When true, the type checker is not run at all (for pure JS mode)
func (p *Paserati) SetSkipTypeCheck(skip bool) {
	p.skipTypeCheck = skip
	// Also propagate to module loader so imported modules respect this setting
	if p.moduleLoader != nil {
		p.moduleLoader.SetSkipTypeCheck(skip)
	}
}

// SetSkipStrictPropertyInit controls whether TS2564 is emitted. Default false
// (emit). Used by paserati-testtsc to opt out per-file based on TS directives.
func (p *Paserati) SetSkipStrictPropertyInit(skip bool) {
	p.checker.SetSkipStrictPropertyInit(skip)
}

// SetSkipDefiniteAssignment controls whether TS2454 is emitted. Default false
// (emit). Used by paserati-testtsc to opt out per-file based on TS directives.
func (p *Paserati) SetSkipDefiniteAssignment(skip bool) {
	p.checker.SetSkipDefiniteAssignment(skip)
}

// SetAllowUnreachableCode mirrors --allowUnreachableCode, which suppresses
// TS2695. Default false (emit). Used by paserati-testtsc to opt out per-file
// based on TS directives.
func (p *Paserati) SetAllowUnreachableCode(allow bool) {
	p.checker.SetAllowUnreachableCode(allow)
}

// SetStrictNullChecks mirrors --strictNullChecks, which gates TS18050.
// Default true, matching TypeScript 6.0.
func (p *Paserati) SetStrictNullChecks(strict bool) {
	p.checker.SetStrictNullChecks(strict)
}

// SetAlwaysStrict mirrors --alwaysStrict, which gates TS1212. Part of the
// strict family, so on by default as of TypeScript 6.0.
func (p *Paserati) SetAlwaysStrict(strict bool) {
	p.checker.SetAlwaysStrict(strict)
}

// SetIsModule tells the checker the source is a module, which selects TS1214
// over TS1212 when reporting a strict-mode reserved word.
func (p *Paserati) SetIsModule(isModule bool) {
	p.checker.SetIsModule(isModule)
}

// SetNoImplicitOverride controls whether overriding class members require an
// explicit `override` modifier.
func (p *Paserati) SetNoImplicitOverride(enabled bool) {
	p.checker.SetNoImplicitOverride(enabled)
}

// SetAllowTopLevelReturn controls whether script/eval style top-level returns
// are accepted by the type checker.
func (p *Paserati) SetAllowTopLevelReturn(allow bool) {
	p.checker.SetAllowTopLevelReturn(allow)
}

// EnableModuleMode enables module mode for the checker and compiler
func (p *Paserati) EnableModuleMode(modulePath string) {
	p.checker.EnableModuleMode(modulePath, p.moduleLoader)
	p.compiler.EnableModuleMode(modulePath, p.moduleLoader)
}

// Cleanup breaks circular references to allow garbage collection
// CancelVM signals the VM to stop execution at the next safe point
func (p *Paserati) CancelVM() {
	if p.vmInstance != nil {
		p.vmInstance.Cancel()
	}
}

func (p *Paserati) Cleanup() {
	// Reset VM state to clear all references to objects/closures/frames
	// This is critical to prevent memory leaks in long-running processes
	if p.vmInstance != nil {
		p.vmInstance.Reset()
		p.vmInstance.SetModuleLoader(nil)
	}

	// Break circular references between VM and module loader
	if p.moduleLoader != nil {
		p.moduleLoader.SetVMInstance(nil)
	}

	// Clear references
	p.vmInstance = nil
	p.checker = nil
	p.compiler = nil
	p.moduleLoader = nil
	p.heapAlloc = nil
	p.builtinGlobals = nil
	p.preparedChunk = nil
	p.nativeResolver = nil
}

// NewPaserati creates a new Paserati session with a fresh VM and Checker.
// Uses the current working directory as the base for module resolution.
func NewPaserati() *Paserati {
	return NewPaseratiWithBaseDir(".")
}

// NewPaseratiWithInitializers creates a new Paserati session with custom builtin initializers
func NewPaseratiWithInitializers(initializers []builtins.BuiltinInitializer) *Paserati {
	return NewPaseratiWithInitializersAndBaseDir(initializers, ".")
}

// NewPaseratiWithInitializersAndBaseDir creates a new Paserati session with custom builtin initializers and base directory
func NewPaseratiWithInitializersAndBaseDir(customInitializers []builtins.BuiltinInitializer, baseDir string) *Paserati {
	// Create module loader first
	config := modules.DefaultLoaderConfig()

	// Create file system resolver for the specified base directory. Uses the
	// real OS filesystem directly (not an fs.FS wrapping os.DirFS(baseDir))
	// so that an entry file passed by absolute path - and any relative
	// import resolved against it - can be found: fs.FS's contract rejects
	// any absolute path outright, which surfaced as "no resolver could
	// handle specifier" for a relative import from an absolute-path entry
	// module (#232, misreported as top-level await not working, since that's
	// what the failing import happened to sit next to).
	fsResolver := modules.NewOSFileSystemResolver(baseDir)

	// Create module loader with file system resolver
	moduleLoader := modules.NewModuleLoader(config, fsResolver)

	// Create unified heap allocator for coordinating global indices
	heapAlloc := compiler.NewHeapAlloc()

	// Create checker and compiler with custom initializers
	typeChecker := checker.NewCheckerWithInitializers(customInitializers)
	comp := compiler.NewCompiler()
	comp.SetChecker(typeChecker)

	// Create VM and initialize builtin system
	vmInstance := vm.NewVM()

	paserati := &Paserati{
		vmInstance:   vmInstance,
		checker:      typeChecker,
		compiler:     comp,
		moduleLoader: moduleLoader,
		heapAlloc:    heapAlloc,
	}

	// Wire the module loader into the VM
	vmInstance.SetModuleLoader(moduleLoader)
	vmInstance.SetImportMetaBaseDir(baseDir)

	// Set the eval driver for OpDirectEval
	vmInstance.SetEvalDriver(paserati)

	// Set the VM instance in the module loader for native module initialization
	moduleLoader.SetVMInstance(vmInstance)

	// Initialize builtins using custom initializers
	if err := initializeBuiltinsWithCustom(paserati, customInitializers); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Builtin initialization failed: %v\n", err)
	}

	// Sync the VM's prototype fields back to the default realm
	// This ensures the realm has the real prototypes (not just initial placeholders)
	vmInstance.SyncPrototypesToRealm()

	// Set up the checker factory for the module loader
	// This allows the module loader to create type checkers without circular imports
	moduleLoader.SetCheckerFactory(func() modules.TypeChecker {
		// Create a new checker instance for module type checking with custom initializers
		newChecker := checker.NewCheckerWithInitializers(customInitializers)
		// Enable module mode so the checker can resolve imports
		newChecker.EnableModuleMode("", moduleLoader)
		debugPrintf("// [Driver] Created new checker for module: %p\n", newChecker)
		return newChecker
	})

	// Set up the compiler factory for the module loader AFTER builtins are initialized
	// This allows the module loader to create compilers without circular imports
	moduleLoader.SetCompilerFactory(func() modules.Compiler {
		// Create a new compiler instance for module compilation
		newCompiler := compiler.NewCompiler()

		// CRITICAL: Give module compiler the SAME heap allocator instance
		// This ensures all compilers coordinate on the exact same global indices
		newCompiler.SetHeapAlloc(paserati.heapAlloc)
		// This compiler compiles one specific file resolved by the module
		// loader (an import target, or the entry file under RunFile), unlike
		// the persistent session compiler reused across RunString/REPL calls.
		// See #103.
		newCompiler.SetLoadedViaModuleLoader(true)

		// Return a wrapper that adapts the return type to interface{}
		return &compilerAdapter{newCompiler}
	})

	// Enable module mode for the main checker by default for consistent type checking
	typeChecker.EnableModuleMode("", moduleLoader)

	// Install built-in Paserati modules
	installBuiltinModules(paserati)

	return paserati
}

// NewPaseratiWithBaseDir creates a new Paserati session with a custom base directory
// for module resolution. This allows tests and other code to specify where modules
// should be resolved from without changing the global working directory.
func NewPaseratiWithBaseDir(baseDir string) *Paserati {
	// Create module loader first
	config := modules.DefaultLoaderConfig()

	// Create file system resolver for the specified base directory. Uses the
	// real OS filesystem directly (not an fs.FS wrapping os.DirFS(baseDir))
	// so that an entry file passed by absolute path - and any relative
	// import resolved against it - can be found: fs.FS's contract rejects
	// any absolute path outright, which surfaced as "no resolver could
	// handle specifier" for a relative import from an absolute-path entry
	// module (#232, misreported as top-level await not working, since that's
	// what the failing import happened to sit next to).
	fsResolver := modules.NewOSFileSystemResolver(baseDir)

	// Create module loader with file system resolver
	moduleLoader := modules.NewModuleLoader(config, fsResolver)

	// Create unified heap allocator for coordinating global indices
	heapAlloc := compiler.NewHeapAlloc()

	// Create checker and compiler
	typeChecker := checker.NewChecker()
	comp := compiler.NewCompiler()
	comp.SetChecker(typeChecker)

	// Create VM and initialize builtin system
	vmInstance := vm.NewVM()

	paserati := &Paserati{
		vmInstance:   vmInstance,
		checker:      typeChecker,
		compiler:     comp,
		moduleLoader: moduleLoader,
		heapAlloc:    heapAlloc,
	}

	// Wire the module loader into the VM
	vmInstance.SetModuleLoader(moduleLoader)
	vmInstance.SetImportMetaBaseDir(baseDir)

	// Set the eval driver for OpDirectEval
	vmInstance.SetEvalDriver(paserati)

	// Set the VM instance in the module loader for native module initialization
	moduleLoader.SetVMInstance(vmInstance)

	// Initialize builtins using new initializer system FIRST
	if err := initializeBuiltins(paserati); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Builtin initialization failed: %v\n", err)
	}

	// Sync the VM's prototype fields back to the default realm
	// This ensures the realm has the real prototypes (not just initial placeholders)
	vmInstance.SyncPrototypesToRealm()

	// Set up the checker factory for the module loader
	// This allows the module loader to create type checkers without circular imports
	moduleLoader.SetCheckerFactory(func() modules.TypeChecker {
		// Create a new checker instance for module type checking with standard initializers
		newChecker := checker.NewChecker()
		// Enable module mode so the checker can resolve imports
		newChecker.EnableModuleMode("", moduleLoader)
		debugPrintf("// [Driver] Created new checker for module: %p\n", newChecker)
		return newChecker
	})

	// Set up the compiler factory for the module loader AFTER builtins are initialized
	// This allows the module loader to create compilers without circular imports
	moduleLoader.SetCompilerFactory(func() modules.Compiler {
		// Create a new compiler instance for module compilation
		newCompiler := compiler.NewCompiler()

		// CRITICAL: Give module compiler the SAME heap allocator instance
		// This ensures all compilers coordinate on the exact same global indices
		newCompiler.SetHeapAlloc(paserati.heapAlloc)
		// This compiler compiles one specific file resolved by the module
		// loader (an import target, or the entry file under RunFile), unlike
		// the persistent session compiler reused across RunString/REPL calls.
		// See #103.
		newCompiler.SetLoadedViaModuleLoader(true)

		// Return a wrapper that adapts the return type to interface{}
		return &compilerAdapter{newCompiler}
	})

	// Enable module mode for the main checker by default for consistent type checking
	typeChecker.EnableModuleMode("", moduleLoader)

	// Install built-in Paserati modules
	installBuiltinModules(paserati)

	return paserati
}

// CompileProgram compiles a parsed program using the initialized Paserati session
// This is used by the test framework to compile with proper initialization
func (p *Paserati) CompileProgram(program *parser.Program) (*vm.Chunk, []errors.PaseratiError) {
	// Honor session settings to ignore/skip type errors (used for Test262)
	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
	p.compiler.SetSkipTypeCheck(p.skipTypeCheck)
	chunk, errs := p.compiler.Compile(program)
	if chunk != nil {
		chunk.BuiltinGlobalNames = append([]string(nil), p.builtinGlobals...)
		chunk.GlobalNames = indexedGlobalNames(p.heapAlloc.GetNameToIndexMap())
	}
	// Sync global names to VM so with statements can resolve global variable names
	p.SyncGlobalNamesFromCompiler()
	return chunk, errs
}

// CompileProgramWithStrictMode compiles a parsed program with the specified strict mode
// This is used by eval() to compile code in strict mode when called from strict context
func (p *Paserati) CompileProgramWithStrictMode(program *parser.Program, strict bool) (*vm.Chunk, []errors.PaseratiError) {
	// Honor session settings to ignore/skip type errors (used for Test262)
	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
	p.compiler.SetSkipTypeCheck(p.skipTypeCheck)
	// Set strict mode before compilation
	if strict {
		p.compiler.SetStrictMode(true)
	}
	chunk, errs := p.compiler.Compile(program)
	// Note: Don't sync global names here - this is used for eval which may have
	// its own scope that shouldn't leak to the global scope (especially in strict mode)
	return chunk, errs
}

// CompileProgramAsScript compiles a parsed program explicitly as Script code (not Module)
// This is used by Function() constructor where import.meta must not be allowed
// even if the outer context is a module
func (p *Paserati) CompileProgramAsScript(program *parser.Program) (*vm.Chunk, []errors.PaseratiError) {
	// Honor session settings to ignore/skip type errors (used for Test262)
	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
	p.compiler.SetSkipTypeCheck(p.skipTypeCheck)
	// Force script mode to disallow import.meta
	p.compiler.SetForceScriptMode(true)
	chunk, errs := p.compiler.Compile(program)
	// Reset the flag after compilation
	p.compiler.SetForceScriptMode(false)
	// Sync global names to VM so Function constructor code can access globals
	p.SyncGlobalNamesFromCompiler()
	return chunk, errs
}

// EvalCode implements vm.EvalDriver interface for direct eval at global scope
// It compiles and executes eval code with the given strict mode inheritance
// This is used by OpDirectEval when there's no scope descriptor (global scope).
func (p *Paserati) EvalCode(code string, inheritStrict bool) (vm.Value, []error) {
	// Parse the source code
	lx := lexer.NewLexer(code)
	ps := parser.NewParser(lx)
	// Set strict mode before parsing so legacy octal etc. are rejected during parse
	if inheritStrict {
		ps.SetStrictMode(true)
	}
	prog, parseErrs := ps.ParseProgram()
	if len(parseErrs) > 0 {
		errs := make([]error, len(parseErrs))
		for i, e := range parseErrs {
			errs[i] = e
		}
		return vm.Undefined, errs
	}

	// Per ECMAScript spec, eval creates a new lexical environment for let/const/class.
	// Set indirect eval mode so let/const/class declarations stay local to the eval chunk
	// (only var declarations should be synced to the outer scope).
	p.compiler.SetIndirectEval(true)
	defer p.compiler.SetIndirectEval(false)

	// Compile with inherited strict mode
	chunk, compileErrs := p.CompileProgramWithStrictMode(prog, inheritStrict)
	if len(compileErrs) > 0 {
		errs := make([]error, len(compileErrs))
		for i, e := range compileErrs {
			errs[i] = e
		}
		return vm.Undefined, errs
	}

	if chunk == nil {
		return vm.Undefined, []error{fmt.Errorf("eval: compilation returned nil chunk")}
	}

	// Only sync global names for non-strict eval
	// In strict mode, eval creates its own variable environment and declarations stay local
	if !chunk.IsStrict {
		p.SyncGlobalNamesFromCompiler()
	}

	// Execute the chunk
	result, runtimeErrs := p.vmInstance.Interpret(chunk)
	if len(runtimeErrs) > 0 {
		errs := make([]error, len(runtimeErrs))
		for i, e := range runtimeErrs {
			errs[i] = e
		}
		return vm.Undefined, errs
	}

	return result, nil
}

// IndirectEvalCode compiles and executes indirect eval code
// Indirect eval creates a new declarative environment for let/const/class (not visible outside)
// while var declarations go to the global environment.
// Per ECMAScript spec, indirect eval does NOT inherit strict mode from caller.
func (p *Paserati) IndirectEvalCode(code string) (vm.Value, []error) {
	// Parse the source code
	// Per ECMAScript spec, indirect eval is always outside method context,
	// so super property access is always a SyntaxError.
	lx := lexer.NewLexer(code)
	ps := parser.NewParser(lx)
	ps.SetDisallowSuper(true)
	prog, parseErrs := ps.ParseProgram()
	if len(parseErrs) > 0 {
		errs := make([]error, len(parseErrs))
		for i, e := range parseErrs {
			errs[i] = e
		}
		return vm.Undefined, errs
	}

	// Set indirect eval mode - let/const stay local, var goes to global
	p.compiler.SetIndirectEval(true)
	defer p.compiler.SetIndirectEval(false)

	// Indirect eval does NOT inherit strict mode - only strict if code has "use strict"
	chunk, compileErrs := p.CompileProgramWithStrictMode(prog, false)
	if len(compileErrs) > 0 {
		errs := make([]error, len(compileErrs))
		for i, e := range compileErrs {
			errs[i] = e
		}
		return vm.Undefined, errs
	}

	if chunk == nil {
		return vm.Undefined, []error{fmt.Errorf("eval: compilation returned nil chunk")}
	}

	// Only sync global names for non-strict eval
	if !chunk.IsStrict {
		p.SyncGlobalNamesFromCompiler()
	}

	// Execute the chunk
	result, runtimeErrs := p.vmInstance.Interpret(chunk)
	if len(runtimeErrs) > 0 {
		errs := make([]error, len(runtimeErrs))
		for i, e := range runtimeErrs {
			errs[i] = e
		}
		return vm.Undefined, errs
	}

	// For non-strict indirect eval, sync heap vars to GlobalObject
	// This makes var declarations accessible via globalThis (ECMAScript requirement)
	if !chunk.IsStrict {
		p.vmInstance.SyncHeapToGlobalObject()
	}

	return result, nil
}

// DirectEvalCode implements vm.EvalDriver interface for direct eval with caller scope access
// This compiles and executes eval code with access to the caller's local variables, 'this', and homeObject.
func (p *Paserati) DirectEvalCode(code string, inheritStrict bool, scopeDesc *vm.ScopeDescriptor, callerRegs []vm.Value, callerThis vm.Value, callerHomeObject vm.Value) (vm.Value, []error) {
	// Parse the source code
	lx := lexer.NewLexer(code)
	ps := parser.NewParser(lx)
	// Set strict mode before parsing so legacy octal etc. are rejected during parse
	if inheritStrict {
		ps.SetStrictMode(true)
	}
	prog, parseErrs := ps.ParseProgram()
	if len(parseErrs) > 0 {
		errs := make([]error, len(parseErrs))
		for i, e := range parseErrs {
			errs[i] = e
		}
		return vm.Undefined, errs
	}

	// Per ECMAScript spec, eval creates a new lexical environment for let/const/class.
	// Set indirect eval mode so let/const/class declarations stay local to the eval chunk
	// (only var declarations should be synced to the outer scope).
	p.compiler.SetIndirectEval(true)
	defer p.compiler.SetIndirectEval(false)

	// Compile with inherited strict mode and caller scope info
	chunk, compileErrs := p.CompileDirectEvalCode(prog, inheritStrict, scopeDesc)
	if len(compileErrs) > 0 {
		errs := make([]error, len(compileErrs))
		for i, e := range compileErrs {
			errs[i] = e
		}
		return vm.Undefined, errs
	}

	if chunk == nil {
		return vm.Undefined, []error{fmt.Errorf("eval: compilation returned nil chunk")}
	}

	// Only sync global names for non-strict eval
	// In strict mode, eval creates its own variable environment and declarations stay local
	if !chunk.IsStrict {
		p.SyncGlobalNamesFromCompiler()
	}

	// Execute the chunk with caller scope access, inherited 'this', and homeObject for super property access
	result, runtimeErrs := p.vmInstance.InterpretWithCallerScope(chunk, callerRegs, callerThis, callerHomeObject)
	if len(runtimeErrs) > 0 {
		errs := make([]error, len(runtimeErrs))
		for i, e := range runtimeErrs {
			errs[i] = e
		}
		return vm.Undefined, errs
	}

	return result, nil
}

// CompileDirectEvalCode compiles eval code with access to caller's scope
func (p *Paserati) CompileDirectEvalCode(program *parser.Program, inheritStrict bool, scopeDesc *vm.ScopeDescriptor) (*vm.Chunk, []errors.PaseratiError) {
	// Set the caller's scope descriptor on the compiler
	p.compiler.SetCallerScopeDesc(scopeDesc)
	defer p.compiler.SetCallerScopeDesc(nil) // Clear after compilation

	// Use the existing strict mode compilation
	return p.CompileProgramWithStrictMode(program, inheritStrict)
}

// SyncGlobalNamesFromCompiler syncs the compiler's global name mappings to the VM
// This should be called after CompileProgram to ensure globalThis property access works
func (p *Paserati) SyncGlobalNamesFromCompiler() {
	if p.compiler == nil {
		return
	}
	heapAlloc := p.compiler.GetHeapAlloc()
	if heapAlloc == nil {
		return
	}
	nameMap := heapAlloc.GetNameToIndexMap()
	if debugDriver {
		fmt.Printf("[DEBUG SyncGlobalNames] Syncing %d names from compiler to VM\n", len(nameMap))
		hasArray := false
		for name := range nameMap {
			if name == "Array" {
				hasArray = true
				fmt.Printf("[DEBUG SyncGlobalNames]   Found 'Array' in name map at index %d\n", nameMap[name])
			}
		}
		if !hasArray {
			fmt.Printf("[DEBUG SyncGlobalNames]   WARNING: 'Array' NOT in name map!\n")
		}
	}
	p.vmInstance.SyncGlobalNames(nameMap)
}

// GetVM returns the VM instance for direct access (used by test framework)
func (p *Paserati) GetVM() *vm.VM {
	return p.vmInstance
}

// GetModuleLoader returns the session's module loader so a host (e.g. noderati)
// can inspect cache state or load modules without reaching into unexported fields.
func (p *Paserati) GetModuleLoader() modules.ModuleLoader {
	return p.moduleLoader
}

// AddResolver appends a module resolver to the loader chain and re-sorts by
// Priority() (lower = earlier). Native modules are -100; the filesystem resolver
// is 100. A Node node_modules resolver should sit in between (e.g. 0).
func (p *Paserati) AddResolver(resolver modules.ModuleResolver) {
	if p.moduleLoader == nil || resolver == nil {
		return
	}
	p.moduleLoader.AddResolver(resolver)
}

// PreloadAllNativeModules loads every declared native module and registers
// its exports on the shared heap so nested ESM files can import them.
func (p *Paserati) PreloadAllNativeModules() error {
	if p.nativeResolver == nil {
		return nil
	}
	seen := make(map[*NativeModule]bool)
	for name, mod := range p.nativeResolver.modules {
		if seen[mod] {
			continue
		}
		seen[mod] = true
		rec, err := p.LoadModule(name, ".")
		if err != nil {
			return err
		}
		if concrete, ok := rec.(*modules.ModuleRecord); ok {
			p.registerNativeModuleExports(concrete)
		}
	}
	return nil
}

// CompileModule compiles a module file with proper dependency resolution
// This is used by the test framework to compile modules with full module loading
func (p *Paserati) CompileModule(filename string) (*vm.Chunk, []errors.PaseratiError) {
	// Load the module using the module system to ensure dependencies are resolved
	moduleRecordInterface, err := p.moduleLoader.LoadModule(filename, ".")
	if err != nil {
		loadErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to load module '%s': %s", filename, err.Error()),
		}
		return nil, []errors.PaseratiError{loadErr}
	}

	// Extract the module record
	moduleRecord, ok := moduleRecordInterface.(*modules.ModuleRecord)
	if !ok {
		typeErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module loader returned unexpected type for '%s'", filename),
		}
		return nil, []errors.PaseratiError{typeErr}
	}

	if moduleRecord.Error != nil {
		compileErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module error: %s", moduleRecord.Error.Error()),
		}
		return nil, []errors.PaseratiError{compileErr}
	}

	// Register native module exports with HeapAlloc before compilation
	if moduleRecord.IsNativeModule() {
		p.registerNativeModuleExports(moduleRecord)
	}

	// Enable module mode in the checker and compiler for this specific module
	p.checker.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)
	p.compiler.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)

	// Compile the module
	// Honor session settings to ignore/skip type errors (used for Test262)
	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
	p.compiler.SetSkipTypeCheck(p.skipTypeCheck)
	chunk, compileErrs := p.compiler.Compile(moduleRecord.AST)
	if len(compileErrs) > 0 {
		return nil, compileErrs
	}

	return chunk, nil
}

// RunString compiles and executes the given source code in the current session.
// It uses the persistent type checker environment.
// Returns the result value and any errors that occurred.
// RunString executes Paserati source code in module mode.
// All code is executed as a module, which means:
// - import statements work
// - export statements work
// - Top-level variables don't pollute global scope (they're module-scoped)
// - Simple scripts still work transparently
//
// This is the new default behavior - module mode everywhere.
func (p *Paserati) RunString(sourceCode string) (vm.Value, []errors.PaseratiError) {
	// Parse the source code
	sourceFile := source.NewEvalSource(sourceCode)
	l := lexer.NewLexerWithSource(sourceFile)
	parseInstance := parser.NewParser(l)
	program, parseErrs := parseInstance.ParseProgram()
	if len(parseErrs) > 0 {
		return vm.Undefined, parseErrs
	}

	// Always run in module mode (module-first design)
	return p.runAsModule(sourceCode, program, "__eval_module__")
}

// DisplayResult formats and prints the result value and any errors.
// Returns true if execution completed without any errors, false otherwise.
func (p *Paserati) DisplayResult(sourceCode string, value vm.Value, errs []errors.PaseratiError) bool {
	if len(errs) > 0 {
		errors.DisplayErrors(errs, sourceCode)
		return false
	}

	// Only print non-undefined results in REPL-like contexts
	if value != vm.Undefined {
		fmt.Println(value.Inspect())
	}
	return true
}

// CompileString takes Paserati source code as a string, compiles it,
// and returns the resulting VM chunk or an aggregated list of Paserati errors.
// This version does NOT use a persistent session.
func CompileString(sourceCode string) (*vm.Chunk, []errors.PaseratiError) {
	sourceFile := source.NewEvalSource(sourceCode)
	l := lexer.NewLexerWithSource(sourceFile)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return nil, parseErrs
	}

	// Dump AST if enabled
	parser.DumpAST(program, "CompileString")

	// --- Type Check is handled internally by Compile when no checker is set ---
	// No need to call checker.Check() here.

	// A chunk's global indices are assigned by the compiler that produced it, so
	// they are only meaningful against a VM whose globals were numbered the same
	// way. A bare compiler.NewCompiler() has never seen the builtins, so it starts
	// numbering user globals at 0 - straight over the slots NewPaserati() gives to
	// Array, Map and friends. Compiling through an initialised instance numbers
	// them behind the standard builtins. CompileProgram records that layout on the
	// chunk so InterpretChunk can import compatible user mappings and reject a VM
	// with colliding custom builtins before execution (#149).
	chunk, compileAndTypeErrs := NewPaserati().CompileProgram(program)
	if len(compileAndTypeErrs) > 0 {
		return nil, compileAndTypeErrs
	}

	return chunk, nil
}

// CompileFile reads a file, compiles its content, and returns the
// resulting VM chunk or an aggregated list of Paserati errors.
// This version does NOT use a persistent session.
func CompileFile(filename string) (*vm.Chunk, []errors.PaseratiError) {
	sourceBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		readErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to read file '%s': %s", filename, err.Error()),
		}
		return nil, []errors.PaseratiError{readErr}
	}
	sourceCode := string(sourceBytes)
	sourceFile := source.FromFile(filename, sourceCode)
	l := lexer.NewLexerWithSource(sourceFile)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return nil, parseErrs
	}

	// Dump AST if enabled
	parser.DumpAST(program, "CompileFile")

	// A chunk's global indices are assigned by the compiler that produced it, so
	// they are only meaningful against a VM whose globals were numbered the same
	// way. A bare compiler.NewCompiler() has never seen the builtins, so it starts
	// numbering user globals at 0 - straight over the slots NewPaserati() gives to
	// Array, Map and friends. Compiling through an initialised instance numbers
	// them behind the standard builtins. CompileProgram records that layout on the
	// chunk so InterpretChunk can import compatible user mappings and reject a VM
	// with colliding custom builtins before execution (#149).
	chunk, compileAndTypeErrs := NewPaserati().CompileProgram(program)
	if len(compileAndTypeErrs) > 0 {
		return nil, compileAndTypeErrs
	}

	return chunk, nil
}

// RunString compiles and interprets Paserati source code from a string.
// It prints any errors encountered (syntax, compile, runtime) and the
// final result if execution is successful.
// Returns true if execution completed without any errors, false otherwise.
// This version creates a fresh Paserati session.
func RunString(source string) bool {
	return RunStringWithOptions(source, RunOptions{})
}

// RunStringWithOptions is like RunString but accepts options for debugging output
func RunStringWithOptions(source string, options RunOptions) bool {
	// Create a new Paserati session to handle builtin initialization properly
	paserati := NewPaserati()

	// Run the code using the session
	value, errs := paserati.RunCode(source, options)

	// Display the result
	return paserati.DisplayResult(source, value, errs)
}

// RunFile reads, compiles, and interprets a Paserati source file.
// Always uses module mode - if no imports/exports are present, it works like regular mode.
// Returns true if execution completed without any errors, false otherwise.
func RunFile(filename string) bool {
	// Create a new Paserati session
	paserati := NewPaserati()

	// Convert file path to module specifier
	// Check if file exists first
	if _, err := os.Stat(filename); err != nil {
		fmt.Fprintf(os.Stderr, "File not found: %s\n", filename)
		return false
	}

	// Convert to module specifier
	// If it doesn't start with ./ or ../ or /, add ./ prefix
	moduleSpecifier := filename
	if !strings.HasPrefix(filename, "./") && !strings.HasPrefix(filename, "../") && !strings.HasPrefix(filename, "/") {
		moduleSpecifier = "./" + filename
	}

	// Always use module mode - it gracefully handles both module and non-module files
	return paserati.RunModule(moduleSpecifier)
}

// LoadModule loads a module and all its dependencies using the module system.
// This enables cross-module type checking and proper import/export resolution.
func (p *Paserati) LoadModule(specifier string, fromPath string) (vm.ModuleRecord, error) {
	return p.moduleLoader.LoadModule(specifier, fromPath)
}

// RunModule loads and executes a module file with full module system support.
// Unlike RunFile, this enables import/export statements and cross-module type checking.
func (p *Paserati) RunModule(filename string) bool {
	// Load the module using the module system
	// Use sequential loading for now until parallel processing is fully debugged
	moduleRecordInterface, err := p.moduleLoader.LoadModule(filename, ".")
	if err != nil {
		loadErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to load module '%s': %s", filename, err.Error()),
		}
		fmt.Fprintf(os.Stderr, "%s Error: %s\n", loadErr.Kind(), loadErr.Message())
		return false
	}

	// Check if module loaded successfully
	if moduleRecordInterface == nil {
		moduleErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module '%s' was not loaded", filename),
		}
		fmt.Fprintf(os.Stderr, "%s Error: %s\n", moduleErr.Kind(), moduleErr.Message())
		return false
	}

	// Type assert to get access to the concrete ModuleRecord fields
	moduleRecord, ok := moduleRecordInterface.(*modules.ModuleRecord)
	if !ok {
		moduleErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module '%s' has invalid type", filename),
		}
		fmt.Fprintf(os.Stderr, "%s Error: %s\n", moduleErr.Kind(), moduleErr.Message())
		return false
	}

	if moduleRecord.Error != nil {
		moduleErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module error in '%s': %s", filename, moduleRecord.Error.Error()),
		}
		fmt.Fprintf(os.Stderr, "%s Error: %s\n", moduleErr.Kind(), moduleErr.Message())
		return false
	}

	// Check if AST is available
	if moduleRecord.AST == nil {
		moduleErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module '%s' has no AST (possibly not parsed)", filename),
		}
		fmt.Fprintf(os.Stderr, "%s Error: %s\n", moduleErr.Kind(), moduleErr.Message())
		return false
	}

	// Check if module already has a compiled chunk from the loader
	var chunk *vm.Chunk
	if moduleRecord.CompiledChunk != nil {
		// Module was already compiled by the loader, use that chunk
		chunk = moduleRecord.CompiledChunk
		debugPrintf("// [Driver] Using pre-compiled chunk for module '%s'\n", filename)
	} else {
		// Module needs compilation (shouldn't happen with current loader, but handle it)
		debugPrintf("// [Driver] Module '%s' needs compilation\n", filename)

		// Enable module mode in the checker and compiler
		p.checker.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)
		p.compiler.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)

		// Compile the module
		var compileErrs []errors.PaseratiError
		chunk, compileErrs = p.compiler.Compile(moduleRecord.AST)
		if len(compileErrs) > 0 {
			// Read source for error display
			sourceCode := ""
			if moduleRecord.Source != nil {
				sourceCode = moduleRecord.Source.Content
			}
			return p.DisplayResult(sourceCode, vm.Undefined, compileErrs)
		}

		if chunk == nil {
			internalErr := &errors.RuntimeError{
				Position: errors.Position{Line: 0, Column: 0},
				Msg:      "Internal Error: Compilation returned nil chunk without errors.",
			}
			return p.DisplayResult("", vm.Undefined, []errors.PaseratiError{internalErr})
		}

		// Store the compiled chunk in the module record for VM access
		moduleRecord.CompiledChunk = chunk

		// This compile (unlike the loader's own, in the common case above)
		// happened on p.compiler right here, so its export indices/re-exports
		// are trustworthy - record them on moduleRecord now, the same way the
		// loader's applyCompilerExports does, so export collection below can
		// rely solely on moduleRecord regardless of which branch ran.
		exportGlobalIndices := p.compiler.GetExportGlobalIndices()
		exportIndices := make(map[string]uint16, len(exportGlobalIndices))
		for name, idx := range exportGlobalIndices {
			exportIndices[name] = uint16(idx)
		}
		moduleRecord.ExportIndices = exportIndices
		moduleRecord.ReExports = p.compiler.GetReExports()
	}

	// Set the module path in the VM so import.meta.url works correctly
	p.vmInstance.SetCurrentModulePath(moduleRecord.ResolvedPath)

	// Execute the module
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)
	if len(runtimeErrs) > 0 {
		// Get source code for error display
		sourceCode := ""
		if moduleRecord.Source != nil {
			sourceCode = moduleRecord.Source.Content
		}
		return p.DisplayResult(sourceCode, finalValue, runtimeErrs)
	}

	// After successful execution, collect exported values using this
	// module's own recorded export indices (see collectExportedValuesForModule -
	// paserati#165: p.compiler's *current* IsModuleMode()/export indices are
	// not reliable here, since this call may be reentrant).
	exportedValues := p.collectExportedValuesForModule(moduleRecord)
	moduleRecord.ExportValues = exportedValues
	debugPrintf("// [Driver] Collected %d exported values from module\n", len(exportedValues))

	// Get source code for error display
	sourceCode := ""
	if moduleRecord.Source != nil {
		sourceCode = moduleRecord.Source.Content
	}

	return p.DisplayResult(sourceCode, finalValue, runtimeErrs)
}

// RunModuleWithValue loads and executes a module file with full module system support
// and returns the final value along with any errors. This combines the functionality
// of RunModule with the value return capability of RunCode.
func (p *Paserati) RunModuleWithValue(filename string) (vm.Value, []errors.PaseratiError, []errors.PaseratiError) {
	// Load the module using the module system
	moduleRecordInterface, err := p.moduleLoader.LoadModule(filename, ".")
	if err != nil {
		loadErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to load module '%s': %s", filename, err.Error()),
		}
		return vm.Undefined, []errors.PaseratiError{loadErr}, nil
	}

	// Check if module loaded successfully
	if moduleRecordInterface == nil {
		moduleErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module '%s' was not loaded", filename),
		}
		return vm.Undefined, []errors.PaseratiError{moduleErr}, nil
	}

	// Type assert to get access to the concrete ModuleRecord fields
	moduleRecord, ok := moduleRecordInterface.(*modules.ModuleRecord)
	if !ok {
		moduleErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module '%s' has invalid type", filename),
		}
		return vm.Undefined, []errors.PaseratiError{moduleErr}, nil
	}

	if moduleRecord.Error != nil {
		moduleErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module error in '%s': %s", filename, moduleRecord.Error.Error()),
		}
		return vm.Undefined, []errors.PaseratiError{moduleErr}, nil
	}

	// Check if AST is available
	if moduleRecord.AST == nil {
		moduleErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Module '%s' has no AST (possibly not parsed)", filename),
		}
		return vm.Undefined, []errors.PaseratiError{moduleErr}, nil
	}

	// Register native module exports with HeapAlloc before any processing
	if moduleRecord.IsNativeModule() {
		p.registerNativeModuleExports(moduleRecord)
	}

	// Check if module already has a compiled chunk from the loader
	var chunk *vm.Chunk
	if moduleRecord.CompiledChunk != nil {
		// Module was already compiled by the loader, use that chunk
		chunk = moduleRecord.CompiledChunk
		debugPrintf("// [Driver] Using pre-compiled chunk for module '%s'\n", filename)
	} else {
		// Module needs compilation (shouldn't happen with current loader, but handle it)
		debugPrintf("// [Driver] Module '%s' needs compilation\n", filename)

		// Enable module mode in the checker and compiler
		p.checker.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)
		p.compiler.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)

		// Compile the module
		// Set the compiler's ignore/skip type errors flags based on our setting
		p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
		p.compiler.SetSkipTypeCheck(p.skipTypeCheck)

		var compileErrs []errors.PaseratiError
		chunk, compileErrs = p.compiler.Compile(moduleRecord.AST)
		if len(compileErrs) > 0 {
			return vm.Undefined, compileErrs, nil
		}

		if chunk == nil {
			internalErr := &errors.RuntimeError{
				Position: errors.Position{Line: 0, Column: 0},
				Msg:      "Internal Error: Compilation returned nil chunk without errors.",
			}
			return vm.Undefined, []errors.PaseratiError{internalErr}, nil
		}

		// Store the compiled chunk in the module record for VM access
		moduleRecord.CompiledChunk = chunk

		// This compile (unlike the loader's own, in the common case above)
		// happened on p.compiler right here, so its export indices/re-exports
		// are trustworthy - record them on moduleRecord now, the same way the
		// loader's applyCompilerExports does, so export collection below can
		// rely solely on moduleRecord regardless of which branch ran.
		exportGlobalIndices := p.compiler.GetExportGlobalIndices()
		exportIndices := make(map[string]uint16, len(exportGlobalIndices))
		for name, idx := range exportGlobalIndices {
			exportIndices[name] = uint16(idx)
		}
		moduleRecord.ExportIndices = exportIndices
		moduleRecord.ReExports = p.compiler.GetReExports()
	}

	// Set the module path in the VM so import.meta.url works correctly
	p.vmInstance.SetCurrentModulePath(moduleRecord.ResolvedPath)

	// Execute the module and return the final value
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)

	// After successful execution, collect exported values using this
	// module's own recorded export indices (see collectExportedValuesForModule -
	// paserati#165: p.compiler's *current* IsModuleMode()/export indices are
	// not reliable here, since this call may be reentrant, e.g. reached via a
	// require() partway through executing an unrelated entry script).
	exportedValues := p.collectExportedValuesForModule(moduleRecord)
	moduleRecord.ExportValues = exportedValues
	debugPrintf("// [Driver] Collected %d exported values from module\n", len(exportedValues))

	return finalValue, []errors.PaseratiError{}, runtimeErrs
}

// RunStringWithModules runs TypeScript code that may contain import statements.
// If imports are detected, it automatically enables module mode.
// If no imports are found, it falls back to script mode like RunString.
// RunStringWithModules executes Paserati source code in module mode.
// DEPRECATED: Use RunString instead - all code now runs in module mode by default.
// This method is kept for backward compatibility and simply calls RunString.
func (p *Paserati) RunStringWithModules(sourceCode string) (vm.Value, []errors.PaseratiError) {
	return p.RunString(sourceCode)
}

// containsImports checks if a program contains any import statements
func containsImports(program *parser.Program) bool {
	for _, stmt := range program.Statements {
		if _, isImport := stmt.(*parser.ImportDeclaration); isImport {
			return true
		}
	}
	return false
}

// runAsModule runs code as a module with the given module name
// This is the unified path for all module execution
func (p *Paserati) runAsModule(sourceCode string, program *parser.Program, moduleName string) (vm.Value, []errors.PaseratiError) {
	// Preload all native modules that might be imported
	// This ensures their exports are registered with HeapAlloc before compilation
	if err := p.preloadNativeModules(program); err != nil {
		return vm.Undefined, []errors.PaseratiError{err}
	}

	// Enable module mode in checker and compiler
	p.checker.EnableModuleMode(moduleName, p.moduleLoader)
	p.compiler.EnableModuleMode(moduleName, p.moduleLoader)

	// Dump AST if enabled
	parser.DumpAST(program, "runAsModule")

	// Compile with module mode enabled
	// Set the compiler's ignore/skip type errors flags based on our setting
	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
	p.compiler.SetSkipTypeCheck(p.skipTypeCheck)

	chunk, compileAndTypeErrs := p.compiler.Compile(program)
	if len(compileAndTypeErrs) > 0 {
		return vm.Undefined, compileAndTypeErrs
	}
	if chunk == nil {
		internalErr := &errors.RuntimeError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      "Internal Error: Compilation returned nil chunk without errors.",
		}
		return vm.Undefined, []errors.PaseratiError{internalErr}
	}

	// Sync global names from compiler to VM heap so globalThis property access works
	p.vmInstance.SyncGlobalNames(p.compiler.GetHeapAlloc().GetNameToIndexMap())

	// Resize heap to accommodate all global indices assigned during compilation
	// This ensures that OpGetGlobal can properly detect uninitialized variables
	p.vmInstance.ResizeHeapForGlobals(p.compiler.GetHeapAlloc().GetAllocatedSize())

	// Set the module path in the VM so import.meta.url works correctly
	p.vmInstance.SetCurrentModulePath(moduleName)

	// Register this module as executing *before* running it, so that a dependency
	// which circularly imports or re-exports back from this same path (most commonly:
	// the entry module referenced by one of its own dependencies) recognizes it as
	// already in flight instead of triggering an independent, divergent reload+re-run
	// of the same source. See VM.RegisterExecutingModule.
	p.vmInstance.RegisterExecutingModule(moduleName, chunk)

	// Execute the chunk
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)

	// Drain async work (microtasks, timers, etc.) until idle
	p.vmInstance.DrainUntilIdle()

	// Record the final exported values so a circular reference that read this module
	// mid-execution (and got Undefined for anything not yet initialized) is corrected
	// for any subsequent read, and so a later, separate call resolves correctly too.
	if p.compiler.IsModuleMode() {
		p.vmInstance.FinishExecutingModule(moduleName, p.collectExportedValues(moduleName))
	}

	return finalValue, runtimeErrs
}

// runAsTemporaryModule runs code with imports as a temporary module
// DEPRECATED: Use runAsModule instead
func (p *Paserati) runAsTemporaryModule(sourceCode string, program *parser.Program) (vm.Value, []errors.PaseratiError) {
	return p.runAsModule(sourceCode, program, "__temp_module__")
}

// runAsScript compiles and runs a program as a Script (not an ESM module).
// Used for CommonJS wrapping: the host wraps the file body in a function and
// calls it with (exports, require, module, __filename, __dirname).
func (p *Paserati) runAsScript(program *parser.Program, filename string) (vm.Value, []errors.PaseratiError) {
	p.checker.DisableModuleMode()
	p.compiler.DisableModuleMode()

	parser.DumpAST(program, "runAsScript")

	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
	p.compiler.SetSkipTypeCheck(p.skipTypeCheck)
	p.compiler.SetForceScriptMode(true)
	chunk, compileAndTypeErrs := p.compiler.Compile(program)
	p.compiler.SetForceScriptMode(false)
	if len(compileAndTypeErrs) > 0 {
		return vm.Undefined, compileAndTypeErrs
	}
	if chunk == nil {
		internalErr := &errors.RuntimeError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      "Internal Error: Compilation returned nil chunk without errors.",
		}
		return vm.Undefined, []errors.PaseratiError{internalErr}
	}

	p.vmInstance.SyncGlobalNames(p.compiler.GetHeapAlloc().GetNameToIndexMap())
	p.vmInstance.ResizeHeapForGlobals(p.compiler.GetHeapAlloc().GetAllocatedSize())
	p.vmInstance.SetCurrentModulePath(filename)

	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)
	p.vmInstance.DrainUntilIdle()
	return finalValue, runtimeErrs
}

// RunScript compiles source as a Script (not ESM) and runs it with filename as
// the current path. It does not create a ModuleRecord or ESM exports.
// A Node host typically wraps CommonJS as:
//
//	(function (exports, require, module, __filename, __dirname) { /* body */ })
//
// then Call()s the returned function with those arguments. filename is the
// value noderati should pass as __filename (and from which it derives __dirname).
func (p *Paserati) RunScript(sourceCode, filename string) (vm.Value, []errors.PaseratiError) {
	return p.RunCode(sourceCode, RunOptions{Script: true, Filename: filename})
}

// EmitJavaScript parses TypeScript source and emits equivalent JavaScript code
// without type annotations and TypeScript-specific syntax.
func EmitJavaScript(sourceCode string) (string, []errors.PaseratiError) {
	sourceFile := source.NewEvalSource(sourceCode)
	l := lexer.NewLexerWithSource(sourceFile)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return "", parseErrs
	}

	// Create JavaScript emitter and emit JS code
	emitter := parser.NewJSEmitter()
	jsCode := emitter.Emit(program)

	return jsCode, nil
}

// EmitJavaScriptFile reads a TypeScript file and emits equivalent JavaScript code.
// It returns the JavaScript code as a string or an error list.
func EmitJavaScriptFile(filename string) (string, []errors.PaseratiError) {
	sourceBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		readErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to read file '%s': %s", filename, err.Error()),
		}
		return "", []errors.PaseratiError{readErr}
	}
	sourceCode := string(sourceBytes)
	sourceFile := source.FromFile(filename, sourceCode)
	l := lexer.NewLexerWithSource(sourceFile)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return "", parseErrs
	}

	// Create JavaScript emitter and emit JS code
	emitter := parser.NewJSEmitter()
	jsCode := emitter.Emit(program)

	return jsCode, nil
}

// WriteJavaScriptFile reads a TypeScript file, converts it to JavaScript,
// and writes the output to a file with a .js extension.
// Returns true if successful, false otherwise.
func WriteJavaScriptFile(inputFilename string, outputFilename string) bool {
	if outputFilename == "" {
		// Default to replacing .ts with .js
		outputFilename = inputFilename
		if len(outputFilename) > 3 && outputFilename[len(outputFilename)-3:] == ".ts" {
			outputFilename = outputFilename[:len(outputFilename)-3] + ".js"
		} else {
			outputFilename = outputFilename + ".js"
		}
	}

	jsCode, errs := EmitJavaScriptFile(inputFilename)
	if len(errs) > 0 {
		// Print errors
		errors.DisplayErrors(errs)
		return false
	}

	// Write JavaScript code to the output file
	err := ioutil.WriteFile(outputFilename, []byte(jsCode), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing JavaScript file: %s\n", err)
		return false
	}

	fmt.Printf("JavaScript code written to %s\n", outputFilename)
	return true
}

// RunOptions configures optional debugging output
type RunOptions struct {
	ShowTokens     bool
	ShowAST        bool
	ShowBytecode   bool
	ShowCacheStats bool   // Show inline cache statistics
	ModuleName     string // Module name to use (defaults to "__code_module__" if empty)
	DisasmFilter   string // Filter string for disassembly (empty = all)
	Script         bool   // Compile and run as a Script, not an ESM module
	Filename       string // Current path for Script mode (and error reporting)
}

func scriptFilename(options RunOptions) string {
	if options.Filename != "" {
		return options.Filename
	}
	if options.ModuleName != "" {
		return options.ModuleName
	}
	return "<script>"
}

// RunCode runs source code with the given Paserati session and options.
// Module mode is the default; set Script to run as a Script (see RunScript).
func (p *Paserati) RunCode(sourceCode string, options RunOptions) (vm.Value, []errors.PaseratiError) {
	sourceFile := source.NewEvalSource(sourceCode)
	// Name the source after the file it came from whenever the caller told us
	// one, in module mode as well as Script mode. Errors raised while running
	// it now carry this SourceFile (#148), so leaving it as "<eval>" would
	// label a file the user ran by name with a placeholder. Callers that
	// genuinely have no file (the REPL, -e) set neither field and keep
	// "<eval>"; ModuleName alone is not enough, since it is often a synthetic
	// specifier rather than a path.
	if filename := options.Filename; filename != "" {
		sourceFile = source.NewSourceFile(filepath.Base(filename), filename, sourceCode)
	} else if options.Script {
		filename := scriptFilename(options)
		sourceFile = source.NewSourceFile(filepath.Base(filename), filename, sourceCode)
	}
	l := lexer.NewLexerWithSource(sourceFile)
	parseInstance := parser.NewParser(l)
	program, parseErrs := parseInstance.ParseProgram()
	if len(parseErrs) > 0 {
		return vm.Undefined, parseErrs
	}

	var value vm.Value
	var errs []errors.PaseratiError
	if options.Script {
		value, errs = p.runAsScript(program, scriptFilename(options))
	} else {
		moduleName := options.ModuleName
		if moduleName == "" {
			moduleName = "__code_module__"
		}
		value, errs = p.runAsModule(sourceCode, program, moduleName)
	}

	// Get the compiled chunk for debugging output if needed
	if options.ShowBytecode || options.ShowCacheStats {
		// Re-compile to get chunk for display (the runAsModule already executed it)
		// This is a bit wasteful but only happens when debugging flags are on
		p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
		p.compiler.SetSkipTypeCheck(p.skipTypeCheck)
		chunk, _ := p.compiler.Compile(program)

		if chunk != nil {
			// Show bytecode if requested
			if options.ShowBytecode {
				fmt.Println("\n=== Bytecode ===")
				// Use the filter from options
				fmt.Print(chunk.DisassembleChunkFiltered("<module>", options.DisasmFilter))
				fmt.Println("================")
			}
		}

		// Show cache statistics if requested
		if options.ShowCacheStats {
			fmt.Println("\n=== Inline Cache Statistics === ")
			p.vmInstance.PrintCacheStats()
			fmt.Println("===============================")
		}
	}

	return value, errs
}

// GetCacheStats returns extended cache statistics from the VM instance
func (p *Paserati) GetCacheStats() vm.ExtendedCacheStats {
	return vm.GetExtendedStatsFromVM(p.vmInstance)
}

// InterpretChunk executes a compiled chunk on the VM instance with initialized builtins
func (p *Paserati) InterpretChunk(chunk *vm.Chunk) (vm.Value, []errors.PaseratiError) {
	if err := p.prepareChunkGlobalLayout(chunk); err != nil {
		return vm.Undefined, []errors.PaseratiError{err}
	}
	return p.vmInstance.Interpret(chunk)
}

// prepareChunkGlobalLayout verifies that the target session assigns every
// compile-time builtin and global name to the same index as the chunk. New user
// names can be imported into an unused target slot; an occupied or differently
// indexed slot is rejected before any bytecode runs.
func (p *Paserati) prepareChunkGlobalLayout(chunk *vm.Chunk) errors.PaseratiError {
	if chunk == nil || len(chunk.GlobalNames) == 0 || p.heapAlloc == nil {
		return nil
	}
	if p.preparedChunk == chunk {
		return nil
	}

	targetNames := p.heapAlloc.GetNameToIndexMap()
	targetByIndex := make(map[int]string, len(targetNames))
	for name, idx := range targetNames {
		targetByIndex[idx] = name
	}

	for idx, name := range chunk.BuiltinGlobalNames {
		if name == "" {
			continue
		}
		targetIdx, exists := targetNames[name]
		if !exists || targetIdx != idx {
			return incompatibleChunkGlobalLayout(idx, name, targetByIndex[idx])
		}
	}

	for idx, name := range chunk.GlobalNames {
		if name == "" {
			continue
		}
		if targetIdx, exists := targetNames[name]; exists {
			if targetIdx != idx {
				return incompatibleChunkGlobalLayout(idx, name, targetByIndex[idx])
			}
			continue
		}
		if targetName, occupied := targetByIndex[idx]; occupied {
			return incompatibleChunkGlobalLayout(idx, name, targetName)
		}
	}

	for idx, name := range chunk.GlobalNames {
		if name == "" {
			continue
		}
		if _, exists := targetNames[name]; !exists {
			p.heapAlloc.SetIndex(name, idx)
		}
	}
	p.SyncGlobalNamesFromCompiler()
	p.preparedChunk = chunk
	return nil
}

func incompatibleChunkGlobalLayout(index int, chunkName, targetName string) errors.PaseratiError {
	if targetName == "" {
		targetName = "<unassigned>"
	}
	return &errors.RuntimeError{
		Position: errors.Position{Line: 0, Column: 0},
		Msg: fmt.Sprintf(
			"cannot interpret chunk: incompatible global layout at index %d (chunk=%q, VM=%q)",
			index, chunkName, targetName,
		),
	}
}

func indexedGlobalNames(nameToIndex map[string]int) []string {
	maxIndex := -1
	for _, idx := range nameToIndex {
		if idx > maxIndex {
			maxIndex = idx
		}
	}
	if maxIndex < 0 {
		return nil
	}
	names := make([]string, maxIndex+1)
	for name, idx := range nameToIndex {
		names[idx] = name
	}
	return names
}

// initializeBuiltins sets up all builtin global variables in both the compiler and VM
// ensuring they use the same global index ordering via the unified heap allocator
func initializeBuiltins(paserati *Paserati) error {
	return initializeBuiltinsWithCustom(paserati, builtins.GetStandardInitializers())
}

// initializeBuiltinsWithCustom sets up builtin global variables using custom initializers
func initializeBuiltinsWithCustom(paserati *Paserati, initializers []builtins.BuiltinInitializer) error {
	vmInstance := paserati.vmInstance
	comp := paserati.compiler
	heapAlloc := paserati.heapAlloc

	// Create runtime context for VM initialization
	globalVariables := make(map[string]vm.Value)

	// Track which initializer defined which global to separate standard vs custom
	// Build a set of standard initializer names for lookup
	standardInitSet := make(map[string]bool)
	for _, init := range builtins.GetStandardInitializers() {
		standardInitSet[init.Name()] = true
	}

	// Track globals defined by each initializer during the SINGLE initialization pass
	globalsPerInitializer := make(map[string][]string)
	currentInitializer := ""

	runtimeCtx := &builtins.RuntimeContext{
		VM:     vmInstance,
		Driver: paserati, // Pass driver for Function constructor
		DefineGlobal: func(name string, value vm.Value) error {
			globalVariables[name] = value
			// Track which initializer defined this global
			if currentInitializer != "" {
				globalsPerInitializer[currentInitializer] = append(globalsPerInitializer[currentInitializer], name)
			}
			return nil
		},
	}

	// Initialize all builtins runtime values ONCE
	for _, init := range initializers {
		currentInitializer = init.Name()
		if err := init.InitRuntime(runtimeCtx); err != nil {
			return fmt.Errorf("failed to initialize %s runtime: %v", init.Name(), err)
		}
	}

	// vmInstance.GlobalObject was created (in realm.InitializePrototypes) before
	// this loop ran, chained to the placeholder Object.prototype that existed at
	// that time. ObjectInitializer.InitRuntime above replaces vmInstance.ObjectPrototype
	// wholesale with a brand-new object carrying the real methods (hasOwnProperty,
	// toString, valueOf, ...) - GlobalObject's prototype link still points at the
	// orphaned placeholder unless re-pointed here. Without this, bare references to
	// Object.prototype methods (which fall back to a lookup on GlobalObject) report
	// as absent even though `globalThis.hasOwnProperty` correctly resolves via a
	// separate special case in property access. See #246.
	if vmInstance.GlobalObject != nil && vmInstance.ObjectPrototype.IsObject() {
		vmInstance.GlobalObject.SetPrototype(vmInstance.ObjectPrototype)
	}

	// Get builtin names and preallocate indices in the heap allocator
	// IMPORTANT: Separate standard builtins from custom ones to ensure stable indices
	// Standard builtins (from GetStandardInitializers) must have consistent indices
	// across all Paserati instances for bytecode compatibility
	var standardNames []string
	var customNames []string

	// Separate globals into standard vs custom based on which initializer defined them
	// IMPORTANT: Iterate over initializers in their original order to ensure stable heap indices
	for _, init := range initializers {
		globals := globalsPerInitializer[init.Name()]
		if standardInitSet[init.Name()] {
			standardNames = append(standardNames, globals...)
		} else {
			customNames = append(customNames, globals...)
		}
	}

	// Preallocate standard builtins first (indices 0-N)
	heapAlloc.PreallocateBuiltins(standardNames)
	// Then preallocate custom builtins (indices N+1 onwards)
	heapAlloc.PreallocateBuiltins(customNames)

	// Set the heap allocator in the main compiler
	comp.SetHeapAlloc(heapAlloc)

	// Set up global variables in VM using the coordinated indices
	indexMap := heapAlloc.GetNameToIndexMap()
	if err := vmInstance.SetBuiltinGlobals(globalVariables, indexMap); err != nil {
		return err
	}
	paserati.builtinGlobals = indexedGlobalNames(indexMap)

	return nil
}

// InitializeRealmBuiltins initializes builtins for a new realm.
// This allows creating additional realms with their own builtins for cross-realm testing.
func (p *Paserati) InitializeRealmBuiltins(realm *vm.Realm, initializers []builtins.BuiltinInitializer) error {
	vmInstance := p.vmInstance

	// Clone the main realm's heap layout so compiled bytecode indices match.
	// The new realm gets the same nameToIndex mapping but separate value storage.
	mainHeap := vmInstance.GetHeap()
	if mainHeap != nil {
		realm.Heap = mainHeap.CloneLayout()
	}

	// Temporarily switch to the new realm for initialization
	prevRealm := vmInstance.CurrentRealm()
	vmInstance.WithRealm(realm, func() {
		// Create runtime context for the new realm
		// Use the realm's prototypes for proper initialization
		runtimeCtx := &builtins.RuntimeContext{
			VM:                vmInstance,
			Driver:            p,
			ObjectPrototype:   realm.ObjectPrototype,
			FunctionPrototype: realm.FunctionPrototype,
			ArrayPrototype:    realm.ArrayPrototype,
			DefineGlobal: func(name string, value vm.Value) error {
				realm.SetGlobal(name, value)
				// Also set on realm's GlobalObject for property access
				if realm.GlobalObject != nil {
					realm.GlobalObject.SetOwn(name, value)
				}
				return nil
			},
		}

		// Initialize builtins in the new realm
		for _, init := range initializers {
			if err := init.InitRuntime(runtimeCtx); err != nil {
				// Log error but continue - some initializers may fail in secondary realms
				continue
			}
		}

		// realm.GlobalObject was created (in realm.InitializePrototypes) chained to
		// whatever placeholder Object.prototype existed at that time. The
		// ObjectInitializer.InitRuntime call above replaces vmInstance.ObjectPrototype
		// wholesale with a brand-new object carrying the real methods (hasOwnProperty,
		// toString, valueOf, ...) - GlobalObject's prototype link still points at the
		// orphaned placeholder unless re-pointed here, same root cause fixed for the
		// main realm above in initializeBuiltinsWithCustom. Do this once, here,
		// rather than in SyncPrototypesToRealm (called on every WithRealm exit),
		// which would silently re-force the link over a legitimate later
		// Object.setPrototypeOf(globalThis, ...) in this realm. See #246.
		if realm.GlobalObject != nil && vmInstance.ObjectPrototype.IsObject() {
			realm.GlobalObject.SetPrototype(vmInstance.ObjectPrototype)
		}
	})

	// Restore previous realm (already done by WithRealm defer)
	_ = prevRealm

	return nil
}

// collectExportedValues collects the runtime values of exported variables from the VM.
// This is called after successful module execution to populate the ModuleRecord.ExportValues.
//
// Unlike collectExportedValuesForModule, this trusts p.compiler's *current*
// module-mode state and export indices directly - safe only at a call site
// that just finished compiling and running this exact chunk on p.compiler
// itself (runAsModule, the sole remaining caller), so nothing reentrant can
// have touched it in between. fromResolvedPath is that module's own
// resolved path, needed to resolve any re-exports of an imported binding
// (see paserati#163) via VM.GetModuleExport.
func (p *Paserati) collectExportedValues(fromResolvedPath string) map[string]vm.Value {
	exports := make(map[string]vm.Value)

	// Debug disabled
	if !p.compiler.IsModuleMode() {
		return exports
	}

	// Get the export name to global index mapping from the compiler
	exportIndices := p.compiler.GetExportGlobalIndices()
	// Debug disabled

	// For each export, get the value directly from the VM's global table using the index
	for exportName, globalIdx := range exportIndices {
		if value, exists := p.vmInstance.GetGlobalByIndex(globalIdx); exists {
			exports[exportName] = value
			// Debug disabled
		} else {
			exports[exportName] = vm.Undefined
			// Debug disabled
		}
	}
	// Debug disabled

	// Re-exports (export { x } from "./mod", or export { X } where X was
	// itself an imported binding - see paserati#163) never occupy a local
	// global slot, so exportIndices above can't see them.
	for exportName, re := range p.compiler.GetReExports() {
		if _, already := exports[exportName]; already {
			continue
		}
		exports[exportName] = p.vmInstance.GetModuleExport(fromResolvedPath, re.SourceModule, re.SourceName)
	}

	return exports
}

// collectExportedValuesForModule collects the runtime values of a specific
// module's exports directly from moduleRecord.ExportIndices, rather than
// asking the shared p.compiler what its *current* module-mode state and
// export indices happen to be.
//
// p.compiler is a single stateful instance whose module-mode flag and export
// indices reflect whichever compile last ran on it. RunModule/RunModuleWithValue
// don't always do that compile themselves - most of the time moduleRecord
// already arrives pre-compiled by the module loader's own per-module compiler
// (see modules/loader.go's applyCompilerExports, which is what actually
// populated moduleRecord.ExportIndices/ReExports at compile time). A call
// reached reentrantly - e.g. a require() handled partway through executing
// the entry script - can find p.compiler's mode flag flipped back to
// non-module by whatever compiled in between, silently skipping export
// collection with no error. See paserati#165.
func (p *Paserati) collectExportedValuesForModule(moduleRecord *modules.ModuleRecord) map[string]vm.Value {
	// A native module's real exports are populated directly by
	// handleNativeModuleSource at load time, not by compiling/running a
	// chunk - it has no ExportIndices/ReExports of its own to derive
	// anything from (RunModule/RunModuleWithValue still compile+run its
	// placeholder empty AST for such a module, since nothing short-circuits
	// that path). Recomputing from those empty maps would produce an empty
	// result and silently wipe out the real exports already on the record.
	if moduleRecord.IsNativeModule() {
		return moduleRecord.GetExportValues()
	}

	exports := make(map[string]vm.Value)
	for exportName, globalIdx := range moduleRecord.ExportIndices {
		if value, exists := p.vmInstance.GetGlobalByIndex(int(globalIdx)); exists {
			exports[exportName] = value
		} else {
			exports[exportName] = vm.Undefined
		}
	}

	// Re-exports (export { x } from "./mod", or export { X } where X was
	// itself an imported binding rather than a local declaration - see
	// paserati#163) never occupy a local global slot, so ExportIndices above
	// can't see them. They resolve through the VM's own module-context
	// machinery instead - see VM.GetModuleExport.
	for exportName, re := range moduleRecord.GetReExports() {
		if _, already := exports[exportName]; already {
			continue
		}
		exports[exportName] = p.vmInstance.GetModuleExport(moduleRecord.ResolvedPath, re.SourceModule, re.SourceName)
	}

	return exports
}

// registerNativeModuleExports registers native module exports with the HeapAlloc system
// This ensures that when other modules import from native modules, the compiler can
// find the correct global indices for the imported names
func (p *Paserati) registerNativeModuleExports(moduleRecord *modules.ModuleRecord) {
	if !moduleRecord.IsNativeModule() {
		return
	}

	exportValues := moduleRecord.GetExportValues()
	debugPrintf("// [Driver] Registering %d native module exports with HeapAlloc\n", len(exportValues))

	// Get the HeapAlloc instance from the compiler
	heapAlloc := p.compiler.GetHeapAlloc()
	if heapAlloc == nil {
		debugPrintf("// [Driver] Warning: No HeapAlloc available, cannot register native module exports\n")
		return
	}

	// Register each export with the HeapAlloc and set the value in the VM heap
	for exportName, exportValue := range exportValues {
		// Get or assign a global index for this export name
		globalIndex := heapAlloc.GetOrAssignIndex(exportName)
		debugPrintf("// [Driver] Registered native export '%s' at global index %d\n", exportName, globalIndex)

		// Set the value directly in the VM's heap
		if err := p.vmInstance.GetHeap().Set(globalIndex, exportValue); err != nil {
			debugPrintf("// [Driver] Warning: Failed to set native export '%s' in VM heap: %v\n", exportName, err)
		}
	}
}

// tryGetExportValue attempts to get the runtime value of an exported variable
// This looks up the variable in the VM's global space or symbol table
func (p *Paserati) tryGetExportValue(exportName string) (vm.Value, bool) {
	// Try to get the value from the VM's global table first
	if globalValue, exists := p.vmInstance.GetGlobal(exportName); exists {
		debugPrintf("// [Driver] tryGetExportValue: Found global value for '%s'\n", exportName)
		return globalValue, true
	}

	// If not found in globals, try getting from the compiler's symbol table
	// This is where local variables would be stored
	debugPrintf("// [Driver] tryGetExportValue: '%s' not found in globals, checking symbol table\n", exportName)

	// TODO: For local variables, we need a different approach
	// Local variables are stored in registers during execution and may not be
	// accessible after the function/module completes

	return vm.Undefined, false
}

// coordinateModuleCompilerGlobals pre-populates a module compiler with builtin global indices
// This ensures module compilers start allocating from index 21+ (after builtins 0-20)
func (p *Paserati) coordinateModuleCompilerGlobals(moduleCompiler *compiler.Compiler) {
	// Get all global variables that have been initialized in the main compiler
	globalNames := p.compiler.GetGlobalNames()

	debugPrintf("// [Driver] coordinateModuleCompilerGlobals: Pre-populating %d builtin globals\n", len(globalNames))

	// Pre-assign the same global indices in the module compiler to maintain consistency
	for _, name := range globalNames {
		globalIdx := p.compiler.GetGlobalIndex(name)
		if globalIdx >= 0 {
			// Force the module compiler to use the same index for this builtin
			moduleCompiler.SetGlobalIndex(name, globalIdx)
			debugPrintf("// [Driver] coordinateModuleCompilerGlobals: Set '%s' to index %d\n", name, globalIdx)
		}
	}
}

// preloadNativeModules scans the AST for import statements and preloads any native modules
// This ensures their exports are registered with HeapAlloc before the importing code is compiled
func (p *Paserati) preloadNativeModules(program *parser.Program) errors.PaseratiError {
	// Scan the AST for import declarations
	for _, stmt := range program.Statements {
		if importDecl, ok := stmt.(*parser.ImportDeclaration); ok {
			if importDecl.Source != nil {
				modulePath := importDecl.Source.Value

				// Check if this is a native module
				if p.nativeResolver != nil {
					if p.nativeResolver.CanResolve(modulePath) {
						debugPrintf("// [Driver] Preloading native module: %s\n", modulePath)

						// Load the native module through the module loader
						moduleRecord, err := p.moduleLoader.LoadModule(modulePath, ".")
						if err != nil {
							return &errors.CompileError{
								Position: errors.Position{Line: 0, Column: 0},
								Msg:      fmt.Sprintf("Failed to preload native module '%s': %v", modulePath, err),
							}
						}

						// Register its exports with HeapAlloc
						if concreteRecord, ok := moduleRecord.(*modules.ModuleRecord); ok {
							p.registerNativeModuleExports(concreteRecord)
						}
					}
				}
			}
		}
	}

	return nil
}

// installBuiltinModules installs all built-in Paserati modules
func installBuiltinModules(p *Paserati) {
	// Note: fetch and Headers are now global builtins (defined in pkg/builtins/fetch_init.go)
	// The paserati/http module is deprecated - use global fetch instead

	// Add more modules here as we create them
	// p.DeclareModule("paserati/fs", fsModule)
	// p.DeclareModule("paserati/crypto", cryptoModule)
}

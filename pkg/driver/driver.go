package driver

import (
	"fmt"
	"io/ioutil"
	"os"
	"paserati/pkg/builtins"
	"paserati/pkg/checker"
	"paserati/pkg/compiler"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/modules"
	"paserati/pkg/parser"
	"paserati/pkg/source"
	"paserati/pkg/vm"
	"strings"
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
	nativeResolver   *NativeModuleResolver // *NativeModuleResolver - defined in native_module.go to avoid import cycles
	ignoreTypeErrors bool                  // When true, type checking errors are ignored and compilation continues
}

// SetIgnoreTypeErrors sets whether type checking errors should be ignored
func (p *Paserati) SetIgnoreTypeErrors(ignore bool) {
	p.ignoreTypeErrors = ignore
}

// Cleanup breaks circular references to allow garbage collection
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

	// Create file system resolver for the specified base directory
	fsResolver := modules.NewFileSystemResolver(os.DirFS(baseDir), baseDir)

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

	// Set the VM instance in the module loader for native module initialization
	moduleLoader.SetVMInstance(vmInstance)

	// Initialize builtins using custom initializers
	if err := initializeBuiltinsWithCustom(paserati, customInitializers); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Builtin initialization failed: %v\n", err)
	}

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

	// Create file system resolver for the specified base directory
	fsResolver := modules.NewFileSystemResolver(os.DirFS(baseDir), baseDir)

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

	// Set the VM instance in the module loader for native module initialization
	moduleLoader.SetVMInstance(vmInstance)

	// Initialize builtins using new initializer system FIRST
	if err := initializeBuiltins(paserati); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Builtin initialization failed: %v\n", err)
	}

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
	// Honor session setting to ignore type errors (used for Test262)
	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
	return p.compiler.Compile(program)
}

// SyncGlobalNamesFromCompiler syncs the compiler's global name mappings to the VM
// This should be called after CompileProgram to ensure globalThis property access works
func (p *Paserati) SyncGlobalNamesFromCompiler() {
	nameMap := p.compiler.GetHeapAlloc().GetNameToIndexMap()
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
	// Honor session setting to ignore type errors (used for Test262)
	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
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

	comp := compiler.NewCompiler() // Fresh compiler
	// Compile will create and use its own internal checker
	chunk, compileAndTypeErrs := comp.Compile(program)
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

	comp := compiler.NewCompiler() // Fresh compiler
	// Compile will create and use its own internal checker
	chunk, compileAndTypeErrs := comp.Compile(program)
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

	// After successful execution, collect exported values from the compiler
	if p.compiler.IsModuleMode() {
		exportedValues := p.collectExportedValues()
		moduleRecord.ExportValues = exportedValues
		// Also store the export indices mapping for dynamic import support
		// Convert map[string]int to map[string]uint16
		exportGlobalIndices := p.compiler.GetExportGlobalIndices()
		exportIndices := make(map[string]uint16, len(exportGlobalIndices))
		for name, idx := range exportGlobalIndices {
			exportIndices[name] = uint16(idx)
		}
		moduleRecord.ExportIndices = exportIndices
		debugPrintf("// [Driver] Collected %d exported values from module\n", len(exportedValues))
	}

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
		// Set the compiler's ignore type errors flag based on our setting
		p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)

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
	}

	// Set the module path in the VM so import.meta.url works correctly
	p.vmInstance.SetCurrentModulePath(moduleRecord.ResolvedPath)

	// Execute the module and return the final value
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)

	// After successful execution, collect exported values from the compiler
	if p.compiler.IsModuleMode() {
		exportedValues := p.collectExportedValues()
		moduleRecord.ExportValues = exportedValues
		// Also store the export indices mapping for dynamic import support
		// Convert map[string]int to map[string]uint16
		exportGlobalIndices := p.compiler.GetExportGlobalIndices()
		exportIndices := make(map[string]uint16, len(exportGlobalIndices))
		for name, idx := range exportGlobalIndices {
			exportIndices[name] = uint16(idx)
		}
		moduleRecord.ExportIndices = exportIndices
		debugPrintf("// [Driver] Collected %d exported values from module\n", len(exportedValues))
	}

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
	// Set the compiler's ignore type errors flag based on our setting
	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)

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

	// Set the module path in the VM so import.meta.url works correctly
	p.vmInstance.SetCurrentModulePath(moduleName)

	// Execute the chunk
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)

	// Drain microtasks for async operations (Promises, etc.)
	p.vmInstance.DrainMicrotasks()

	return finalValue, runtimeErrs
}

// runAsTemporaryModule runs code with imports as a temporary module
// DEPRECATED: Use runAsModule instead
func (p *Paserati) runAsTemporaryModule(sourceCode string, program *parser.Program) (vm.Value, []errors.PaseratiError) {
	return p.runAsModule(sourceCode, program, "__temp_module__")
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
}

// RunCode runs source code with the given Paserati session and options.
// Now runs in module mode by default.
func (p *Paserati) RunCode(sourceCode string, options RunOptions) (vm.Value, []errors.PaseratiError) {
	sourceFile := source.NewEvalSource(sourceCode)
	l := lexer.NewLexerWithSource(sourceFile)
	parseInstance := parser.NewParser(l)
	program, parseErrs := parseInstance.ParseProgram()
	if len(parseErrs) > 0 {
		return vm.Undefined, parseErrs
	}

	// Determine module name - use provided name or default to "__code_module__"
	moduleName := options.ModuleName
	if moduleName == "" {
		moduleName = "__code_module__"
	}

	// Run in module mode
	value, errs := p.runAsModule(sourceCode, program, moduleName)

	// Get the compiled chunk for debugging output if needed
	if options.ShowBytecode || options.ShowCacheStats {
		// Re-compile to get chunk for display (the runAsModule already executed it)
		// This is a bit wasteful but only happens when debugging flags are on
		p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
		chunk, _ := p.compiler.Compile(program)

		if chunk != nil {
			// Show bytecode if requested
			if options.ShowBytecode {
				fmt.Println("\n=== Bytecode ===")
				fmt.Print(chunk.DisassembleChunk("<module>"))
				fmt.Println("================")
			}
		}

		// Show cache statistics if requested
		if options.ShowCacheStats {
			fmt.Println("\n=== Inline Cache Statistics ===")
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
	return p.vmInstance.Interpret(chunk)
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

	return nil
}

// collectExportedValues collects the runtime values of exported variables from the VM
// This is called after successful module execution to populate the ModuleRecord.ExportValues
func (p *Paserati) collectExportedValues() map[string]vm.Value {
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
	// HTTP module
	p.DeclareModule("paserati/http", httpModule)

	// Add more modules here as we create them
	// p.DeclareModule("paserati/fs", fsModule)
	// p.DeclareModule("paserati/crypto", cryptoModule)
}

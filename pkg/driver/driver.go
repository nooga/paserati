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
	"sort"
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
	vmInstance   *vm.VM
	checker      *checker.Checker
	compiler     *compiler.Compiler
	moduleLoader modules.ModuleLoader
}

// NewPaserati creates a new Paserati session with a fresh VM and Checker.
// Uses the current working directory as the base for module resolution.
func NewPaserati() *Paserati {
	return NewPaseratiWithBaseDir(".")
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
	}
	
	// Wire the module loader into the VM
	vmInstance.SetModuleLoader(moduleLoader)
	
	// Set up the checker factory for the module loader
	// This allows the module loader to create type checkers without circular imports
	moduleLoader.SetCheckerFactory(func() modules.TypeChecker {
		// Create a new checker instance for module type checking
		newChecker := checker.NewChecker()
		return newChecker
	})
	
	// Set up the compiler factory for the module loader
	// This allows the module loader to create compilers without circular imports
	moduleLoader.SetCompilerFactory(func() modules.Compiler {
		// Create a new compiler instance for module compilation
		newCompiler := compiler.NewCompiler()
		// Return a wrapper that adapts the return type to interface{}
		return &compilerAdapter{newCompiler}
	})

	// Initialize builtins using new initializer system
	if err := initializeBuiltins(paserati); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Builtin initialization failed: %v\n", err)
	}

	return paserati
}

// RunString compiles and executes the given source code in the current session.
// It uses the persistent type checker environment.
// Returns the result value and any errors that occurred.
func (p *Paserati) RunString(sourceCode string) (vm.Value, []errors.PaseratiError) {
	sourceFile := source.NewEvalSource(sourceCode)
	l := lexer.NewLexerWithSource(sourceFile)
	parseInstance := parser.NewParser(l)
	// Parse into a Program node, which the checker expects
	program, parseErrs := parseInstance.ParseProgram()
	if len(parseErrs) > 0 {
		// Convert parser errors (which might not implement PaseratiError directly)
		// TODO: Ensure parser errors conform to PaseratiError interface or wrap them.
		// For now, we'll assume they are compatible or handle later.
		// If ParseProgram already returns PaseratiError, this is fine.
		// If not, we need a conversion step here.
		// Let's assume parser.ParseProgram returns compatible errors for now.
		return vm.Undefined, parseErrs
	}

	// Dump AST if enabled
	parser.DumpAST(program, "RunString")

	// --- Type Checking is now done inside Compiler.Compile using the persistent checker ---
	// No need to call p.checker.Check(program) here directly.

	// --- Compilation Step ---
	chunk, compileAndTypeErrs := p.compiler.Compile(program)
	if len(compileAndTypeErrs) > 0 {
		return vm.Undefined, compileAndTypeErrs // Errors could be type or compile errors
	}
	if chunk == nil {
		// Handle internal compiler error where no chunk is returned despite no errors
		internalErr := &errors.RuntimeError{
			Position: errors.Position{Line: 0, Column: 0}, // Placeholder position
			Msg:      "Internal Error: Compilation returned nil chunk without errors.",
		}
		return vm.Undefined, []errors.PaseratiError{internalErr}
	}
	// --- End Compilation ---

	// --- Execution Step (using persistent VM) ---
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)
	// Interpret errors are already PaseratiError
	return finalValue, runtimeErrs
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
	
	// Always use module mode - it gracefully handles both module and non-module files
	return paserati.RunModule(filename)
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
	
	// Enable module mode in the checker and compiler
	p.checker.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)
	p.compiler.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)
	
	// Type check the module (already done during loading, but we need to compile)
	chunk, compileErrs := p.compiler.Compile(moduleRecord.AST)
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
	
	// Enable module mode in the checker and compiler
	p.checker.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)
	p.compiler.EnableModuleMode(moduleRecord.ResolvedPath, p.moduleLoader)
	
	// Type check the module (already done during loading, but we need to compile)
	chunk, compileErrs := p.compiler.Compile(moduleRecord.AST)
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
	
	// Execute the module and return the final value
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)
	
	// After successful execution, collect exported values from the compiler
	if p.compiler.IsModuleMode() {
		exportedValues := p.collectExportedValues()
		moduleRecord.ExportValues = exportedValues
		debugPrintf("// [Driver] Collected %d exported values from module\n", len(exportedValues))
	}
	
	return finalValue, []errors.PaseratiError{}, runtimeErrs
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
	ShowCacheStats bool // Show inline cache statistics
}

// RunCode runs source code with the given Paserati session and options
func (p *Paserati) RunCode(sourceCode string, options RunOptions) (vm.Value, []errors.PaseratiError) {
	sourceFile := source.NewEvalSource(sourceCode)
	l := lexer.NewLexerWithSource(sourceFile)
	parseInstance := parser.NewParser(l)
	program, parseErrs := parseInstance.ParseProgram()
	if len(parseErrs) > 0 {
		return vm.Undefined, parseErrs
	}

	// Dump AST if enabled
	parser.DumpAST(program, "RunCode")

	// --- Compilation Step ---
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

	// Show bytecode if requested
	if options.ShowBytecode {
		fmt.Println("\n=== Bytecode ===")
		fmt.Print(chunk.DisassembleChunk("<script>"))
		fmt.Println("================")
	}

	// --- Execution Step (using persistent VM) ---
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)

	// Show cache statistics if requested
	if options.ShowCacheStats {
		fmt.Println("\n=== Inline Cache Statistics ===")
		p.vmInstance.PrintCacheStats()
		fmt.Println("===============================")
	}

	return finalValue, runtimeErrs
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
// ensuring they use the same global index ordering
func initializeBuiltins(paserati *Paserati) error {
	vmInstance := paserati.vmInstance
	comp := paserati.compiler
	//checker := paserati.checker

	// Get all standard initializers
	initializers := builtins.GetStandardInitializers()

	// Create runtime context for VM initialization
	globalVariables := make(map[string]vm.Value)

	runtimeCtx := &builtins.RuntimeContext{
		VM: vmInstance,
		DefineGlobal: func(name string, value vm.Value) error {
			globalVariables[name] = value
			return nil
		},
	}

	// Initialize all builtins runtime values
	for _, init := range initializers {
		if err := init.InitRuntime(runtimeCtx); err != nil {
			return fmt.Errorf("failed to initialize %s runtime: %v", init.Name(), err)
		}
	}

	// Pre-populate compiler global indices in alphabetical order to match VM
	var globalNames []string
	for name := range globalVariables {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)

	// Pre-assign global indices in the compiler to match VM ordering
	for _, name := range globalNames {
		comp.GetOrAssignGlobalIndex(name)
	}

	// Set up global variables in VM
	return vmInstance.SetBuiltinGlobals(globalVariables)
}

// collectExportedValues collects the runtime values of exported variables from the VM
// This is called after successful module execution to populate the ModuleRecord.ExportValues
func (p *Paserati) collectExportedValues() map[string]vm.Value {
	exports := make(map[string]vm.Value)
	
	if !p.compiler.IsModuleMode() {
		return exports
	}
	
	// Get the export bindings from the compiler
	moduleExports := p.compiler.GetModuleExports()
	debugPrintf("// [Driver] collectExportedValues: Found %d module exports in compiler\n", len(moduleExports))
	
	// For each export binding, try to get the actual runtime value
	// TODO: This is a simplified implementation. In a full implementation,
	// we would need to map from the binding information to actual VM values
	for exportName, _ := range moduleExports {
		// For now, we'll try to get the value from the VM's global scope
		// This is a placeholder - we need a better way to map exports to values
		if value, exists := p.tryGetExportValue(exportName); exists {
			exports[exportName] = value
			debugPrintf("// [Driver] collectExportedValues: Collected export '%s' = %s\n", exportName, value.Type())
		} else {
			debugPrintf("// [Driver] collectExportedValues: Could not find runtime value for export '%s'\n", exportName)
		}
	}
	
	return exports
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

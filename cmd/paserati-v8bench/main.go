package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/driver"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/source"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// V8BenchInitializer provides `load` and `print` builtins for V8 benchmark compatibility
type V8BenchInitializer struct {
	baseDir  string           // Base directory for resolving load() paths
	paserati *driver.Paserati // Reference to the Paserati session for compiling loaded scripts
}

func NewV8BenchInitializer(baseDir string) *V8BenchInitializer {
	return &V8BenchInitializer{
		baseDir: baseDir,
	}
}

func (v *V8BenchInitializer) SetPaserati(p *driver.Paserati) {
	v.paserati = p
}

func (v *V8BenchInitializer) Name() string {
	return "V8Bench"
}

func (v *V8BenchInitializer) Priority() int {
	return 100 // Lower priority, run after standard builtins
}

func (v *V8BenchInitializer) InitTypes(ctx *builtins.TypeContext) error {
	// Define load function type: (filename: string) => void
	loadFunctionType := types.NewSimpleFunction([]types.Type{types.String}, types.Void)
	if err := ctx.DefineGlobal("load", loadFunctionType); err != nil {
		return err
	}

	// Define print function type: (...args: any[]) => void
	printFunctionType := types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)
	if err := ctx.DefineGlobal("print", printFunctionType); err != nil {
		return err
	}

	return nil
}

func (v *V8BenchInitializer) InitRuntime(ctx *builtins.RuntimeContext) error {
	vmInstance := ctx.VM

	// Create print function - outputs to stdout
	printFunc := vm.NewNativeFunction(0, true, "print", func(args []vm.Value) (vm.Value, error) {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = arg.ToString()
		}
		fmt.Println(strings.Join(parts, " "))
		return vm.Undefined, nil
	})

	if err := ctx.DefineGlobal("print", printFunc); err != nil {
		return err
	}

	// Create load function - loads and executes script in same context
	// We need to capture v in the closure
	initV := v
	loadFunc := vm.NewNativeFunction(1, false, "load", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, fmt.Errorf("load: missing filename argument")
		}

		filename := args[0].ToString()

		// Resolve relative path from base directory
		fullPath := filepath.Join(initV.baseDir, filename)

		// Read the file
		sourceBytes, err := os.ReadFile(fullPath)
		if err != nil {
			return vm.Undefined, fmt.Errorf("load: failed to read '%s': %v", filename, err)
		}

		sourceCode := string(sourceBytes)

		// Parse the source code
		sourceFile := source.FromFile(filename, sourceCode)
		lx := lexer.NewLexerWithSource(sourceFile)
		parseInstance := parser.NewParser(lx)
		program, parseErrs := parseInstance.ParseProgram()
		if len(parseErrs) > 0 {
			return vm.Undefined, fmt.Errorf("load: parse error in '%s': %v", filename, parseErrs[0])
		}

		// Compile using the Paserati session's compiler
		// This ensures we use the same global index allocation
		chunk, compileErrs := initV.paserati.CompileProgram(program)
		if len(compileErrs) > 0 {
			return vm.Undefined, fmt.Errorf("load: compile error in '%s': %v", filename, compileErrs[0])
		}

		if chunk == nil {
			return vm.Undefined, fmt.Errorf("load: compilation returned nil chunk for '%s'", filename)
		}

		// Execute in the same VM context
		_, runtimeErrs := vmInstance.Interpret(chunk)
		if len(runtimeErrs) > 0 {
			return vm.Undefined, fmt.Errorf("load: runtime error in '%s': %v", filename, runtimeErrs[0])
		}

		return vm.Undefined, nil
	})

	if err := ctx.DefineGlobal("load", loadFunc); err != nil {
		return err
	}

	return nil
}

func main() {
	var (
		benchDir = flag.String("dir", "benchmarks/v8-v7", "Directory containing V8 benchmark files")
		runFile  = flag.String("run", "run.js", "Entry point script to run")
		verbose  = flag.Bool("verbose", false, "Verbose output")
	)

	flag.Parse()

	// Ensure AST dump is off
	parser.DumpASTEnabled = false

	// Get absolute path for the benchmark directory
	absDir, err := filepath.Abs(*benchDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving benchmark directory: %v\n", err)
		os.Exit(1)
	}

	// Verify the entry point exists
	entryPoint := filepath.Join(absDir, *runFile)
	if _, err := os.Stat(entryPoint); err != nil {
		fmt.Fprintf(os.Stderr, "Error: entry point not found at %s\n", entryPoint)
		os.Exit(1)
	}

	if *verbose {
		fmt.Printf("Running V8 benchmark from: %s\n", absDir)
		fmt.Printf("Entry point: %s\n", *runFile)
	}

	// Create custom initializer for V8 benchmark builtins
	v8Init := NewV8BenchInitializer(absDir)

	// Get standard initializers and add our custom one
	initializers := append(builtins.GetStandardInitializers(), v8Init)

	// Create Paserati session with custom initializers
	paserati := driver.NewPaseratiWithInitializersAndBaseDir(initializers, absDir)
	defer paserati.Cleanup()

	// Give the initializer a reference to paserati so load() can compile scripts
	v8Init.SetPaserati(paserati)

	// Disable type checking for JavaScript files
	paserati.SetIgnoreTypeErrors(true)

	// Read and execute the entry point
	sourceBytes, err := os.ReadFile(entryPoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading entry point: %v\n", err)
		os.Exit(1)
	}

	sourceCode := string(sourceBytes)

	// Parse and compile
	sourceFile := source.FromFile(*runFile, sourceCode)
	lx := lexer.NewLexerWithSource(sourceFile)
	parseInstance := parser.NewParser(lx)
	program, parseErrs := parseInstance.ParseProgram()
	if len(parseErrs) > 0 {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", parseErrs[0])
		os.Exit(1)
	}

	chunk, compileErrs := paserati.CompileProgram(program)
	if len(compileErrs) > 0 {
		fmt.Fprintf(os.Stderr, "Compile error: %v\n", compileErrs[0])
		os.Exit(1)
	}

	if chunk == nil {
		fmt.Fprintf(os.Stderr, "Error: compilation returned nil chunk\n")
		os.Exit(1)
	}

	// Execute
	_, runtimeErrs := paserati.GetVM().Interpret(chunk)
	if len(runtimeErrs) > 0 {
		fmt.Fprintf(os.Stderr, "Runtime error: %v\n", runtimeErrs[0])
		os.Exit(1)
	}

	if *verbose {
		fmt.Println("\nBenchmark completed successfully")
	}
}

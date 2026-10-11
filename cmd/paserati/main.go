package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/driver"
	"github.com/nooga/paserati/pkg/nodemodules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/tsconfig"
	"github.com/nooga/paserati/pkg/vm"
)

// Version information - set via ldflags at build time
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// noImplicitAny is the -noImplicitAny flag, read where each Paserati is made.
var noImplicitAny bool

func main() {
	// Define flags
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	exprFlag := flag.String("e", "", "Run the given expression and exit")
	emitJSFlag := flag.Bool("js", false, "Emit JavaScript from TypeScript source file")
	jsOutputFile := flag.String("o", "", "Output file for JavaScript emission (default: input file with .js extension)")
	cacheStatsFlag := flag.Bool("cache-stats", false, "Show inline cache statistics after execution")
	bytecodeFlag := flag.Bool("bytecode", false, "Show compiled bytecode before execution")
	disasmFilterFlag := flag.String("disasm-filter", "", "Filter disassembly output by function name")
	astDumpFlag := flag.Bool("ast", false, "Show AST dump before type checking")
	noImplicitAnyFlag := flag.Bool("noImplicitAny", false, "Report parameters and rest parameters with an implicit any type (TS7006, TS7019)")
	noTypecheckFlag := flag.Bool("no-typecheck", false, "Ignore TypeScript type errors (like paserati-test262)")
	cpuProfileFlag := flag.String("cpuprofile", "", "Write CPU profile to file (pprof)")
	memProfileFlag := flag.String("memprofile", "", "Write heap profile to file (pprof)")
	checkFlag := flag.Bool("check", false, "Type-check the given files and their imports without running them (like tsc --noEmit); packages resolve to their typings from node_modules")
	noNodeModulesFlag := flag.Bool("no-node-modules", false, "Do not resolve bare package imports from node_modules")
	projectFlag := flag.String("p", "", "Type-check the project of a tsconfig.json (a file, or a directory holding one), like tsc -p --noEmit")

	flag.Parse() // Parses the command-line flags
	noImplicitAny = *noImplicitAnyFlag

	// Handle version flag
	if *versionFlag {
		fmt.Printf("paserati %s\n", version)
		if commit != "unknown" {
			fmt.Printf("  commit: %s\n", commit)
		}
		if buildDate != "unknown" {
			fmt.Printf("  built:  %s\n", buildDate)
		}
		fmt.Printf("  go:     %s\n", runtime.Version())
		return
	}

	// Optional: CPU profiling for the whole process lifetime.
	if *cpuProfileFlag != "" {
		f, err := os.Create(*cpuProfileFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create cpuprofile '%s': %s\n", *cpuProfileFlag, err.Error())
			os.Exit(70)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			_ = f.Close()
			fmt.Fprintf(os.Stderr, "Failed to start CPU profile: %s\n", err.Error())
			os.Exit(70)
		}
		defer func() {
			pprof.StopCPUProfile()
			_ = f.Close()
		}()
	}
	writeHeapProfile := func() {
		// Optional: heap profile at exit (after a GC for a cleaner snapshot).
		if *memProfileFlag == "" {
			return
		}
		runtime.GC()
		f, err := os.Create(*memProfileFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create memprofile '%s': %s\n", *memProfileFlag, err.Error())
			return
		}
		_ = pprof.WriteHeapProfile(f)
		_ = f.Close()
	}
	defer writeHeapProfile()

	// Set global AST dump flag
	parser.DumpASTEnabled = *astDumpFlag

	if *projectFlag != "" {
		code := runProjectCheck(*projectFlag, !*noNodeModulesFlag)
		if *cpuProfileFlag != "" {
			pprof.StopCPUProfile() // os.Exit skips the deferred stop
		}
		writeHeapProfile()
		os.Exit(code)
	}

	if *checkFlag {
		if flag.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: paserati --check [options] <file.ts>...\n")
			os.Exit(64)
		}
		code := runCheck(flag.Args(), !*noNodeModulesFlag)
		if *cpuProfileFlag != "" {
			pprof.StopCPUProfile() // os.Exit skips the deferred stop
		}
		writeHeapProfile()
		os.Exit(code)
	}

	// JavaScript emission mode
	if *emitJSFlag {
		if flag.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: paserati -js [options] <input.ts>\n")
			os.Exit(64) // Exit code 64: command line usage error
		}

		inputFile := flag.Arg(0)
		ok := driver.WriteJavaScriptFile(inputFile, *jsOutputFile)
		if !ok {
			os.Exit(70) // Exit code 70: internal software error
		}
		return
	}

	// Normal execution mode
	if *exprFlag != "" {
		// Run the expression provided via -e flag
		runExpressionWithTypes(*exprFlag, *cacheStatsFlag, *bytecodeFlag, *noTypecheckFlag, *disasmFilterFlag)
		return
	}

	if flag.NArg() >= 1 {
		// Execute the script file provided as an argument
		// Additional arguments after the script are passed as process.argv
		scriptArgs := flag.Args() // [script, arg1, arg2, ...]
		runFileWithTypes(scriptArgs[0], scriptArgs, *cacheStatsFlag, *bytecodeFlag, *noTypecheckFlag, *disasmFilterFlag)
	} else {
		// No file provided, start the REPL
		runReplWithTypes(*cacheStatsFlag, *bytecodeFlag, *noTypecheckFlag, *disasmFilterFlag)
	}
}

// runExpression executes a single expression provided via the -e flag
func runExpression(expr string, showCacheStats bool, showBytecode bool) {
	// Create a new Paserati session
	paserati := driver.NewPaserati()
	paserati.SetNoImplicitAny(noImplicitAny)

	// Run the expression with options
	options := driver.RunOptions{ShowCacheStats: showCacheStats, ShowBytecode: showBytecode}
	value, errs := paserati.RunCode(expr, options)

	// Display the result or errors
	ok := paserati.DisplayResult(expr, value, errs)

	// Exit with appropriate code
	if !ok {
		os.Exit(70) // Exit code 70: internal software error
	}
}

// exitOnUnhandledRejection makes a promise rejection that is still unhandled
// once the microtask queue drains terminate the program: it is reported on
// stderr and the process exits with the same status as an uncaught
// exception, like Node (#120).
func exitOnUnhandledRejection(paserati *driver.Paserati) {
	machine := paserati.GetVM()
	machine.SetUnhandledRejectionHandler(func(reason vm.Value, _ vm.Value) {
		fmt.Fprintln(os.Stderr, machine.FormatUnhandledRejection(reason))
		os.Exit(70)
	})
}

func runExpressionWithTypes(expr string, showCacheStats bool, showBytecode bool, ignoreTypes bool, disasmFilter string) {
	paserati := driver.NewPaserati()
	paserati.SetNoImplicitAny(noImplicitAny)
	exitOnUnhandledRejection(paserati)
	if ignoreTypes {
		// Completely skip type checking for pure JS mode
		paserati.SetSkipTypeCheck(true)
	}
	options := driver.RunOptions{ShowCacheStats: showCacheStats, ShowBytecode: showBytecode, DisasmFilter: disasmFilter}
	value, errs := paserati.RunCode(expr, options)
	ok := paserati.DisplayResult(expr, value, errs)
	if !ok {
		os.Exit(70)
	}
}

// runFile uses the driver to compile and execute a Paserati script from a file.
func runFile(filename string, showCacheStats bool, showBytecode bool) {
	if showCacheStats || showBytecode {
		// For file execution with debug options, we need to read the file and use RunStringWithOptions
		sourceBytes, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read file '%s': %s\n", filename, err.Error())
			os.Exit(70)
		}
		source := string(sourceBytes)
		options := driver.RunOptions{ShowCacheStats: showCacheStats, ShowBytecode: showBytecode}
		ok := driver.RunStringWithOptions(source, options)
		if !ok {
			os.Exit(70)
		}
	} else {
		// Use the existing simple version
		ok := driver.RunFile(filename)
		if !ok {
			os.Exit(70)
		}
	}
}

func runFileWithTypes(filename string, scriptArgs []string, showCacheStats bool, showBytecode bool, ignoreTypes bool, disasmFilter string) {
	sourceBytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read file '%s': %s\n", filename, err.Error())
		os.Exit(70)
	}
	source := string(sourceBytes)

	// Set up process.argv with: ["paserati", script, ...args]
	argv := append([]string{"paserati"}, scriptArgs...)

	// Create Paserati with standard builtins + process initializer
	initializers := builtins.GetStandardInitializers()
	initializers = append(initializers, driver.NewProcessInitializer(argv))
	paserati := driver.NewPaseratiWithInitializers(initializers)
	paserati.SetNoImplicitAny(noImplicitAny)
	exitOnUnhandledRejection(paserati)
	if ignoreTypes {
		// Completely skip type checking for pure JS mode
		paserati.SetSkipTypeCheck(true)
	}

	options := driver.RunOptions{ShowCacheStats: showCacheStats, ShowBytecode: showBytecode, ModuleName: filename, Filename: filename, DisasmFilter: disasmFilter}
	value, errs := paserati.RunCode(source, options)
	ok := paserati.DisplayResult(source, value, errs)
	if !ok {
		os.Exit(70)
	}
}

// runRepl starts the Read-Eval-Print Loop.
func runRepl(showCacheStats bool, showBytecode bool) {
	reader := bufio.NewReader(os.Stdin)

	// Create a persistent Paserati session for the REPL
	paserati := driver.NewPaserati()
	paserati.SetNoImplicitAny(noImplicitAny)

	fmt.Println("Paserati (Ctrl+C to exit)")
	if showCacheStats {
		fmt.Println("Cache statistics enabled")
	}

	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				break // Exit loop on EOF (Ctrl+D)
			}
			fmt.Fprintf(os.Stderr, "Error reading input: %s\n", err)
			break // Exit on other read errors
		}

		if line == "\n" { // Skip empty lines
			continue
		}

		// Check if the input contains import statements - if so, use module mode
		if containsImportsInString(line) {
			// Use module mode for imports - RunStringWithModules doesn't support options yet,
			// so we lose debug output for import statements but gain module functionality
			value, errs := paserati.RunStringWithModules(line)
			_ = paserati.DisplayResult(line, value, errs) // Ignore the bool return in REPL
		} else {
			// Use script mode for regular statements with full options support
			options := driver.RunOptions{ShowCacheStats: showCacheStats, ShowBytecode: showBytecode}
			value, errs := paserati.RunCode(line, options)
			_ = paserati.DisplayResult(line, value, errs) // Ignore the bool return in REPL
		}
	}
}

func runReplWithTypes(showCacheStats bool, showBytecode bool, ignoreTypes bool, disasmFilter string) {
	reader := bufio.NewReader(os.Stdin)
	paserati := driver.NewPaserati()
	paserati.SetNoImplicitAny(noImplicitAny)
	if ignoreTypes {
		// Completely skip type checking for pure JS mode
		paserati.SetSkipTypeCheck(true)
	}
	fmt.Println("Paserati (Ctrl+C to exit)")
	if showCacheStats {
		fmt.Println("Cache statistics enabled")
	}
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				break
			}
			fmt.Fprintf(os.Stderr, "Error reading input: %s\n", err)
			break
		}
		if line == "\n" {
			continue
		}
		if containsImportsInString(line) {
			value, errs := paserati.RunStringWithModules(line)
			_ = paserati.DisplayResult(line, value, errs)
		} else {
			options := driver.RunOptions{ShowCacheStats: showCacheStats, ShowBytecode: showBytecode, DisasmFilter: disasmFilter}
			value, errs := paserati.RunCode(line, options)
			_ = paserati.DisplayResult(line, value, errs)
		}
	}
}

// containsImportsInString is a simple heuristic to detect import statements in REPL input
// This avoids the need to fully parse the input just to detect imports
func containsImportsInString(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "import ") || strings.Contains(trimmed, "\nimport ")
}

// runCheck type-checks files without running them and prints diagnostics in
// tsc's format, returning tsc's exit status (0 clean, 2 with errors).
func runCheck(files []string, nodeModules bool) int {
	p := driver.NewPaserati()
	if nodeModules {
		p.AddResolver(nodemodules.NewResolver(nodemodules.Options{Mode: nodemodules.Types}))
	}
	opts := driver.DefaultCheckOptions()
	opts.NoImplicitAny = opts.NoImplicitAny || noImplicitAny
	return printCheckResults(p.CheckFilesWithOptions(files, opts))
}

// unsupportedOptions are compiler options that make tsc report errors
// Paserati does not check for yet.
var unsupportedOptions = []string{
	"noUncheckedIndexedAccess", "exactOptionalPropertyTypes", "noPropertyAccessFromIndexSignature",
	"noUnusedLocals", "noUnusedParameters", "noImplicitReturns", "noFallthroughCasesInSwitch",
}

// runProjectCheck type-checks a tsconfig.json project (tsc -p --noEmit).
func runProjectCheck(path string, nodeModules bool) int {
	cfg, err := tsconfig.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	for _, name := range unsupportedOptions {
		if v, set := cfg.Bool(name); set && v {
			fmt.Fprintf(os.Stderr, "paserati: note: compiler option '%s' is not supported yet; its errors are not reported\n", name)
		}
	}
	p := driver.NewPaserati()
	if nodeModules {
		p.AddResolver(nodemodules.NewResolver(nodemodules.Options{Mode: nodemodules.Types, Paths: cfg.Paths}))
	}
	opts := driver.CheckOptions{
		StrictNullChecks:             cfg.StrictFamily("strictNullChecks"),
		StrictFunctionTypes:          cfg.StrictFamily("strictFunctionTypes"),
		StrictPropertyInitialization: cfg.StrictFamily("strictPropertyInitialization"),
		NoImplicitAny:                cfg.StrictFamily("noImplicitAny"),
		AlwaysStrict:                 cfg.StrictFamily("alwaysStrict"),
	}
	opts.NoImplicitOverride, _ = cfg.Bool("noImplicitOverride")
	opts.AllowUnreachableCode, _ = cfg.Bool("allowUnreachableCode")
	opts.SkipLibCheck, _ = cfg.Bool("skipLibCheck")
	if nodeModules {
		opts.Types = cfg.Types()
		opts.TypesFrom = filepath.Dir(cfg.Path)
	}
	// Diagnostics are reported relative to the working directory, as tsc does.
	return printCheckResults(p.CheckFilesWithOptions(cfg.Files, opts))
}

// printCheckResults prints diagnostics as tsc does (file(line,col): error
// TSxxxx: message) and returns the exit status: 2 when there are errors.
func printCheckResults(results []driver.FileDiagnostics) int {
	wd, _ := os.Getwd()
	count := 0
	for _, res := range results {
		name := res.Path
		if rel, err := filepath.Rel(wd, name); err == nil && !strings.HasPrefix(rel, "..") {
			name = rel
		}
		if res.LoadError != nil {
			fmt.Printf("%s: error: %s\n", name, res.LoadError)
			count++
			continue
		}
		for _, d := range res.Diagnostics {
			pos := d.Pos()
			fmt.Printf("%s(%d,%d): error %s: %s\n", name, pos.Line, pos.Column, d.Code(), d.Message())
			count++
		}
	}
	if count > 0 {
		return 2
	}
	return 0
}

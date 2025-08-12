package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"paserati/pkg/driver"
	"paserati/pkg/parser"
	"strings"
	// \"paserati/pkg/vm\" // Remove: VM no longer directly used here
)

func main() {
	// Define flags
	exprFlag := flag.String("e", "", "Run the given expression and exit")
	emitJSFlag := flag.Bool("js", false, "Emit JavaScript from TypeScript source file")
	jsOutputFile := flag.String("o", "", "Output file for JavaScript emission (default: input file with .js extension)")
	cacheStatsFlag := flag.Bool("cache-stats", false, "Show inline cache statistics after execution")
	bytecodeFlag := flag.Bool("bytecode", false, "Show compiled bytecode before execution")
	astDumpFlag := flag.Bool("ast", false, "Show AST dump before type checking")
	noTypecheckFlag := flag.Bool("no-typecheck", false, "Ignore TypeScript type errors (like paserati-test262)")

	flag.Parse() // Parses the command-line flags

	// Set global AST dump flag
	parser.DumpASTEnabled = *astDumpFlag

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
		runExpressionWithTypes(*exprFlag, *cacheStatsFlag, *bytecodeFlag, *noTypecheckFlag)
		return
	}

	if flag.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "Usage: paserati [script] or paserati -e \"expression\" or paserati -js <input.ts>\n")
		os.Exit(64) // Exit code 64: command line usage error
	} else if flag.NArg() == 1 {
		// Execute the script file provided as an argument
		runFileWithTypes(flag.Arg(0), *cacheStatsFlag, *bytecodeFlag, *noTypecheckFlag)
	} else {
		// No file provided, start the REPL
		runReplWithTypes(*cacheStatsFlag, *bytecodeFlag, *noTypecheckFlag)
	}
}

// runExpression executes a single expression provided via the -e flag
func runExpression(expr string, showCacheStats bool, showBytecode bool) {
	// Create a new Paserati session
	paserati := driver.NewPaserati()

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

func runExpressionWithTypes(expr string, showCacheStats bool, showBytecode bool, ignoreTypes bool) {
	paserati := driver.NewPaserati()
	paserati.SetIgnoreTypeErrors(ignoreTypes)
	options := driver.RunOptions{ShowCacheStats: showCacheStats, ShowBytecode: showBytecode}
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

func runFileWithTypes(filename string, showCacheStats bool, showBytecode bool, ignoreTypes bool) {
	sourceBytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read file '%s': %s\n", filename, err.Error())
		os.Exit(70)
	}
	source := string(sourceBytes)
	paserati := driver.NewPaserati()
	paserati.SetIgnoreTypeErrors(ignoreTypes)
	options := driver.RunOptions{ShowCacheStats: showCacheStats, ShowBytecode: showBytecode}
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

func runReplWithTypes(showCacheStats bool, showBytecode bool, ignoreTypes bool) {
	reader := bufio.NewReader(os.Stdin)
	paserati := driver.NewPaserati()
	paserati.SetIgnoreTypeErrors(ignoreTypes)
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
			options := driver.RunOptions{ShowCacheStats: showCacheStats, ShowBytecode: showBytecode}
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

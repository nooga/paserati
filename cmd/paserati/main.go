package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"paserati/pkg/driver"
	// \"paserati/pkg/vm\" // Remove: VM no longer directly used here
)

func main() {
	// Define flags
	exprFlag := flag.String("e", "", "Run the given expression and exit")
	emitJSFlag := flag.Bool("js", false, "Emit JavaScript from TypeScript source file")
	jsOutputFile := flag.String("o", "", "Output file for JavaScript emission (default: input file with .js extension)")
	cacheStatsFlag := flag.Bool("cache-stats", false, "Show inline cache statistics after execution")

	flag.Parse() // Parses the command-line flags

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
		runExpression(*exprFlag, *cacheStatsFlag)
		return
	}

	if flag.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "Usage: paserati [script] or paserati -e \"expression\" or paserati -js <input.ts>\n")
		os.Exit(64) // Exit code 64: command line usage error
	} else if flag.NArg() == 1 {
		// Execute the script file provided as an argument
		runFile(flag.Arg(0), *cacheStatsFlag)
	} else {
		// No file provided, start the REPL
		runRepl(*cacheStatsFlag)
	}
}

// runExpression executes a single expression provided via the -e flag
func runExpression(expr string, showCacheStats bool) {
	// Create a new Paserati session
	paserati := driver.NewPaserati()

	// Run the expression with options
	options := driver.RunOptions{ShowCacheStats: showCacheStats}
	value, errs := paserati.RunCode(expr, options)

	// Display the result or errors
	ok := paserati.DisplayResult(expr, value, errs)

	// Exit with appropriate code
	if !ok {
		os.Exit(70) // Exit code 70: internal software error
	}
}

// runFile uses the driver to compile and execute a Paserati script from a file.
func runFile(filename string, showCacheStats bool) {
	if showCacheStats {
		// For file execution with cache stats, we need to read the file and use RunStringWithOptions
		sourceBytes, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read file '%s': %s\n", filename, err.Error())
			os.Exit(70)
		}
		source := string(sourceBytes)
		options := driver.RunOptions{ShowCacheStats: showCacheStats}
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

// runRepl starts the Read-Eval-Print Loop.
func runRepl(showCacheStats bool) {
	reader := bufio.NewReader(os.Stdin)

	// Create a persistent Paserati session for the REPL
	paserati := driver.NewPaserati()

	fmt.Println("Paserati REPL (Persistent Session) (Ctrl+C to exit)")
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

		// Run the input in the persistent session with options
		options := driver.RunOptions{ShowCacheStats: showCacheStats}
		value, errs := paserati.RunCode(line, options)
		_ = paserati.DisplayResult(line, value, errs) // Ignore the bool return in REPL
	}
}

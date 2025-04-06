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
	// Define the -e flag for running expressions
	exprFlag := flag.String("e", "", "Run the given expression and exit")
	flag.Parse() // Parses the command-line flags

	if *exprFlag != "" {
		// Run the expression provided via -e flag
		runExpression(*exprFlag)
		return
	}

	if flag.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "Usage: paserati [script] or paserati -e \"expression\"\n")
		os.Exit(64) // Exit code 64: command line usage error
	} else if flag.NArg() == 1 {
		// Execute the script file provided as an argument
		runFile(flag.Arg(0))
	} else {
		// No file provided, start the REPL
		runRepl()
	}
}

// runExpression executes a single expression provided via the -e flag
func runExpression(expr string) {
	// Create a new Paserati session
	paserati := driver.NewPaserati()

	// Run the expression
	value, errs := paserati.RunString(expr)

	// Display the result or errors
	ok := paserati.DisplayResult(expr, value, errs)

	// Exit with appropriate code
	if !ok {
		os.Exit(70) // Exit code 70: internal software error
	}
}

// runFile uses the driver to compile and execute a Paserati script from a file.
func runFile(filename string) {
	// driver.RunFile handles compilation, interpretation, and error display.
	ok := driver.RunFile(filename)
	if !ok {
		// Exit with a generic error code if RunFile reported errors.
		// Specific error codes (65, 70) distinction is lost here,
		// but the errors were already printed by the driver.
		os.Exit(70) // Exit code 70: internal software error (generic catch-all)
	}
	// If ok is true, execution succeeded and result (if any) was printed.
}

// runRepl starts the Read-Eval-Print Loop.
func runRepl() {
	reader := bufio.NewReader(os.Stdin)

	// Create a persistent Paserati session for the REPL
	paserati := driver.NewPaserati()

	fmt.Println("Paserati REPL (Persistent Session) (Ctrl+C to exit)")

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

		// Run the input in the persistent session
		value, errs := paserati.RunString(line)
		_ = paserati.DisplayResult(line, value, errs) // Ignore the bool return in REPL

		// --- Old non-persistent implementation ---
		// _ = driver.RunString(line) // Ignore the bool return in REPL

		// --- Even older REPL logic ---
		// // Compile the input line
		// chunk, compileErrs := driver.CompileString(line)
		// if compileErrs != nil {
		// 	// Print compile errors but continue the REPL loop
		// 	fmt.Fprintf(os.Stderr, \"\\t%s\\n\", e)
		// 	continue // Don't try to interpret if compilation failed
		// }
		//
		// // Interpret the compiled chunk
		// // We reset the VM partially for each line, mainly the stack pointer
		// // The Interpret method handles resetting frame count, etc.
		// _ = machine.Interpret(chunk) // Use blank identifier to ignore result
		//
		// // Runtime errors are printed by the VM itself via runtimeError.
		// // InterpretOK might print the result if the code ends with OpReturn.
		// // We don't need to explicitly check result here unless we want
		// // different REPL behavior based on the outcome.
	}
}

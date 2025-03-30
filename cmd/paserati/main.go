package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"paserati/pkg/driver"
	"paserati/pkg/vm"
)

func main() {
	flag.Parse() // Parses the command-line flags

	if flag.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "Usage: paserati [script]\n")
		os.Exit(64) // Exit code 64: command line usage error
	} else if flag.NArg() == 1 {
		// Execute the script file provided as an argument
		runFile(flag.Arg(0))
	} else {
		// No file provided, start the REPL
		runRepl()
	}
}

// runFile compiles and executes a Paserati script from a file.
func runFile(filename string) {
	chunk, compileErrs := driver.CompileFile(filename)
	if compileErrs != nil {
		fmt.Fprintf(os.Stderr, "Compile errors:\n")
		for _, err := range compileErrs {
			fmt.Fprintf(os.Stderr, "\t%s\n", err)
		}
		os.Exit(65) // Exit code 65: data format error
	}

	machine := vm.NewVM()
	result := machine.Interpret(chunk)

	// Check for runtime errors
	if result == vm.InterpretRuntimeError {
		// The VM's runtimeError function already prints the error and stack trace.
		os.Exit(70) // Exit code 70: internal software error
	}
	// InterpretCompileError shouldn't happen here as we check compileErrs above
}

// runRepl starts the Read-Eval-Print Loop.
func runRepl() {
	reader := bufio.NewReader(os.Stdin)
	machine := vm.NewVM() // Create one VM instance for the REPL session

	fmt.Println("Paserati REPL (Ctrl+C to exit)")

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

		// Compile the input line
		chunk, compileErrs := driver.CompileString(line)
		if compileErrs != nil {
			// Print compile errors but continue the REPL loop
			fmt.Fprintf(os.Stderr, "Compile errors:\n")
			for _, e := range compileErrs {
				fmt.Fprintf(os.Stderr, "\t%s\n", e)
			}
			continue // Don't try to interpret if compilation failed
		}

		// Interpret the compiled chunk
		// We reset the VM partially for each line, mainly the stack pointer
		// The Interpret method handles resetting frame count, etc.
		_ = machine.Interpret(chunk) // Use blank identifier to ignore result

		// Runtime errors are printed by the VM itself via runtimeError.
		// InterpretOK might print the result if the code ends with OpReturn.
		// We don't need to explicitly check result here unless we want
		// different REPL behavior based on the outcome.

	}
}

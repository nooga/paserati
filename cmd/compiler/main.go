package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"paserati/pkg/compiler"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
)

func main() {
	fmt.Println("--- Paserati Compiler ---")

	// 1. Check CLI arguments
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <filename.ts>\n", os.Args[0])
		os.Exit(1)
	}
	filename := os.Args[1]

	// 2. Read source file
	inputBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file '%s': %v\n", filename, err)
		os.Exit(1)
	}
	input := string(inputBytes)

	// 3. Lexing
	l := lexer.NewLexer(input)

	// 4. Parsing
	p := parser.NewParser(l)
	program := p.ParseProgram()
	parserErrors := p.Errors()
	if len(parserErrors) != 0 {
		printErrors(os.Stderr, "Parser Errors", parserErrors)
		os.Exit(1)
	}

	// Optional: Print AST for debugging
	// fmt.Println("--- AST ---")
	// fmt.Println(program.String())
	// fmt.Println("-----------")

	// 5. Compiling
	comp := compiler.NewCompiler()
	chunk, compilerErrors := comp.Compile(program)

	if len(compilerErrors) != 0 {
		printErrors(os.Stderr, "Compiler Errors", compilerErrors)
		os.Exit(1)
	}

	// 6. Disassemble & Print Bytecode
	fmt.Printf("--- Bytecode (%s) ---\n", filename)
	chunk.DisassembleChunk("Compiled Chunk")
	fmt.Println("------------------------")

	fmt.Println("Compilation successful.")
	// TODO: Optionally write chunk to file or execute directly
}

func printErrors(out *os.File, header string, errors []string) {
	fmt.Fprintf(out, "\n--- %s: ---\n", header)
	for _, msg := range errors {
		fmt.Fprintf(out, "\t%s\n", msg)
	}
	fmt.Fprintf(out, "--------------\n")
}

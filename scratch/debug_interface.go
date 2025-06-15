package main

import (
	"fmt"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
)

func main() {
	input := "interface Foo<T> {\n}"
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	
	// Try to parse it
	program, _ := p.ParseProgram()
	
	fmt.Printf("Parse errors: %v\n", p.Errors())
	fmt.Printf("Program: %v\n", program)
}
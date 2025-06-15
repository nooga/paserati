package main

import (
	"fmt"
	"paserati/pkg/lexer"
)

func main() {
	input := "interface"
	l := lexer.NewLexer(input)
	
	tok := l.NextToken()
	fmt.Printf("Type: %s, Literal: %s\n", tok.Type, tok.Literal)
}
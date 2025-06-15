package main

import (
	"fmt"
	"paserati/pkg/lexer"
)

func main() {
	input := "interface Foo<T> {"
	l := lexer.NewLexer(input)
	
	for {
		tok := l.NextToken()
		fmt.Printf("Type: %s, Literal: %s\n", tok.Type, tok.Literal)
		if tok.Type == lexer.EOF {
			break
		}
	}
}
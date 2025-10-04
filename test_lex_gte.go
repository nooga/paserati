package main

import (
	"fmt"
	"paserati/pkg/lexer"
)

func main() {
	input := "1 >= 1"
	l := lexer.New(input, "test.ts")
	
	for {
		tok := l.NextToken()
		fmt.Printf("Type: %s, Literal: %s\n", tok.Type, tok.Literal)
		if tok.Type == lexer.EOF {
			break
		}
	}
}

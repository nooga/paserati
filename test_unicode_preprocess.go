package main

import (
	"fmt"
	"paserati/pkg/lexer"
)

func main() {
	input := "-4\\u0009>>\\u00091"
	fmt.Printf("Input: %s\n", input)
	fmt.Printf("Input length: %d\n", len(input))

	output := lexer.PreprocessUnicodeEscapes(input)
	fmt.Printf("Output: %s\n", output)
	fmt.Printf("Output length: %d\n", len(output))

	for i, c := range output {
		fmt.Printf("Char %d: %c (0x%X)\n", i, c, c)
	}
}

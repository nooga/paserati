package main

import (
	"fmt"
	"paserati/pkg/lexer"
)

func main() {
	input := `eval("'\u000Cstr\u000Cing\u000C'")`
	fmt.Printf("Input: %s\n", input)

	output := lexer.PreprocessUnicodeEscapesContextAware(input)
	fmt.Printf("Output: %s\n", output)

	// Check if they're different
	if input != output {
		fmt.Printf("Preprocessing changed the input!\n")
	} else {
		fmt.Printf("Preprocessing did not change the input\n")
	}
}

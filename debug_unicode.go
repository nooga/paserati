package main

import (
	"fmt"
	"paserati/pkg/lexer"
)

func main() {
	// Test what happens with the Mongolian vowel separator
	input := `eval("var\u180Efoo;")`
	fmt.Printf("Input: %s\n", input)

	// Test the preprocessing
	processed := lexer.PreprocessUnicodeEscapesContextAware(input)
	fmt.Printf("Processed: %s\n", processed)

	// Check if they're different
	if input != processed {
		fmt.Printf("Preprocessing changed the input!\n")
		fmt.Printf("Length input: %d, Length processed: %d\n", len(input), len(processed))

		// Show the characters
		fmt.Printf("Input characters: ")
		for i, c := range input {
			fmt.Printf("[%d]='%c'(0x%X) ", i, c, c)
		}
		fmt.Printf("\n")

		fmt.Printf("Processed characters: ")
		for i, c := range processed {
			fmt.Printf("[%d]='%c'(0x%X) ", i, c, c)
		}
		fmt.Printf("\n")
	} else {
		fmt.Printf("Preprocessing did not change the input\n")
	}
}

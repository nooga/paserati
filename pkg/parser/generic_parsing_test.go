package parser

import (
	"paserati/pkg/lexer"
	"testing"
)

func TestParseGenericFunctionLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"function identity<T>(x: T): T { return x; }",
			"function identity<T>(x: T): T { return x; }",
		},
		{
			"function pair<T, U>(a: T, b: U): [T, U] { return [a, b]; }",
			"function pair<T, U>(a: T, b: U): [T, U] { return [a, b]; }",
		},
		{
			"function constrained<T extends string>(x: T): T { return x; }",
			"function constrained<T extends string>(x: T): T { return x; }",
		},
		{
			"function<T>(x: T): T { return x; }", // Anonymous generic function
			"function<T>(x: T): T { return x; }",
		},
	}

	for _, tt := range tests {
		l := lexer.NewLexer(tt.input)
		p := NewParser(l)
		program, parseErrs := p.ParseProgram()
		
		if len(parseErrs) != 0 {
			t.Errorf("Parser had %d errors:", len(parseErrs))
			for _, err := range parseErrs {
				t.Errorf("  %s", err.Error())
			}
			continue
		}
		
		if len(program.Statements) != 1 {
			t.Errorf("Expected 1 statement, got %d", len(program.Statements))
			continue
		}
		
		stmt, ok := program.Statements[0].(*ExpressionStatement)
		if !ok {
			t.Errorf("Expected ExpressionStatement, got %T", program.Statements[0])
			continue
		}
		
		fn, ok := stmt.Expression.(*FunctionLiteral)
		if !ok {
			t.Errorf("Expected FunctionLiteral, got %T", stmt.Expression)
			continue
		}
		
		// Check that type parameters were parsed
		if len(fn.TypeParameters) == 0 {
			t.Errorf("Expected type parameters to be parsed")
			continue
		}
		
		// Check the string representation
		actual := fn.String()
		if actual != tt.expected {
			t.Errorf("Expected '%s', got '%s'", tt.expected, actual)
		}
	}
}

func TestParseGenericArrowFunction(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"<T>(x: T): T => x",
			"<T>(x: T): T => x",
		},
		{
			"<T, U>(a: T, b: U): [T, U] => [a, b]",
			"<T, U>(a: T, b: U): [T, U] => [a, b]",
		},
		{
			"<T extends string>(x: T): T => x",
			"<T extends string>(x: T): T => x",
		},
		{
			"<T>(x: T) => x", // No return type annotation
			"<T>(x: T) => x",
		},
	}

	for _, tt := range tests {
		l := lexer.NewLexer(tt.input)
		p := NewParser(l)
		program, parseErrs := p.ParseProgram()
		
		if len(parseErrs) != 0 {
			t.Errorf("Parser had %d errors for input '%s':", len(parseErrs), tt.input)
			for _, err := range parseErrs {
				t.Errorf("  %s", err.Error())
			}
			continue
		}
		
		if len(program.Statements) != 1 {
			t.Errorf("Expected 1 statement, got %d for input '%s'", len(program.Statements), tt.input)
			continue
		}
		
		stmt, ok := program.Statements[0].(*ExpressionStatement)
		if !ok {
			t.Errorf("Expected ExpressionStatement, got %T for input '%s'", program.Statements[0], tt.input)
			continue
		}
		
		fn, ok := stmt.Expression.(*ArrowFunctionLiteral)
		if !ok {
			t.Errorf("Expected ArrowFunctionLiteral, got %T for input '%s'", stmt.Expression, tt.input)
			continue
		}
		
		// Check that type parameters were parsed
		if len(fn.TypeParameters) == 0 {
			t.Errorf("Expected type parameters to be parsed for input '%s'", tt.input)
			continue
		}
		
		// Check the string representation
		actual := fn.String()
		if actual != tt.expected {
			t.Errorf("Expected '%s', got '%s' for input '%s'", tt.expected, actual, tt.input)
		}
	}
}

func TestParseTypeParameters(t *testing.T) {
	tests := []struct {
		input          string
		expectedCount  int
		expectedNames  []string
		hasConstraints []bool
	}{
		{
			"<T>",
			1,
			[]string{"T"},
			[]bool{false},
		},
		{
			"<T, U>",
			2,
			[]string{"T", "U"},
			[]bool{false, false},
		},
		{
			"<T extends string>",
			1,
			[]string{"T"},
			[]bool{true},
		},
		{
			"<T, U extends number, V>",
			3,
			[]string{"T", "U", "V"},
			[]bool{false, true, false},
		},
	}

	for _, tt := range tests {
		l := lexer.NewLexer(tt.input)
		p := NewParser(l)
		p.nextToken() // Move to '<'
		
		typeParams, err := p.parseTypeParameters()
		if err != nil {
			t.Errorf("Failed to parse type parameters '%s': %v", tt.input, err)
			continue
		}
		
		if len(typeParams) != tt.expectedCount {
			t.Errorf("Expected %d type parameters, got %d for input '%s'", tt.expectedCount, len(typeParams), tt.input)
			continue
		}
		
		for i, param := range typeParams {
			if param.Name.Value != tt.expectedNames[i] {
				t.Errorf("Expected type parameter name '%s', got '%s' at index %d for input '%s'", 
					tt.expectedNames[i], param.Name.Value, i, tt.input)
			}
			
			hasConstraint := param.Constraint != nil
			if hasConstraint != tt.hasConstraints[i] {
				t.Errorf("Expected constraint existence %v, got %v at index %d for input '%s'", 
					tt.hasConstraints[i], hasConstraint, i, tt.input)
			}
		}
	}
}

func TestGenericFunctionWithComplexSignature(t *testing.T) {
	input := "function map<T, U>(arr: Array<T>, fn: (x: T) => U): Array<U> { return []; }"
	
	l := lexer.NewLexer(input)
	p := NewParser(l)
	program, parseErrs := p.ParseProgram()
	
	if len(parseErrs) != 0 {
		t.Errorf("Parser had %d errors:", len(parseErrs))
		for _, err := range parseErrs {
			t.Errorf("  %s", err.Error())
		}
		return
	}
	
	if len(program.Statements) != 1 {
		t.Errorf("Expected 1 statement, got %d", len(program.Statements))
		return
	}
	
	stmt, ok := program.Statements[0].(*ExpressionStatement)
	if !ok {
		t.Errorf("Expected ExpressionStatement, got %T", program.Statements[0])
		return
	}
	
	fn, ok := stmt.Expression.(*FunctionLiteral)
	if !ok {
		t.Errorf("Expected FunctionLiteral, got %T", stmt.Expression)
		return
	}
	
	// Check function name
	if fn.Name.Value != "map" {
		t.Errorf("Expected function name 'map', got '%s'", fn.Name.Value)
	}
	
	// Check type parameters
	if len(fn.TypeParameters) != 2 {
		t.Errorf("Expected 2 type parameters, got %d", len(fn.TypeParameters))
		return
	}
	
	if fn.TypeParameters[0].Name.Value != "T" {
		t.Errorf("Expected first type parameter 'T', got '%s'", fn.TypeParameters[0].Name.Value)
	}
	
	if fn.TypeParameters[1].Name.Value != "U" {
		t.Errorf("Expected second type parameter 'U', got '%s'", fn.TypeParameters[1].Name.Value)
	}
	
	// Check parameters
	if len(fn.Parameters) != 2 {
		t.Errorf("Expected 2 parameters, got %d", len(fn.Parameters))
		return
	}
	
	if fn.Parameters[0].Name.Value != "arr" {
		t.Errorf("Expected first parameter 'arr', got '%s'", fn.Parameters[0].Name.Value)
	}
	
	if fn.Parameters[1].Name.Value != "fn" {
		t.Errorf("Expected second parameter 'fn', got '%s'", fn.Parameters[1].Name.Value)
	}
}

func TestParseGenericArrowFunctionErrors(t *testing.T) {
	errorTests := []string{
		"<>", // Empty type parameters
		"<T", // Missing closing >
		"<T>(", // Missing parameter list end
		"<T>x", // Missing parentheses
	}

	for _, input := range errorTests {
		l := lexer.NewLexer(input)
		p := NewParser(l)
		program, parseErrs := p.ParseProgram()
		
		if len(parseErrs) == 0 {
			t.Errorf("Expected parser errors for input '%s', but got none", input)
		}
		
		// The program should still parse (possibly with nil expressions)
		if program == nil {
			t.Errorf("Expected program to be parsed even with errors for input '%s'", input)
		}
	}
}
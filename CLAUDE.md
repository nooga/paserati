# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Paserati is a TypeScript/JavaScript runtime implementation written in Go that compiles TypeScript directly to bytecode for a register-based virtual machine, bypassing JavaScript transpilation entirely. It combines compile-time type checking with runtime execution in a single pipeline.

## Development Commands

### Build and Run
```bash
# Build the main binary
go build -o paserati cmd/paserati/main.go

# Run the REPL
./paserati

# Execute a TypeScript file
./paserati path/to/script.ts

# Run expression directly
./paserati -e "let x = 42; x * 2"

# Show bytecode output
./paserati -bytecode script.ts

# Show inline cache statistics
./paserati -cache-stats script.ts
```

### Testing
```bash
# Run all tests
go test ./tests/...

# Run script-based tests (most comprehensive)
go test ./tests -run TestScripts

# Run a specific script test
go test ./tests -run "TestScripts/filename.ts"

# Run with verbose output to see individual test names
go test ./tests -v
```

### Adding New Language Features

When implementing new operators, statements, or language constructs, follow this pipeline:

1. **Lexer** (`pkg/lexer/lexer.go`): Add token types and keyword mappings
2. **Parser** (`pkg/parser/`): Add AST nodes (`ast.go`) and parsing logic (`parser.go`)
3. **Type Checker** (`pkg/checker/`): Add type checking logic in appropriate files
4. **Compiler** (`pkg/compiler/`): Add bytecode generation in `compile_*.go` files
5. **VM** (`pkg/vm/`): Add bytecode execution logic and any new opcodes
6. **Tests**: Create test scripts in `tests/scripts/` with expectation comments

## Architecture Overview

### Compilation Pipeline
The codebase follows a traditional compiler pipeline:
- **Lexer**: Tokenizes TypeScript source code
- **Parser**: Builds AST using Pratt parser for expressions
- **Type Checker**: Performs TypeScript-compliant type checking with type narrowing
- **Compiler**: Generates bytecode for register-based VM with register allocation
- **VM**: Executes bytecode with inline caching and optimized object representations

### Core Components

**Driver Package** (`pkg/driver/`): Entry point that orchestrates the compilation pipeline. The `Paserati` struct maintains persistent state between evaluations for REPL functionality.

**Virtual Machine** (`pkg/vm/`): Register-based VM with two object representations:
- `PlainObject`: Shape-based optimization similar to V8's hidden classes
- `DictObject`: Simple hash-map based storage for dynamic objects
- `ArrayObject`: Specialized array representation with `length` property

**Type System** (`pkg/types/`): Comprehensive TypeScript type system supporting:
- Primitive types, literal types, union/intersection types
- Object types, interfaces with inheritance
- Function types with overloads, optional/default parameters
- Tuple types with contextual typing integration
- Type narrowing and control flow analysis

**Built-ins** (`pkg/builtins/`): Modern architecture with prototype registry system. Each built-in type (Array, String, Math, etc.) has consolidated implementation with both runtime methods and type information.

### Testing Framework

**Script-Based Testing**: The primary testing mechanism uses TypeScript files in `tests/scripts/` with special comment annotations:
```typescript
// expect: value                    // Expected output value
// expect_runtime_error: message    // Expected runtime error
// expect_compile_error: message    // Expected compile error
```

Test files are automatically discovered and executed by `tests/scripts_test.go`.

## Key Implementation Patterns

### Adding New Operators
1. Add token type to lexer with appropriate precedence
2. Register infix/prefix parser function 
3. Add type checking case in checker's operator switch
4. Add compilation case in compiler's operator switch  
5. Add VM execution case with proper bytecode
6. Create test files covering success and error cases

### Object Property Operations
The VM supports three object types with different property access patterns:
- Use `HasOwn()` method for property existence
- Use `GetOwn()`/`SetOwn()` for property access
- Arrays handle numeric indices specially

### Type Checking Integration
The type checker uses widened types for compatibility checking and maintains separate environments for type narrowing in control flow branches.

### Bytecode Generation
The compiler uses register allocation with automatic cleanup via defer patterns. Each compilation function manages its temporary registers and frees them on completion.

## Feature Status

See `docs/bucketlist.md` for current implementation status. Recently implemented features include:
- Type assertions (`as` operator)
- Property existence checking (`in` operator)  
- Spread syntax in function calls
- Comprehensive type narrowing
- Interface inheritance
- Optional/default parameters

## Development Notes

- Use `go build ./...` to verify all packages compile
- The codebase emphasizes TypeScript compliance over JavaScript quirks
- Performance optimizations include inline caching, shape-based objects, and specialized bytecode operations
- Error messages should match TypeScript compiler output for familiarity
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

# Run script-based tests (most comprehensive) - THIS IS OUR SMOKE TEST
# This test suite MUST always be green - fix issues or mark as FIXME
go test ./tests -run TestScripts

# Run a specific script test
go test ./tests -run "TestScripts/filename.ts"

# Run with verbose output to see individual test names
go test ./tests -v
```

**Important**: The `TestScripts` suite in `tests/scripts_test.go` is our smoke test and should always pass. When something doesn't work, either fix it immediately or mark it as FIXME - never sweep issues under the rug.

### Adding New Language Features

When implementing new operators, statements, or language constructs, follow this pipeline:

1. **Lexer** (`pkg/lexer/lexer.go`): Add token types and keyword mappings
2. **Parser** (`pkg/parser/`): Add AST nodes (`ast.go`) and parsing logic (`parser.go`)
   - For complex features, create dedicated `parse_*.go` files (e.g., `parse_class.go`)
3. **Type Checker** (`pkg/checker/`): Add type checking logic in appropriate files
   - Create dedicated checker files for complex features (e.g., `class.go`, `narrowing.go`)
4. **Compiler** (`pkg/compiler/`): Add bytecode generation in `compile_*.go` files
   - Create dedicated compilation files for major features (e.g., `compile_class.go`)
5. **VM** (`pkg/vm/`): Add bytecode execution logic and any new opcodes
6. **Tests**: Create test scripts in `tests/scripts/` with expectation comments

## Architecture Overview

### Code Structure
The codebase is organized into distinct packages:
- `pkg/lexer/` - Lexical analysis and tokenization
- `pkg/parser/` - AST construction and syntax analysis
- `pkg/checker/` - Type checking and semantic analysis
- `pkg/compiler/` - Bytecode generation and optimization
- `pkg/vm/` - Virtual machine and runtime execution
- `pkg/types/` - Type system definitions and operations
- `pkg/builtins/` - Built-in objects and functions
- `pkg/driver/` - Main compilation pipeline orchestration
- `pkg/errors/` - Error types and handling
- `pkg/source/` - Source code management

**Important**: When implementing major features, create new files in the appropriate package rather than editing long existing files. For example:
- Parser features: Create `parse_feature.go` in `pkg/parser/`
- Type checking: Create `feature.go` in `pkg/checker/`
- Compilation: Create `compile_feature.go` in `pkg/compiler/`

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
- **Generic types** with constraints and type inference
- **Class types** represented as constructor function types
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
- **Classes** - Basic class declarations with constructors and property declarations
- Type assertions (`as` operator)
- Property existence checking (`in` operator)  
- Spread syntax in function calls
- Comprehensive type narrowing
- Interface inheritance
- Optional/default parameters
- **Generics** - Complete generic type system with constraints and inference
- **Regular expressions** - Full RegExp support with literal syntax and methods

## Development Notes

### Build and Debug

- **Always rebuild the binary before testing**: `go build -o paserati cmd/paserati/main.go`
- Use `go build ./...` to verify all packages compile
- Main components have debug flags in their `.go` files:
  - `pkg/lexer/lexer.go`: `const debugLexer = false`
  - `pkg/parser/parser.go`: `const debugParser = false`
  - `pkg/checker/checker.go`: `const debugChecker = false`
  - `pkg/compiler/compiler.go`: `const debugCompiler = false`
  - Enable these flags (set to `true`) to see detailed debug information when troubleshooting

### Quality Standards

- **Goal**: Production-quality compiler and VM - no shortcuts
- The codebase emphasizes TypeScript compliance over JavaScript quirks
- Performance optimizations include inline caching, shape-based objects, and specialized bytecode operations
- Error messages should match TypeScript compiler output for familiarity
- **When something doesn't work**: Either fix it immediately or mark it as FIXME - never sweep issues under the rug
- **Test quality**: Our smoke test (`TestScripts`) must always be green

### Implementation Guidelines

- **Important**: When implementing classes, properties must be explicitly declared in the class body (TypeScript requirement)
- **Important**: Never use git commands with the -i flag (like git rebase -i or git add -i) since they require interactive input which is not supported
- NEVER create files unless they're absolutely necessary for achieving your goal
- ALWAYS prefer editing an existing file to creating a new one, unless implementing a major feature
- NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User
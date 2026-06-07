# Source Pipeline Modernization

This document outlines a modernized source handling pipeline for Paserati that prepares for modules while improving error reporting and maintaining clean architecture.

## Current Issues

- No unified concept of "Source" (file vs REPL vs eval)
- Error reporting lacks file paths and spans
- Architecture not prepared for modules
- Heavy approach of attaching source to every token/position

## Proposed Architecture

### 1. Source Abstraction

```go
// pkg/source/source.go
type Source interface {
    Name() string        // Display name: "script.ts", "<repl>", "<eval>"
    Path() string        // Full path for files, empty for REPL/eval
    Content() string     // Source content
    Lines() []string     // Cached split lines
    IsFile() bool        // True if represents actual file
    DisplayPath() string // Best path for display (path or name)
}

type FileSource struct {
    name    string
    path    string
    content string
    lines   []string // lazy
}

type EvalSource struct {
    content string
    lines   []string // lazy
}

type ReplSource struct {
    content string
    lines   []string // lazy
}
```

### 2. Source as Pipeline Invariant

**Key Insight**: Lexer and Parser are instantiated per source, so Source becomes an invariant throughout the pipeline.

```go
// Lexer holds source reference
type Lexer struct {
    source Source
    // ... existing fields
}

func NewLexer(source Source) *Lexer {
    return &Lexer{
        source: source,
        input:  source.Content(),
        // ... rest
    }
}

// Parser reads source from lexer
type Parser struct {
    lexer  *Lexer
    source Source // cached from lexer
}

func NewParser(lexer *Lexer) *Parser {
    return &Parser{
        lexer:  lexer,
        source: lexer.GetSource(),
    }
}
```

### 3. Clean Token/Position Design

**No source on every token** - much cleaner than heavyweight approach:

```go
// Token stays lightweight
type Token struct {
    Type     TokenType
    Literal  string
    Line     int
    Column   int
    StartPos int
    EndPos   int
    // NO source field - lexer holds it
}

// Position created during error reporting
type Position struct {
    Line     int
    Column   int
    StartPos int
    EndPos   int
}

// Span represents a range in source with context
type Span struct {
    Source   Source
    Start    Position
    End      Position
}
```

### 4. AST Integration

Attach source to AST root when passing between pipeline stages:

```go
// AST nodes get span computation capability
type Node interface {
    GetToken() lexer.Token
    GetSpan(source Source) Span  // Computes span from tokens
}

// AST root carries source context
type Program struct {
    Statements []Statement
    Source     Source  // Added when passing to checker/compiler
}

// Recursive span computation
func (n *SomeNode) GetSpan(source Source) Span {
    // Compute span from children tokens
    start := n.GetFirstToken()
    end := n.GetLastToken()
    return Span{
        Source: source,
        Start:  Position{start.Line, start.Column, start.StartPos, start.StartPos},
        End:    Position{end.Line, end.Column, end.EndPos, end.EndPos},
    }
}
```

### 5. Error Creation Helpers

Smart error creation with span helpers:

```go
// Helper functions for error creation
func NewTypeErrorFromToken(source Source, token lexer.Token, msg string) *TypeError {
    return &TypeError{
        Span: SpanFromToken(source, token),
        Msg:  msg,
    }
}

func NewTypeErrorFromNode(source Source, node ast.Node, msg string) *TypeError {
    return &TypeError{
        Span: node.GetSpan(source),
        Msg:  msg,
    }
}

func SpanFromToken(source Source, token lexer.Token) Span {
    pos := Position{token.Line, token.Column, token.StartPos, token.EndPos}
    return Span{Source: source, Start: pos, End: pos}
}
```

### 6. Checker Integration

Checker gets source from AST:

```go
func (c *Checker) CheckProgram(program *ast.Program) {
    c.source = program.Source  // Cache source for error reporting
    
    for _, stmt := range program.Statements {
        c.checkStatement(stmt)
    }
}

func (c *Checker) addError(node ast.Node, msg string) {
    err := NewTypeErrorFromNode(c.source, node, msg)
    c.errors = append(c.errors, err)
}
```

### 7. Compiler/Runtime Integration

Bytecode chunks carry source information:

```go
type Chunk struct {
    Instructions []byte
    Constants    []vm.Value
    Source       Source      // For runtime error reporting
    SourceMap    []Position  // Maps instructions to source positions
}

// Runtime errors with file context
type RuntimeError struct {
    Span  Span
    Msg   string
    Stack []StackFrame
}

type StackFrame struct {
    FunctionName string
    Source       Source
    Position     Position
}
```

## Implementation Plan

### Phase 1: Core Source Abstraction
1. Implement `Source` interface and concrete types
2. Update lexer constructor and add `GetSource()` method
3. Update parser to read source from lexer

### Phase 2: AST Integration
1. Add `GetSpan()` methods to AST nodes
2. Thread source through checker via AST root
3. Create error helper functions

### Phase 3: Enhanced Error Reporting
1. Update `DisplayErrors` to use spans
2. Implement clickable file paths
3. Enhanced source context display

### Phase 4: Runtime Integration
1. Add source to bytecode chunks
2. Runtime error reporting with file context
3. Stack trace improvements

### Phase 5: Module Preparation
1. Source resolution for imports
2. Module-aware error reporting
3. Cross-module span tracking

## Benefits

### Immediate Benefits
- **Clickable file paths** in errors
- **Better error spans** with start/end positions
- **Cleaner architecture** without heavyweight tokens
- **Unified source concept** across pipeline

### Future Benefits
- **Module system ready** - each module has its Source
- **Enhanced debugging** with source maps
- **Better tooling support** (LSP, etc.)
- **Improved runtime errors** with file context

### Module System Readiness
- Each imported module gets its own Source
- Cross-module error reporting works naturally
- Import resolution can create appropriate Source types
- Dependency tracking through Source references

## Example Usage

```go
// File-based compilation
source := source.NewFileSource("script.ts", "/path/to/script.ts", content)
lexer := lexer.NewLexer(source)
parser := parser.NewParser(lexer)
program := parser.ParseProgram()
program.Source = source  // Thread source through

checker := checker.NewChecker()
checker.CheckProgram(program)  // Gets source from program

// Errors now have full context
for _, err := range checker.Errors() {
    fmt.Printf("Error at %s:%d:%d: %s\n", 
        err.Span.Source.DisplayPath(),
        err.Span.Start.Line,
        err.Span.Start.Column,
        err.Msg)
}
```

## Discussion Points

1. **AST Span Computation**: Should spans be computed lazily or cached?
2. **Source Lifetime**: How do we manage Source references in long-running processes?
3. **Module Resolution**: How does Source integrate with import/export?
4. **Performance**: Impact of span computation vs caching?
5. **Backward Compatibility**: Migration strategy for existing code?

This architecture provides a solid foundation for modules while dramatically improving error reporting with minimal complexity overhead.
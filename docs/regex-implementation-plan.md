# Regular Expression Implementation Plan

This document outlines the implementation of regular expression support in Paserati, leveraging Go's built-in `regexp` package for pattern matching while providing JavaScript-compliant syntax and semantics.

## 🎯 Overview

Regular expressions are a fundamental JavaScript feature that enables powerful text processing and pattern matching. By leveraging Go's robust `regexp` package, we can provide production-quality regex support without implementing a pattern engine from scratch.

## 🚀 Implementation Strategy

### Core Approach
- **Lexer**: Parse `/pattern/flags` literals as tokens
- **Parser**: Integrate regex literals as primary expressions
- **VM Types**: Add `TypeRegExp` with `RegExpObject` backed by `*regexp.Regexp`
- **Builtins**: Provide `RegExp` constructor and prototype methods
- **String Integration**: Add regex support to `String.prototype` methods

### Go Regex Integration
```go
// Core regex object structure
type RegExpObject struct {
    Object
    compiledRegex *regexp.Regexp  // Go's compiled regex
    source        string          // Original pattern string
    flags         string          // JavaScript flags (g, i, m, s, etc.)
    global        bool            // Cached global flag for performance
    ignoreCase    bool            // Cached ignoreCase flag
    multiline     bool            // Cached multiline flag
    dotAll        bool            // Cached dotAll flag
}
```

## 📋 Implementation Phases

### Phase 1: Lexer Support ⏳ **PLANNED**

**Goal**: Add regex literal tokenization to the lexer.

#### 1.1 Token Types
```go
// Add to lexer/token.go
REGEX_LITERAL // /pattern/flags
```

#### 1.2 Lexer Logic
```go
// Add to lexer/lexer.go
func (l *Lexer) readRegexLiteral() string {
    // Handle /pattern/flags parsing
    // Support escape sequences: \/, \\, etc.
    // Parse flags: g, i, m, s, u, y
    // Validate flag combinations
}
```

**Key Considerations**:
- **Context-sensitive parsing**: `/` can be division or regex start
- **Escape sequences**: Handle `\/`, `\\`, etc. properly
- **Flag validation**: Ensure valid flag combinations
- **Error handling**: Clear messages for invalid patterns

### Phase 2: Parser Integration ⏳ **PLANNED**

**Goal**: Parse regex literals as primary expressions.

#### 2.1 AST Nodes
```go
// Add to parser/ast.go
type RegexLiteral struct {
    BaseExpression
    Token   lexer.Token // The regex token
    Pattern string      // The pattern part
    Flags   string      // The flags part
}
```

#### 2.2 Parser Logic
```go
// Add to parser/parser.go
func (p *Parser) parseRegexLiteral() Expression {
    // Parse /pattern/flags into RegexLiteral AST node
    // Extract pattern and flags
    // Validate pattern syntax
}
```

**Key Considerations**:
- **Precedence**: Regex literals are primary expressions
- **Pattern validation**: Catch invalid regex patterns early
- **Flag parsing**: Support standard JavaScript flags

### Phase 3: Type System Support ⏳ **PLANNED**

**Goal**: Add regex types to the type checker.

#### 3.1 Type Definitions
```go
// Add to types/builtin.go
var RegExp = NewObjectType().
    WithProperty("source", String).
    WithProperty("flags", String).
    WithProperty("global", Boolean).
    WithProperty("ignoreCase", Boolean).
    WithProperty("multiline", Boolean).
    WithProperty("dotAll", Boolean).
    WithProperty("test", NewSimpleFunction([]Type{String}, Boolean)).
    WithProperty("exec", NewSimpleFunction([]Type{String}, Union(Null, ArrayType{String})))
```

#### 3.2 Type Checking
```go
// Add to checker/checker.go
func (c *Checker) checkRegexLiteral(node *parser.RegexLiteral) Type {
    // Validate pattern syntax
    // Return RegExp type
}
```

### Phase 4: VM Runtime Support ⏳ **PLANNED**

**Goal**: Add RegExp object type and compilation support.

#### 4.1 VM Types
```go
// Add to vm/value.go
const (
    // ... existing types
    TypeRegExp
)

// Add to vm/regex.go
type RegExpObject struct {
    Object
    compiledRegex *regexp.Regexp
    source        string
    flags         string
    global        bool
    ignoreCase    bool
    multiline     bool
    dotAll        bool
    lastIndex     int  // For global regex stateful matching
}

func NewRegExp(pattern, flags string) (Value, error) {
    // Compile Go regex with flag translation
    // Create RegExpObject
}
```

#### 4.2 Flag Translation
```go
func translateJSFlagsToGo(pattern, flags string) (*regexp.Regexp, error) {
    goPattern := pattern
    
    // Translate JavaScript flags to Go inline flags
    if strings.Contains(flags, "i") {
        goPattern = "(?i)" + goPattern
    }
    if strings.Contains(flags, "m") {
        goPattern = "(?m)" + goPattern  
    }
    if strings.Contains(flags, "s") {
        goPattern = "(?s)" + goPattern
    }
    
    return regexp.Compile(goPattern)
}
```

#### 4.3 Compilation
```go
// Add to compiler/compile_expression.go
func (c *Compiler) compileRegexLiteral(node *parser.RegexLiteral) {
    // Emit OpLoadRegex with pattern and flags
    // Runtime will compile the regex
}
```

### Phase 5: RegExp Constructor and Prototype ⏳ **PLANNED**

**Goal**: Implement RegExp builtin with constructor and prototype methods.

#### 5.1 Constructor
```javascript
// new RegExp(pattern)
// new RegExp(pattern, flags)
// new RegExp(regexObj)  // Copy constructor
```

#### 5.2 Prototype Methods
```javascript
RegExp.prototype.test(str)     // → boolean
RegExp.prototype.exec(str)     // → Array | null
RegExp.prototype.toString()    // → string
```

#### 5.3 Properties
```javascript
regex.source      // → string (pattern)
regex.flags       // → string (flags)
regex.global      // → boolean
regex.ignoreCase  // → boolean
regex.multiline   // → boolean
regex.dotAll      // → boolean
regex.lastIndex   // → number (for global matching)
```

### Phase 6: String Method Integration ⏳ **PLANNED**

**Goal**: Add regex support to String prototype methods.

#### 6.1 String.prototype.match()
```javascript
"hello world".match(/l+/g)      // → ["ll", "l"]
"hello world".match(/x/)        // → null
"hello world".match(/(h)(e)/)   // → ["he", "h", "e"]
```

#### 6.2 String.prototype.replace()
```javascript
"hello world".replace(/l/g, "x")           // → "hexxo worxd"
"hello world".replace(/(\w+) (\w+)/, "$2 $1")  // → "world hello"
```

#### 6.3 String.prototype.search()
```javascript
"hello world".search(/wor/)     // → 6
"hello world".search(/xyz/)     // → -1
```

#### 6.4 String.prototype.split()
```javascript
"a,b;c:d".split(/[,;:]/)       // → ["a", "b", "c", "d"]
"hello world".split(/\s+/)     // → ["hello", "world"]
```

## 🔧 Technical Implementation Details

### Flag Translation Mapping
| JavaScript Flag | Go Equivalent | Description |
|----------------|---------------|-------------|
| `g` (global) | N/A (handled in JS) | Find all matches |
| `i` (ignoreCase) | `(?i)` | Case-insensitive |
| `m` (multiline) | `(?m)` | `^` and `$` match line boundaries |
| `s` (dotAll) | `(?s)` | `.` matches newlines |
| `u` (unicode) | Default in Go | Unicode support |
| `y` (sticky) | Custom logic | Match from lastIndex |

### Pattern Compatibility
- **Go uses RE2**: Subset of PCRE, but covers most JavaScript use cases
- **No backtracking**: Go regex is linear time (actually better than JS!)
- **Capture groups**: Fully supported with `FindStringSubmatch()`
- **Character classes**: `\d`, `\w`, `\s` work identically

### Performance Considerations
- **Compile once**: Cache `*regexp.Regexp` in `RegExpObject`
- **Lazy compilation**: Compile patterns only when first used
- **Global matching**: Implement stateful matching for `g` flag
- **Memory management**: Go GC handles regex compilation memory

## 📝 JavaScript Compatibility

### Supported Features
- ✅ **Literal syntax**: `/pattern/flags`
- ✅ **Constructor**: `new RegExp(pattern, flags)`
- ✅ **All standard flags**: `g`, `i`, `m`, `s`, `u`
- ✅ **Prototype methods**: `test()`, `exec()`, `toString()`
- ✅ **String integration**: `match()`, `replace()`, `search()`, `split()`
- ✅ **Capture groups**: Full support with Go's submatch API
- ✅ **Character classes**: `\d`, `\w`, `\s`, etc.

### Limitations (Initial Implementation)
- 🚧 **Lookbehind assertions**: Not supported in Go RE2
- 🚧 **Backreferences**: Not supported in Go RE2
- 🚧 **Some advanced features**: May need emulation or graceful degradation

### Migration Path
1. **Start with core features**: Cover 90% of regex use cases
2. **Add emulation layer**: For unsupported features if needed
3. **Error handling**: Clear messages for unsupported patterns

## 🧪 Testing Strategy

### Phase 1-2 Tests: Parsing
```javascript
// Basic literal parsing
let regex1 = /hello/;
let regex2 = /world/gi;
let regex3 = /complex[A-Z]+/m;

// Constructor parsing  
let regex4 = new RegExp("pattern");
let regex5 = new RegExp("pattern", "gi");
```

### Phase 3-4 Tests: Runtime
```javascript
// Pattern compilation
let regex = /test/i;
console.log(regex.source);     // "test"
console.log(regex.flags);      // "i"
console.log(regex.ignoreCase); // true
```

### Phase 5 Tests: Methods
```javascript
// RegExp methods
let regex = /h(e)llo/;
console.log(regex.test("hello"));      // true
console.log(regex.exec("hello"));      // ["hello", "e"]
```

### Phase 6 Tests: String Integration
```javascript
// String methods with regex
console.log("hello world".match(/l+/g));        // ["ll", "l"]
console.log("hello world".replace(/l/g, "x"));  // "hexxo worxd"
console.log("a,b;c".split(/[,;]/));            // ["a", "b", "c"]
```

## 🎯 Success Criteria

### Phase 1-2: Parsing Complete
- ✅ Regex literals parse correctly: `/pattern/flags`
- ✅ Constructor calls parse: `new RegExp(pattern, flags)`
- ✅ AST represents regex structure properly

### Phase 3-4: Runtime Complete  
- ✅ Regex objects compile and store patterns
- ✅ Flag translation works correctly
- ✅ Properties accessible: `source`, `flags`, etc.

### Phase 5: Methods Complete
- ✅ `RegExp.prototype.test()` returns correct boolean
- ✅ `RegExp.prototype.exec()` returns match arrays
- ✅ Global flag state management works

### Phase 6: Integration Complete
- ✅ `String.prototype.match()` finds patterns
- ✅ `String.prototype.replace()` substitutes correctly
- ✅ `String.prototype.split()` splits on patterns
- ✅ Capture groups work in all contexts

## 🔄 Future Enhancements

### Advanced Features
- **Unicode property escapes**: `\p{Script=Latin}`
- **Named capture groups**: `(?<name>pattern)`
- **Lookbehind emulation**: Custom implementation for compatibility
- **RegExp.prototype.matchAll()**: ES2020 method
- **String.prototype.replaceAll()**: ES2021 method

### Performance Optimizations
- **Pattern caching**: Global cache for frequently used patterns
- **JIT compilation**: Pre-compile common patterns
- **Streaming matching**: For large text processing

## 🏁 Implementation Timeline

1. **Week 1**: Phase 1-2 (Lexer and Parser)
2. **Week 2**: Phase 3-4 (Types and Runtime)
3. **Week 3**: Phase 5 (RegExp builtin)
4. **Week 4**: Phase 6 (String integration)
5. **Week 5**: Testing and polish

## 🤝 Benefits

### For Developers
- **Complete regex support**: No missing functionality gaps
- **Performance**: Go's linear-time regex is actually faster than backtracking
- **Familiar syntax**: Standard JavaScript regex semantics
- **Rich string processing**: Unlocks advanced text manipulation

### For Paserati
- **Major feature milestone**: Regex is fundamental to JavaScript
- **Enables advanced builtins**: String methods become much more powerful
- **Library compatibility**: Many JS libraries require regex support
- **Production readiness**: Critical feature for real-world applications

By leveraging Go's excellent regex engine, we can provide robust, performant regular expression support that matches or exceeds the capabilities of traditional JavaScript engines.
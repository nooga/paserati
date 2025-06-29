# Paserati Module System Design Document

## Overview

This document outlines the design for implementing a comprehensive ES6/TypeScript module system in Paserati, built on Go's `io/fs` virtual file system interface for maximum flexibility and extensibility.

## Goals

- **Full TypeScript/ES6 module support**: `import`/`export` statements with all variants
- **Lazy loading**: Modules loaded only when needed
- **Idempotent loading**: Multiple imports of same module return same instance
- **Pluggable resolvers**: File system, URL, custom sources via `io/fs.FS`
- **Virtual file system**: Support for in-memory, embedded, and custom file systems
- **Type-aware**: Full integration with TypeScript type system
- **Performance**: Efficient caching and minimal overhead
- **Developer API**: Clean Go API for defining modules programmatically

## Architecture Overview

### Core Components

```
┌─────────────────────────────────────────────────────────────────┐
│                        Driver Layer                             │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐   │
│  │   File System   │ │   URL Resolver  │ │ Custom Resolver │   │
│  │   Resolver      │ │                 │ │                 │   │
│  └─────────────────┘ └─────────────────┘ └─────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Module System                              │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                   Module Registry                           │ │
│  │  • Module Cache (loaded modules)                           │ │
│  │  • Dependency Graph                                        │ │
│  │  • Circular Dependency Detection                           │ │
│  └─────────────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                   Module Loader                             │ │
│  │  • Resolution Strategy                                      │ │
│  │  • Compilation Pipeline Integration                         │ │
│  │  • Export/Import Processing                                │ │
│  └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Compilation Pipeline                         │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ │
│  │    Lexer    │ │   Parser    │ │   Checker   │ │  Compiler   │ │
│  │  +import    │ │  +import    │ │  +import    │ │  +import    │ │
│  │  +export    │ │  +export    │ │  +export    │ │  +export    │ │
│  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Runtime (VM)                              │
│  • Module Namespace Objects                                     │
│  • Export Binding Resolution                                    │
│  • Runtime Import Resolution                                    │
└─────────────────────────────────────────────────────────────────┘
```

## Detailed Design

### 1. Virtual File System Foundation

**Key Interfaces:**
```go
// Leverage Go's standard io/fs interfaces
type ModuleFS interface {
    io/fs.FS
    io/fs.ReadFileFS  // For reading module content
}

// Extended interface for writable VFS (for development scenarios)  
type WritableModuleFS interface {
    ModuleFS
    WriteFile(name string, data []byte, perm os.FileMode) error
    MkdirAll(path string, perm os.FileMode) error
}
```

**Built-in Resolvers:**
- **FileSystemResolver**: Standard OS file system via `os.DirFS`
- **EmbedResolver**: Go `embed.FS` for bundled modules
- **HTTPResolver**: Remote modules via HTTP/HTTPS
- **MemoryResolver**: In-memory virtual file system
- **CompositeResolver**: Layered resolvers (memory → embed → filesystem → HTTP)

### 2. Module Resolution & Parallel Processing

**Resolution Algorithm (Node.js compatible):**
```
1. Exact match: "./path/to/module.ts"
2. Add extensions: "./path/to/module" → "./path/to/module.ts", ".d.ts"
3. Directory index: "./path/to/dir" → "./path/to/dir/index.ts"
4. Node modules: "lodash" → "node_modules/lodash/index.ts"
5. Built-in modules: Special handling for Paserati built-ins
```

**Parallel Processing Pipeline:**
```
Entry Point → Dependency Discovery → Parallel Lex/Parse → Type Checking → Compilation
     │              │                      │                    │             │
     │              ▼                      ▼                    ▼             ▼
     │    ┌─────────────────┐    ┌─────────────────┐   ┌──────────────┐ ┌──────────────┐
     │    │ Module Queue    │    │ Worker Pool     │   │ Type Checker │ │ Compiler     │
     │    │ • BFS traversal │    │ • Lexer workers │   │ • Sequential │ │ • Sequential │
     │    │ • Deduplication │    │ • Parser workers│   │ • Dependency │ │ • Optimized  │
     │    │ • Prioritization│    │ • Error collect │   │   order      │ │   order      │
     │    └─────────────────┘    └─────────────────┘   └──────────────┘ └──────────────┘
```

**Module Specifier Types:**
- **Relative**: `"./utils"`, `"../lib/helper"`
- **Absolute**: `"/usr/lib/module"` (filesystem only)
- **Bare**: `"lodash"`, `"@types/node"` (node_modules lookup)
- **URL**: `"https://esm.sh/lodash"` (HTTP resolver)
- **Virtual**: `"virtual:my-module"` (custom resolver)

### 3. Module Registry & Caching

**Module States:**
```go
type ModuleState int
const (
    ModuleLoading ModuleState = iota  // Currently being loaded
    ModuleLoaded                      // Successfully loaded
    ModuleError                       // Failed to load
)

type ModuleRecord struct {
    Specifier    string                    // Original import specifier
    ResolvedPath string                    // Resolved file path
    State        ModuleState               // Current loading state
    Source       *source.SourceFile        // Parsed source file
    AST          *parser.Program           // Parsed AST
    Exports      map[string]types.Type     // Exported types
    ExportValues map[string]vm.Value       // Exported runtime values
    Namespace    vm.Value                  // Module namespace object
    Dependencies []string                  // Direct dependencies
    Error        error                     // Loading error (if any)
    LoadTime     time.Time                 // When module was loaded
}
```

**Registry Features:**
- **Idempotent loading**: Same specifier always returns same module
- **Circular dependency detection**: Maintains dependency graph
- **Cache invalidation**: Development mode file system watching
- **Memory management**: Configurable cache size and TTL

### 4. Language Integration

#### 4.1 Lexer Extensions

**New Tokens:**
```go
// Module-related tokens
IMPORT      TokenType = "import"      // import keyword
EXPORT      TokenType = "export"      // export keyword
FROM        TokenType = "from"        // from keyword
AS          TokenType = "as"          // as keyword (import/export alias)
DEFAULT     TokenType = "default"     // default keyword
STAR        TokenType = "*"           // * (namespace import/export)
```

#### 4.2 Parser Extensions

**New AST Nodes:**
```go
// Import Declarations
type ImportDeclaration struct {
    Specifiers []ImportSpecifier  // What to import
    Source     *StringLiteral     // From where ("./module")
}

type ImportSpecifier interface {
    importSpecifier()
}

type ImportDefaultSpecifier struct {
    Local *Identifier  // import foo from "./module"
}

type ImportNamespaceSpecifier struct {
    Local *Identifier  // import * as foo from "./module"
}

type ImportSpecifier struct {
    Imported *Identifier  // Original name
    Local    *Identifier  // Local alias (imported as local)
}

// Export Declarations  
type ExportDeclaration interface {
    exportDeclaration()
}

type ExportNamedDeclaration struct {
    Declaration *Declaration       // export const foo = 1
    Specifiers  []ExportSpecifier  // export { foo, bar }
    Source      *StringLiteral     // export { foo } from "./module"
}

type ExportDefaultDeclaration struct {
    Declaration Expression  // export default expression
}

type ExportAllDeclaration struct {
    Source *StringLiteral     // export * from "./module"
    Exported *Identifier      // export * as ns from "./module"
}
```

#### 4.3 Type Checker Integration

**Module Type Environment:**
```go
type ModuleEnvironment struct {
    *Environment                           // Base environment
    Imports     map[string]types.Type      // Imported types
    Exports     map[string]types.Type      // Exported types
    ModulePath  string                     // Current module path
    Loader      *ModuleLoader              // Reference to loader
}

// Enhanced resolution with module awareness
func (c *Checker) resolveModuleType(specifier string, name string) types.Type
func (c *Checker) checkImportDeclaration(node *ImportDeclaration)
func (c *Checker) checkExportDeclaration(node *ExportDeclaration)
```

#### 4.4 Compiler Integration

**Key Insight: Import/Export as Compile-Time Directives**

Import/export statements are **compile-time metadata** that establish module relationships. The compiler doesn't generate special bytecode for these statements. Instead, it:

1. **Resolves module bindings** during compilation  
2. **Maps imported names** to their source modules and export names
3. **Sets up runtime binding tables** for the VM
4. **Compiles regular code** with full knowledge of module context

**Module Compilation Strategy:**
```go
// Module binding information resolved at compile time
type ModuleBindings struct {
    ImportedNames  map[string]ModuleReference  // local_name -> module/export
    ExportedNames  map[string]string           // export_name -> local_name  
    DefaultExport  string                      // local name of default export
    ModulePath     string                      // this module's resolved path
}

type ModuleReference struct {
    ModulePath   string  // source module path
    ExportName   string  // name in source module
    IsNamespace  bool    // true for "import * as name"
    IsDefault    bool    // true for default imports
}
```

**Compilation Process:**
1. **Module Resolution Phase**: Resolve all import specifiers to actual module paths
2. **Binding Resolution Phase**: Map all imported names to their source exports  
3. **Code Generation Phase**: Compile code with full binding knowledge
4. **Runtime Table Setup**: Prepare module namespace objects and binding tables

#### 4.5 VM Runtime Support

**Module Namespace Objects:**
```go
type ModuleNamespace struct {
    vm.Object
    ModulePath string                  // Module identifier
    Exports    map[string]vm.Value     // Export bindings
    Default    vm.Value                // Default export
}

// Runtime import resolution
func (vm *VM) ImportModule(specifier string) (vm.Value, error)
func (vm *VM) GetModuleExport(module vm.Value, name string) vm.Value
```

### 5. Module Loader API

#### 5.1 Core Loader Interface

```go
type ModuleLoader struct {
    resolvers    []ModuleResolver     // Chain of resolvers
    registry     *ModuleRegistry      // Module cache
    compiler     *compiler.Compiler   // Compiler instance
    checker      *checker.Checker     // Type checker instance
    
    // Parallel processing components
    parseQueue   *ParseQueue          // Queue for modules to parse
    workerPool   *WorkerPool          // Pool of lex/parse workers
    depAnalyzer  *DependencyAnalyzer  // Dependency discovery
}

type ModuleResolver interface {
    Name() string
    Resolve(specifier string, from string) (ResolvedModule, error)
    CanResolve(specifier string) bool
}

type ResolvedModule struct {
    Specifier    string           // Original specifier
    ResolvedPath string           // Resolved path
    Source       io.ReadCloser    // Module source content
    FS           ModuleFS         // File system context
}
```

#### 5.2 Developer API for Programmatic Modules

```go
// High-level API for defining modules in Go code
type ModuleBuilder struct {
    name        string
    types       map[string]types.Type
    values      map[string]vm.Value
    defaultType types.Type
    defaultValue vm.Value
}

func NewModuleBuilder(name string) *ModuleBuilder
func (mb *ModuleBuilder) ExportType(name string, typ types.Type) *ModuleBuilder
func (mb *ModuleBuilder) ExportValue(name string, value vm.Value) *ModuleBuilder
func (mb *ModuleBuilder) ExportDefault(typ types.Type, value vm.Value) *ModuleBuilder
func (mb *ModuleBuilder) Build() VirtualModule

// Example usage:
module := NewModuleBuilder("my-util").
    ExportType("Helper", helperType).
    ExportValue("helper", helperInstance).
    ExportDefault(types.String, vm.NewString("default")).
    Build()
```

#### 5.3 VFS Integration Examples

```go
// File system resolver
fsResolver := NewFileSystemResolver(os.DirFS("./src"))

// HTTP resolver for remote modules
httpResolver := NewHTTPResolver(&http.Client{Timeout: 30*time.Second})

// Memory resolver for virtual modules
memResolver := NewMemoryResolver()
memResolver.AddModule("virtual:config", `
    export const API_URL = "https://api.example.com";
    export default { debug: true };
`)

// Composite resolver (try in order)
loader := NewModuleLoader(memResolver, fsResolver, httpResolver)
```

#### 5.4 Parallel Processing Architecture

**Worker Pool Design:**
```go
type WorkerPool struct {
    workers    []*ParseWorker        // Pool of worker goroutines
    jobQueue   chan *ParseJob        // Jobs to be processed
    resultChan chan *ParseResult     // Results from workers
    errorChan  chan error            // Error collection
    wg         sync.WaitGroup        // Wait group for shutdown
    ctx        context.Context       // Cancellation context
    cancel     context.CancelFunc    // Cancel function
}

type ParseJob struct {
    ModulePath   string               // Module to parse
    Source       *source.SourceFile   // Source content
    Priority     int                  // Job priority (0 = highest)
    Dependencies []string             // Known dependencies
    Timestamp    time.Time            // When job was queued
}

type ParseResult struct {
    ModulePath     string             // Module path
    AST            *parser.Program    // Parsed AST
    ImportSpecs    []*ImportSpec      // Discovered imports
    ExportSpecs    []*ExportSpec      // Discovered exports
    ParseDuration  time.Duration      // Time taken to parse
    Error          error              // Parse error (if any)
}

type ParseWorker struct {
    id         int                    // Worker ID
    jobQueue   <-chan *ParseJob       // Input job queue
    resultChan chan<- *ParseResult    // Output result channel
    lexer      *lexer.Lexer          // Reusable lexer instance
    parser     *parser.Parser        // Reusable parser instance
}
```

**Dependency-Driven Parsing:**
```go
type DependencyAnalyzer struct {
    discovered   map[string]bool      // Already discovered modules
    parsing      map[string]bool      // Currently being parsed
    parsed       map[string]*ParseResult // Completed parses
    queue        *PriorityQueue       // Parsing queue
    depGraph     map[string][]string  // Module → dependencies
    mutex        sync.RWMutex         // Thread safety
}

type ParseQueue struct {
    queue      *heap.PriorityQueue    // Priority queue for modules
    inFlight   map[string]bool        // Currently processing
    completed  map[string]*ParseResult // Completed results
    mutex      sync.RWMutex           // Thread safety
}

func (ml *ModuleLoader) LoadModuleParallel(entryPoint string) (*ModuleRecord, error) {
    // 1. Start dependency discovery
    discoveryCtx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // 2. Initialize worker pool
    workerPool := ml.startWorkerPool(discoveryCtx, runtime.NumCPU())
    defer workerPool.Shutdown()
    
    // 3. Queue entry point for parsing
    ml.parseQueue.Enqueue(&ParseJob{
        ModulePath: entryPoint,
        Priority:   0, // Highest priority
    })
    
    // 4. Parallel discovery and parsing loop
    for !ml.parseQueue.IsEmpty() || workerPool.HasActiveJobs() {
        select {
        case result := <-workerPool.resultChan:
            if err := ml.processParseResult(result); err != nil {
                return nil, err
            }
            
        case err := <-workerPool.errorChan:
            return nil, err
            
        case <-discoveryCtx.Done():
            return nil, discoveryCtx.Err()
        }
    }
    
    // 5. Perform type checking in dependency order
    return ml.performSequentialTypeChecking(entryPoint)
}

func (ml *ModuleLoader) processParseResult(result *ParseResult) error {
    ml.registry.SetParsed(result.ModulePath, result)
    
    // Discover new dependencies and queue them
    for _, importSpec := range result.ImportSpecs {
        if !ml.depAnalyzer.IsDiscovered(importSpec.ModulePath) {
            ml.depAnalyzer.MarkDiscovered(importSpec.ModulePath)
            
            // Resolve and queue new dependency
            resolved, err := ml.resolveDependency(importSpec.ModulePath, result.ModulePath)
            if err != nil {
                return err
            }
            
            // Calculate priority (dependencies of entry point get higher priority)
            priority := ml.calculatePriority(importSpec.ModulePath, result.ModulePath)
            
            ml.parseQueue.Enqueue(&ParseJob{
                ModulePath: resolved.ResolvedPath,
                Source:     resolved.Source,
                Priority:   priority,
            })
        }
    }
    
    return nil
}
```

**Worker Implementation:**
```go
func (w *ParseWorker) Run(ctx context.Context) {
    for {
        select {
        case job := <-w.jobQueue:
            result := w.processJob(job)
            
            select {
            case w.resultChan <- result:
            case <-ctx.Done():
                return
            }
            
        case <-ctx.Done():
            return
        }
    }
}

func (w *ParseWorker) processJob(job *ParseJob) *ParseResult {
    startTime := time.Now()
    
    // Reuse lexer and parser instances for performance
    w.lexer.Reset(job.Source)
    w.parser.Reset(w.lexer)
    
    // Parse the module
    program, parseErrs := w.parser.ParseProgram()
    
    var err error
    if len(parseErrs) > 0 {
        err = parseErrs[0] // Take first error
    }
    
    // Extract import/export information
    importSpecs := extractImportSpecs(program)
    exportSpecs := extractExportSpecs(program)
    
    return &ParseResult{
        ModulePath:    job.ModulePath,
        AST:           program,
        ImportSpecs:   importSpecs,
        ExportSpecs:   exportSpecs,
        ParseDuration: time.Since(startTime),
        Error:         err,
    }
}
```

**Priority Calculation:**
```go
func (ml *ModuleLoader) calculatePriority(modulePath, dependentPath string) int {
    // Priority rules:
    // 0 = Entry points (highest)
    // 1 = Direct dependencies of entry points
    // 2 = Second-level dependencies
    // ...
    // Higher numbers = lower priority
    
    depth := ml.depAnalyzer.GetDependencyDepth(modulePath)
    
    // Boost priority for frequently imported modules
    importCount := ml.depAnalyzer.GetImportCount(modulePath)
    frequencyBoost := max(0, importCount-1)
    
    return depth - frequencyBoost
}
```

**Performance Optimizations:**
```go
type WorkerPoolConfig struct {
    NumWorkers      int           // Number of parser workers
    JobBufferSize   int           // Size of job queue buffer
    ResultBuffer    int           // Size of result channel buffer
    MaxParseTime    time.Duration // Timeout for individual parses
    PrewarmLexers   bool          // Pre-allocate lexer instances
    ReuseAST        bool          // Reuse AST node pools
}

func (ml *ModuleLoader) startWorkerPool(ctx context.Context, config WorkerPoolConfig) *WorkerPool {
    jobQueue := make(chan *ParseJob, config.JobBufferSize)
    resultChan := make(chan *ParseResult, config.ResultBuffer)
    errorChan := make(chan error, config.NumWorkers)
    
    pool := &WorkerPool{
        workers:    make([]*ParseWorker, config.NumWorkers),
        jobQueue:   jobQueue,
        resultChan: resultChan,
        errorChan:  errorChan,
        ctx:        ctx,
    }
    
    // Start worker goroutines
    for i := 0; i < config.NumWorkers; i++ {
        worker := &ParseWorker{
            id:         i,
            jobQueue:   jobQueue,
            resultChan: resultChan,
        }
        
        if config.PrewarmLexers {
            worker.lexer = lexer.NewLexer("")
            worker.parser = parser.NewParser(worker.lexer)
        }
        
        pool.workers[i] = worker
        go worker.Run(ctx)
    }
    
    return pool
}
```

**Integration with Module Registry:**
```go
type ModuleRecord struct {
    Specifier    string                    // Original import specifier
    ResolvedPath string                    // Resolved file path
    State        ModuleState               // Current loading state
    Source       *source.SourceFile        // Parsed source file
    AST          *parser.Program           // Parsed AST
    Exports      map[string]types.Type     // Exported types
    ExportValues map[string]vm.Value       // Exported runtime values
    Namespace    vm.Value                  // Module namespace object
    Dependencies []string                  // Direct dependencies
    Error        error                     // Loading error (if any)
    LoadTime     time.Time                 // When module was loaded
    
    // Parallel processing metadata
    ParseDuration  time.Duration           // Time spent parsing
    QueueTime      time.Time               // When queued for parsing
    WorkerID       int                     // Which worker parsed this
    ParsePriority  int                     // Priority when queued
}

func (mr *ModuleRegistry) SetParsed(path string, result *ParseResult) {
    mr.mutex.Lock()
    defer mr.mutex.Unlock()
    
    record := mr.modules[path]
    if record == nil {
        record = &ModuleRecord{
            ResolvedPath: path,
            State:        ModuleLoading,
        }
        mr.modules[path] = record
    }
    
    record.AST = result.AST
    record.ParseDuration = result.ParseDuration
    record.Error = result.Error
    
    if result.Error == nil {
        record.State = ModuleParsed  // New state
    } else {
        record.State = ModuleError
    }
}
```

## Implementation Phases

### Phase 1: Foundation ✅ COMPLETE
- [x] **VFS Infrastructure**: Implement core `ModuleFS` interfaces
- [x] **Basic Resolvers**: File system and memory resolvers  
- [x] **Module Registry**: Core caching and state management with parallel processing support
- [x] **Integration Points**: Wire into existing `driver.go` and `source.go`

### Phase 2: Parallel Processing ✅ COMPLETE  
- [x] **Worker Pool**: Implement parallel lexing and parsing infrastructure
- [x] **Dependency Discovery**: BFS-based module discovery and queueing
- [x] **Priority System**: Smart prioritization for parsing order
- [x] **Parse Result Processing**: Async result handling and dependency chaining
- [x] **Performance Tests**: Benchmarks for parallel vs sequential parsing
- [x] **Module Test Infrastructure**: New `tests/modules/` with test runner for multi-file scenarios

### Phase 3: Language Support ✅ COMPLETE
- [x] **Lexer Extensions**: Add `import`/`export` tokens (IMPORT, EXPORT, FROM, DEFAULT)
- [x] **Parser Extensions**: Parse import/export statements and AST nodes
- [x] **AST Node Implementation**: Complete AST node hierarchy for all import/export variants
- [x] **Comprehensive Import Support**: All TypeScript import patterns including:
  - `import defaultExport from "module-name"`
  - `import * as name from "module-name"`
  - `import { export1 } from "module-name"`
  - `import { export1 as alias1 } from "module-name"`
  - `import { default as alias } from "module-name"`
  - `import { export1, export2 } from "module-name"`
  - `import { export1, export2 as alias2 } from "module-name"`
  - `import { "string name" as alias } from "module-name"`
  - `import defaultExport, { export1 } from "module-name"`
  - `import defaultExport, * as name from "module-name"`
  - `import "module-name"` (bare imports)
- [x] **Comprehensive Export Support**: All TypeScript export patterns
- [x] **Parser Tests**: All import/export variants parsing correctly with proper test cases

### Phase 4: Type System Integration ✅ COMPLETE
- [x] **Basic Type Checker Integration**: Handle ImportDeclaration and ExportDeclaration AST nodes
- [x] **Import Statement Processing**: Basic validation and binding of imported names to 'any' type
- [x] **Export Statement Processing**: Basic validation of export declarations and re-exports
- [x] **Export Name Extraction**: Track what declarations are being exported
- [x] **Module Environment Implementation**: Complete ModuleEnvironment with import/export tracking
- [x] **Import Type Resolution Framework**: Infrastructure for resolving imported types (ready for ModuleLoader integration)
- [x] **Export Type Analysis**: Track exported types per module with proper type information
- [x] **Module-Aware Type Checking**: Enhanced checker with module mode support
- [x] **Import/Export Binding Management**: Comprehensive binding tracking for all import/export patterns
- [x] **Full ModuleLoader Integration**: Complete integration with TypeChecker interface to avoid circular imports
- [x] **Sequential Type Checking**: Dependency-ordered type checking after parallel parsing with topological sorting
- [x] **Cross-Module Type Resolution**: Infrastructure for resolving imported types from loaded modules
- [x] **Driver Integration**: Complete ModuleLoader wiring into Driver with automatic module detection
- [x] **Default Module Mode**: Module resolution now works by default for all files

### Phase 5: Compilation & Runtime 🔄 IN PROGRESS
- [x] **Runtime Binding Tables**: Module binding resolution using compile-time metadata (following type checker patterns)
- [x] **ModuleBindings System**: Parallel to ModuleEnvironment for runtime value resolution (`pkg/compiler/module_bindings.go`)
- [x] **Compiler Integration**: Import/export statement processing integrated into compiler pipeline
- [x] **Import Resolution Framework**: Compiler detects imported identifiers and generates runtime resolution code
- [x] **Export Processing**: Direct exports, re-exports, and default exports properly tracked in bindings
- [ ] **Runtime Import Resolution**: Replace placeholder `emitImportResolve()` with actual module value lookup
- [ ] **Export Value Collection**: Collect and store exported values during module execution 
- [ ] **Module Namespace Objects**: Complete implementation for `import * as name` syntax
- [ ] **Cross-Module Value Resolution**: Enable actual imported values to be resolved from loaded modules

**Current Status:**
✅ **Core Infrastructure Complete**: ModuleBindings system working, imports tracked, no compilation panics
✅ **Compile-Time Processing**: Import/export statements processed correctly, binding tables built
🔄 **Runtime Resolution**: Currently loads `undefined` placeholder - need actual module value lookup

**Key Implementation Insight:**
Phase 5 follows the **compile-time directives approach** - no special bytecode opcodes needed. Instead:
1. **ModuleBindings** (parallel to type checker's ModuleEnvironment) maps import/export names to runtime values
2. **Compiler builds binding tables** during compilation, reusing the same AST traversal patterns as type checker  
3. **Compiler generates runtime resolution code** for imported identifiers via `emitImportResolve()`
4. **Same resolution logic** as type checker: `localName -> sourceModule.sourceName -> vm.Value`

**Test Results:**
- ✅ Module compilation: `./paserati ./test_module.ts` → `[Function: defaultGreet]`
- 🔄 Import resolution: `./paserati ./test_import.ts` → Compiles but imports resolve to `undefined`

### Phase 6: Advanced Features (Weeks 11-12)
- [ ] **Circular Dependencies**: Proper handling and error reporting
- [ ] **HTTP Resolver**: Remote module loading via URLs
- [ ] **Developer API**: Programmatic module definition interface
- [ ] **Advanced Optimizations**: Cross-module optimizations using parallel parse results

### Phase 7: Production Features (Weeks 13-14)
- [ ] **Error Handling**: Comprehensive error messages and diagnostics
- [ ] **Development Mode**: File watching and hot reloading with parallel re-parsing
- [ ] **Bundle Mode**: Static analysis and tree shaking with parallel processing
- [ ] **Performance Tuning**: Worker pool configuration and optimization
- [ ] **Documentation**: Complete API documentation and examples

## Integration Points

### 1. Driver Package (`pkg/driver/driver.go`)
```go
type Paserati struct {
    vmInstance   *vm.VM
    checker      *checker.Checker
    compiler     *compiler.Compiler
    moduleLoader *modules.ModuleLoader  // NEW
}

func (p *Paserati) LoadModule(specifier string) (*modules.ModuleRecord, error)
func (p *Paserati) AddModuleResolver(resolver modules.ModuleResolver)
```

### 2. Source Package (`pkg/source/source.go`)
```go
type SourceFile struct {
    Name       string
    Path       string
    Content    string
    ModulePath string  // NEW: Resolved module path
    IsModule   bool    // NEW: Whether this is a module
    lines      []string
}

func NewModuleSource(modulePath, content string, fs ModuleFS) *SourceFile
```

### 3. Checker Package (`pkg/checker/checker.go`)
```go
type Checker struct {
    env          *Environment
    moduleEnv    *ModuleEnvironment  // NEW: Module-aware environment
    moduleLoader *ModuleLoader       // NEW: Reference to module loader
    errors       []errors.PaseratiError
}
```

### 4. Builtin System Extension
```go
// Extend the existing initializer pattern for built-in modules
type BuiltinModuleInitializer interface {
    BuiltinInitializer
    ModuleName() string                    // Module specifier
    ModuleExports() map[string]types.Type  // Exported types
    ModuleValues() map[string]vm.Value     // Exported values
}
```

## Testing Strategy

### 1. Unit Tests
- Module resolution algorithm tests
- VFS resolver tests (filesystem, memory, HTTP)
- Import/export parsing tests
- Type checker module integration tests

### 2. Integration Tests  
- End-to-end module loading tests
- Circular dependency tests
- Cross-module type checking tests
- Runtime import/export tests

### 3. Performance Tests
- Module loading benchmarks
- Memory usage with large module graphs
- Cache effectiveness tests

### 4. Compatibility Tests
- Node.js module resolution compatibility
- TypeScript module behavior compatibility
- ES6 module specification compliance

## Example Usage

### Basic Module Usage
```typescript
// math.ts
export function add(a: number, b: number): number {
    return a + b;
}

export const PI = 3.14159;
export default class Calculator {
    compute(expr: string): number { /* ... */ }
}

// main.ts  
import Calculator, { add, PI } from './math';
import * as MathUtils from './math';

const calc = new Calculator();
const result = add(2, 3);
console.log(`Result: ${result}, PI: ${PI}`);
```

### Programmatic Module Definition
```go
// Define a virtual module in Go
builder := modules.NewModuleBuilder("my-api")
builder.ExportType("User", userType)
builder.ExportValue("createUser", createUserFunction)
builder.ExportDefault(types.String, vm.NewString("MyAPI v1.0"))

// Register with module loader
loader.AddVirtualModule("virtual:my-api", builder.Build())
```

### HTTP Module Loading
```typescript
// Load remote module
import { fetchData } from 'https://esm.sh/data-fetcher';

const data = await fetchData('https://api.example.com/users');
```

## Performance Benefits of Parallel Processing

### Sequential vs Parallel Module Loading

**Sequential Loading (Current):**
```
Module A (50ms) → Module B (40ms) → Module C (30ms) → Module D (20ms)
Total Time: 140ms + type checking (sequential)
```

**Parallel Loading (Proposed):**
```
Module A (50ms) ────┐
Module B (40ms) ────┤ → Type Checking (sequential, dependency-ordered)
Module C (30ms) ────┤    Time: ~60ms (longest parse + type checking)
Module D (20ms) ────┘
Total Time: ~110ms (20-40% improvement)
```

### Performance Characteristics

```
Project Size       │ Sequential Time │ Parallel Time │ Improvement │ Worker Count
────────────────────┼────────────────┼─────────────────┼─────────────┼─────────────
Small (5 modules)   │ 50ms           │ 35ms           │ 30%         │ 2-4
Medium (20 modules) │ 200ms          │ 80ms           │ 60%         │ 4-8
Large (100 modules) │ 1000ms         │ 300ms          │ 70%         │ 8-16
XL (500 modules)    │ 5000ms         │ 800ms          │ 84%         │ 16-32
```

### Memory Usage Optimization

**Worker Pool Benefits:**
- **Lexer Reuse**: Pre-allocated lexer instances reduce GC pressure
- **Parser Reuse**: Reusable parser instances avoid repeated initialization
- **Bounded Concurrency**: Configurable worker count prevents memory exhaustion
- **Streaming Processing**: Results processed as they complete, not all in memory

### Scalability Features

**Adaptive Worker Scaling:**
```go
func (ml *ModuleLoader) calculateOptimalWorkers(moduleCount int) int {
    cpuCount := runtime.NumCPU()
    
    if moduleCount < 10 {
        return min(2, cpuCount)  // Small projects: minimal overhead
    } else if moduleCount < 50 {
        return min(cpuCount, 8)  // Medium projects: CPU-bound
    } else {
        return min(cpuCount*2, 16)  // Large projects: I/O + CPU bound
    }
}
```

**Priority-Based Processing:**
- Entry point modules parsed first
- Frequently imported modules get higher priority
- Critical path modules processed before leaf dependencies
- Depth-based prioritization ensures dependency order

This design provides a solid foundation for implementing a production-ready module system that integrates seamlessly with Paserati's existing architecture while providing maximum flexibility through the VFS abstraction and significant performance improvements through parallel processing.
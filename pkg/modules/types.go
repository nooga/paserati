package modules

import (
	"io"
	"runtime"
	"time"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/source"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// ModuleState represents the current state of a module during loading
type ModuleState int

const (
	ModuleUnknown   ModuleState = iota // Initial state
	ModuleResolving                    // Currently resolving specifier
	ModuleResolved                     // Specifier resolved to path
	ModuleLoading                      // Currently loading source
	ModuleLoaded                       // Source loaded successfully
	ModuleParsing                      // Currently parsing
	ModuleParsed                       // Parsed successfully
	ModuleChecking                     // Currently type checking
	ModuleChecked                      // Type checked successfully
	ModuleCompiling                    // Currently compiling
	ModuleCompiled                     // Compiled successfully
	ModuleError                        // Error occurred
)

func (s ModuleState) String() string {
	switch s {
	case ModuleUnknown:
		return "unknown"
	case ModuleResolving:
		return "resolving"
	case ModuleResolved:
		return "resolved"
	case ModuleLoading:
		return "loading"
	case ModuleLoaded:
		return "loaded"
	case ModuleParsing:
		return "parsing"
	case ModuleParsed:
		return "parsed"
	case ModuleChecking:
		return "checking"
	case ModuleChecked:
		return "checked"
	case ModuleCompiling:
		return "compiling"
	case ModuleCompiled:
		return "compiled"
	case ModuleError:
		return "error"
	default:
		return "invalid"
	}
}

// ModuleRecord represents a module in the registry with all its metadata
type ModuleRecord struct {
	// Basic module information
	Specifier    string      // Original import specifier
	ResolvedPath string      // Resolved file path
	State        ModuleState // Current loading state

	// Source and parsing
	Source *source.SourceFile // Source file content
	AST    *parser.Program    // Parsed AST

	// Type information
	Exports       map[string]types.Type // Exported types
	ExportValues  map[string]vm.Value   // Exported runtime values
	ExportIndices map[string]uint16     // Export name to global heap index mapping (for dynamic import)
	Namespace     vm.Value              // Module namespace object

	// Compilation results
	CompiledChunk *vm.Chunk // Compiled bytecode chunk for execution

	// Dependencies
	Dependencies []string // Direct dependencies (module paths)
	Dependents   []string // Modules that depend on this one

	// Native module support
	nativeModule NativeModuleInterface // Native module interface for lazy initialization
	isNative     bool                  // Flag to indicate this is a native module

	// JSON module support
	IsJSON   bool     // Flag to indicate this is a JSON module
	JSONData vm.Value // Parsed JSON data (for JSON modules)

	// Error handling
	Error error // Loading/parsing/checking error

	// Timing information
	LoadTime     time.Time // When module loading started
	ParseTime    time.Time // When parsing started
	CheckTime    time.Time // When type checking started
	CompileTime  time.Time // When compilation started
	CompleteTime time.Time // When processing completed

	// Parallel processing metadata
	ParseDuration time.Duration // Time spent parsing
	CheckDuration time.Duration // Time spent type checking
	QueueTime     time.Time     // When queued for parsing
	WorkerID      int           // Which worker parsed this
	ParsePriority int           // Priority when queued for parsing
}

// ResolvedModule represents a module that has been resolved by a resolver
type ResolvedModule struct {
	Specifier    string        // Original specifier
	ResolvedPath string        // Resolved path (canonical)
	Source       io.ReadCloser // Source content (must be closed by caller)
	FS           ModuleFS      // File system context
	Resolver     string        // Name of resolver that resolved this
}

// ImportSpec represents an import declaration found during parsing
type ImportSpec struct {
	ModulePath  string            // Path to imported module
	ImportType  ImportType        // Type of import (default, named, namespace)
	ImportNames []string          // Names being imported (for named imports)
	LocalNames  []string          // Local aliases for imports
	IsDefault   bool              // Whether this imports the default export
	IsNamespace bool              // Whether this is a namespace import (import * as)
	Attributes  map[string]string // Import attributes (e.g., { type: "json" })
}

// ExportSpec represents an export declaration found during parsing
type ExportSpec struct {
	ExportName string     // Name being exported
	LocalName  string     // Local name (if different from export name)
	IsDefault  bool       // Whether this is the default export
	Type       types.Type // Type of the exported value (if known)
}

// ImportType represents the different types of import statements
type ImportType int

const (
	ImportDefault    ImportType = iota // import foo from "./module"
	ImportNamed                        // import { foo, bar } from "./module"
	ImportNamespace                    // import * as foo from "./module"
	ImportSideEffect                   // import "./module" (side effects only)
)

func (it ImportType) String() string {
	switch it {
	case ImportDefault:
		return "default"
	case ImportNamed:
		return "named"
	case ImportNamespace:
		return "namespace"
	case ImportSideEffect:
		return "side-effect"
	default:
		return "unknown"
	}
}

// ParseJob represents a module parsing task for the worker pool
type ParseJob struct {
	ModulePath   string             // Module path to parse
	Source       *source.SourceFile // Source content
	Priority     int                // Job priority (0 = highest)
	Dependencies []string           // Known dependencies
	Timestamp    time.Time          // When job was created
	RetryCount   int                // Number of times this job has been retried
}

// ParseResult represents the result of parsing a module
type ParseResult struct {
	ModulePath    string          // Module path that was parsed
	AST           *parser.Program // Parsed AST
	ImportSpecs   []*ImportSpec   // Discovered imports
	ExportSpecs   []*ExportSpec   // Discovered exports
	ParseDuration time.Duration   // Time taken to parse
	WorkerID      int             // ID of worker that parsed this
	Error         error           // Parse error (if any)
	Timestamp     time.Time       // When parsing completed
}

// LoaderConfig configures module loader behavior
type LoaderConfig struct {
	// Parallel processing settings
	EnableParallel   bool          // Whether to use parallel processing
	NumWorkers       int           // Number of parser workers (0 = auto)
	JobBufferSize    int           // Size of job queue buffer
	ResultBufferSize int           // Size of result channel buffer
	MaxParseTime     time.Duration // Timeout for individual parses

	// Caching settings
	CacheEnabled bool          // Whether to cache modules
	CacheSize    int           // Maximum number of cached modules (0 = unlimited)
	CacheTTL     time.Duration // Time-to-live for cached modules (0 = no expiry)

	// Resolution settings
	ResolveTimeout time.Duration // Timeout for module resolution
	MaxDepth       int           // Maximum dependency depth (0 = unlimited)

	// Performance settings
	PrewarmLexers bool // Pre-allocate lexer instances
	ReuseAST      bool // Reuse AST node pools

	// Type checking settings
	IgnoreTypeErrors bool // When true, type errors don't stop module loading
}

// DefaultLoaderConfig returns sensible default configuration
func DefaultLoaderConfig() *LoaderConfig {
	return &LoaderConfig{
		EnableParallel:   true,
		NumWorkers:       runtime.NumCPU(), // Use all available CPUs
		JobBufferSize:    100,
		ResultBufferSize: 100,
		MaxParseTime:     30 * time.Second,

		CacheEnabled: true,
		CacheSize:    0, // Unlimited
		CacheTTL:     0, // No expiry

		ResolveTimeout: 10 * time.Second,
		MaxDepth:       100,

		PrewarmLexers: true,
		ReuseAST:      false, // Start with false for simplicity
	}
}

// WorkerPoolStats contains statistics about worker pool performance
type WorkerPoolStats struct {
	TotalJobs     int           // Total jobs processed
	ActiveJobs    int           // Currently active jobs
	CompletedJobs int           // Successfully completed jobs
	FailedJobs    int           // Failed jobs
	AverageTime   time.Duration // Average processing time per job
	TotalTime     time.Duration // Total time spent processing
	WorkerCount   int           // Number of active workers
}

// RegistryStats contains statistics about the module registry
type RegistryStats struct {
	TotalModules  int   // Total modules in registry
	LoadedModules int   // Modules successfully loaded
	FailedModules int   // Modules that failed to load
	CacheHits     int   // Number of cache hits
	CacheMisses   int   // Number of cache misses
	MemoryUsage   int64 // Approximate memory usage in bytes
}

// LoaderStats contains overall statistics about module loading
type LoaderStats struct {
	WorkerPool      WorkerPoolStats // Worker pool statistics
	Registry        RegistryStats   // Registry statistics
	AverageLoadTime time.Duration   // Average time to load a module
	TotalLoadTime   time.Duration   // Total time spent loading modules
}

// VM interface methods for ModuleRecord
// These methods implement the vm.ModuleRecord interface to avoid circular imports

// GetExportValues returns the exported runtime values from this module
func (mr *ModuleRecord) GetExportValues() map[string]vm.Value {
	if mr.ExportValues == nil {
		return make(map[string]vm.Value)
	}
	return mr.ExportValues
}

// GetExportIndices returns the export name to global index mapping
func (mr *ModuleRecord) GetExportIndices() map[string]uint16 {
	if mr.ExportIndices == nil {
		return make(map[string]uint16)
	}
	return mr.ExportIndices
}

// IsNativeModule returns true if this is a native module
func (mr *ModuleRecord) IsNativeModule() bool {
	return mr.nativeModule != nil
}

// GetNativeModule returns the native module interface if this is a native module
func (mr *ModuleRecord) GetNativeModule() NativeModuleInterface {
	return mr.nativeModule
}

// GetCompiledChunk returns the compiled bytecode chunk for this module
func (mr *ModuleRecord) GetCompiledChunk() *vm.Chunk {
	return mr.CompiledChunk
}

// GetExportNames returns the names of all exports from this module
func (mr *ModuleRecord) GetExportNames() []string {
	names := make([]string, 0, len(mr.Exports))
	for name := range mr.Exports {
		names = append(names, name)
	}
	return names
}

// GetError returns the error associated with this module record
func (mr *ModuleRecord) GetError() error {
	return mr.Error
}

// IsJSONModule returns true if this is a JSON module
func (mr *ModuleRecord) IsJSONModule() bool {
	return mr.IsJSON
}

// GetSource returns the source content of the module
func (mr *ModuleRecord) GetSource() string {
	if mr.Source == nil {
		return ""
	}
	return mr.Source.Content
}

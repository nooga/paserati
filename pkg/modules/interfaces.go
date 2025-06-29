package modules

import (
	"context"
	"io/fs"
	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// ModuleFS extends Go's standard io/fs interfaces for module loading
type ModuleFS interface {
	fs.FS
	fs.ReadFileFS // Required for reading module content
}

// WritableModuleFS extends ModuleFS for development scenarios where modules can be written
type WritableModuleFS interface {
	ModuleFS
	WriteFile(name string, data []byte, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
}

// ModuleResolver resolves module specifiers to concrete modules
type ModuleResolver interface {
	// Name returns a human-readable name for this resolver
	Name() string
	
	// CanResolve returns true if this resolver can handle the given specifier
	CanResolve(specifier string) bool
	
	// Resolve attempts to resolve a module specifier to a concrete module
	// fromPath is the path of the module that is importing (for relative resolution)
	Resolve(specifier string, fromPath string) (*ResolvedModule, error)
	
	// Priority returns the priority of this resolver (lower = higher priority)
	Priority() int
}

// ModuleLoader is the main interface for loading modules
type ModuleLoader interface {
	// LoadModule loads a module and all its dependencies
	LoadModule(specifier string, fromPath string) (*ModuleRecord, error)
	
	// LoadModuleParallel loads a module using parallel processing
	LoadModuleParallel(specifier string, fromPath string) (*ModuleRecord, error)
	
	// AddResolver adds a module resolver to the chain
	AddResolver(resolver ModuleResolver)
	
	// SetCheckerFactory sets the factory function for creating type checkers
	SetCheckerFactory(factory func() TypeChecker)
	
	// GetModule retrieves a cached module record
	GetModule(specifier string) *ModuleRecord
	
	// ClearCache clears the module cache
	ClearCache()
	
	// GetStats returns loader statistics
	GetStats() LoaderStats
	
	// GetDependencyStats returns dependency analysis statistics
	GetDependencyStats() DependencyStats
}

// ModuleRegistry manages the cache of loaded modules
type ModuleRegistry interface {
	// Get retrieves a module record by specifier
	Get(specifier string) *ModuleRecord
	
	// Set stores a module record
	Set(specifier string, record *ModuleRecord)
	
	// SetParsed updates a module record with parse results
	SetParsed(specifier string, result *ParseResult)
	
	// Remove removes a module from the cache
	Remove(specifier string)
	
	// Clear clears all cached modules
	Clear()
	
	// List returns all cached module specifiers
	List() []string
	
	// Size returns the number of cached modules
	Size() int
	
	// GetStats returns registry statistics
	GetStats() RegistryStats
}

// ParseWorkerPool manages parallel parsing of modules
type ParseWorkerPool interface {
	// Start initializes the worker pool
	Start(ctx context.Context, numWorkers int) error
	
	// Submit submits a parse job to the worker pool
	Submit(job *ParseJob) error
	
	// Results returns a channel of parse results
	Results() <-chan *ParseResult
	
	// Errors returns a channel of parse errors
	Errors() <-chan error
	
	// Shutdown gracefully shuts down the worker pool
	Shutdown(ctx context.Context) error
	
	// HasActiveJobs returns true if there are jobs in progress
	HasActiveJobs() bool
	
	// GetStats returns worker pool statistics
	GetStats() WorkerPoolStats
}

// TypeChecker interface for type checking modules
type TypeChecker interface {
	EnableModuleMode(modulePath string, loader ModuleLoader)
	Check(program *parser.Program) []errors.PaseratiError
	IsModuleMode() bool
	GetModuleExports() map[string]types.Type
}

// DependencyAnalyzer tracks module dependencies during loading
type DependencyAnalyzer interface {
	// MarkDiscovered marks a module as discovered
	MarkDiscovered(modulePath string)
	
	// IsDiscovered returns true if a module has been discovered
	IsDiscovered(modulePath string) bool
	
	// Parse tracking
	MarkParsing(modulePath string)
	MarkParsed(modulePath string, result *ParseResult)
	IsParsing(modulePath string) bool
	GetParseResult(modulePath string) *ParseResult
	
	// GetDependencyDepth returns how deep a module is in the dependency tree
	GetDependencyDepth(modulePath string) int
	
	// GetImportCount returns how many times a module is imported
	GetImportCount(modulePath string) int
	
	// AddDependency adds a dependency relationship
	AddDependency(from, to string)
	
	// GetDependencies returns all dependencies of a module
	GetDependencies(modulePath string) []string
	
	// GetTopologicalOrder returns modules in dependency-order for type checking
	GetTopologicalOrder() ([]string, error)
	
	// Statistics
	GetStats() DependencyStats
	
	// Clear resets the analyzer state
	Clear()
}
package modules

import (
	"context"
	"fmt"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// workerPool implements ParseWorkerPool interface
type workerPool struct {
	// Configuration
	numWorkers   int
	jobBuffer    int
	resultBuffer int
	
	// Channels
	jobQueue   chan *ParseJob
	resultChan chan *ParseResult
	errorChan  chan error
	
	// Control
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	workers    []*parseWorker
	
	// State
	started    int32 // atomic
	stopped    int32 // atomic
	activeJobs int32 // atomic
	
	// Statistics
	stats      WorkerPoolStats
	statsMutex sync.RWMutex
}

// parseWorker represents a single worker goroutine
type parseWorker struct {
	id         int
	pool       *workerPool
	jobQueue   <-chan *ParseJob
	resultChan chan<- *ParseResult
	errorChan  chan<- error
	
	// Real lexer and parser instances
	lexerInstance  *lexer.Lexer
	parserInstance *parser.Parser
}

// MockLexer interface for testing parallel processing without real lexer
type MockLexer interface {
	Reset(source string)
	Tokenize() ([]MockToken, error)
}

// MockParser interface for testing parallel processing without real parser
type MockParser interface {
	Reset(tokens []MockToken)
	Parse() (*MockAST, error)
}

// MockToken represents a token for testing
type MockToken struct {
	Type  string
	Value string
	Line  int
	Col   int
}

// MockAST represents an AST for testing
type MockAST struct {
	Type     string
	Imports  []*ImportSpec
	Exports  []*ExportSpec
	Children []*MockAST
}

// NewWorkerPool creates a new parallel parsing worker pool
func NewWorkerPool(config *LoaderConfig) ParseWorkerPool {
	numWorkers := config.NumWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	
	return &workerPool{
		numWorkers:   numWorkers,
		jobBuffer:    config.JobBufferSize,
		resultBuffer: config.ResultBufferSize,
	}
}

// Start initializes and starts the worker pool
func (wp *workerPool) Start(ctx context.Context, numWorkers int) error {
	if !atomic.CompareAndSwapInt32(&wp.started, 0, 1) {
		return fmt.Errorf("worker pool already started")
	}
	
	if numWorkers > 0 {
		wp.numWorkers = numWorkers
	}
	
	// Create context with cancellation
	wp.ctx, wp.cancel = context.WithCancel(ctx)
	
	// Initialize channels
	wp.jobQueue = make(chan *ParseJob, wp.jobBuffer)
	wp.resultChan = make(chan *ParseResult, wp.resultBuffer)
	wp.errorChan = make(chan error, wp.numWorkers)
	
	// Initialize statistics
	wp.stats = WorkerPoolStats{
		WorkerCount: wp.numWorkers,
	}
	
	// Start workers
	wp.workers = make([]*parseWorker, wp.numWorkers)
	for i := 0; i < wp.numWorkers; i++ {
		worker := &parseWorker{
			id:         i,
			pool:       wp,
			jobQueue:   wp.jobQueue,
			resultChan: wp.resultChan,
			errorChan:  wp.errorChan,
			// Lexer and parser will be created per job
		}
		
		wp.workers[i] = worker
		wp.wg.Add(1)
		go worker.run(wp.ctx)
	}
	
	return nil
}

// Submit submits a parse job to the worker pool
func (wp *workerPool) Submit(job *ParseJob) error {
	if atomic.LoadInt32(&wp.started) == 0 {
		return fmt.Errorf("worker pool not started")
	}
	
	if atomic.LoadInt32(&wp.stopped) == 1 {
		return fmt.Errorf("worker pool stopped")
	}
	
	select {
	case wp.jobQueue <- job:
		atomic.AddInt32(&wp.activeJobs, 1)
		
		// Update statistics
		wp.statsMutex.Lock()
		wp.stats.TotalJobs++
		wp.stats.ActiveJobs++
		wp.statsMutex.Unlock()
		
		return nil
	case <-wp.ctx.Done():
		return wp.ctx.Err()
	}
}

// Results returns a channel of parse results
func (wp *workerPool) Results() <-chan *ParseResult {
	return wp.resultChan
}

// Errors returns a channel of parse errors
func (wp *workerPool) Errors() <-chan error {
	return wp.errorChan
}

// Shutdown gracefully shuts down the worker pool
func (wp *workerPool) Shutdown(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&wp.stopped, 0, 1) {
		return fmt.Errorf("worker pool already stopped")
	}
	
	// Close job queue to signal workers to stop
	close(wp.jobQueue)
	
	// Wait for workers to finish or context timeout
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		// All workers finished gracefully
		wp.cancel()
		close(wp.resultChan)
		close(wp.errorChan)
		return nil
	case <-ctx.Done():
		// Timeout - force shutdown
		wp.cancel()
		return ctx.Err()
	}
}

// HasActiveJobs returns true if there are jobs in progress
func (wp *workerPool) HasActiveJobs() bool {
	return atomic.LoadInt32(&wp.activeJobs) > 0
}

// GetStats returns current worker pool statistics
func (wp *workerPool) GetStats() WorkerPoolStats {
	wp.statsMutex.RLock()
	defer wp.statsMutex.RUnlock()
	
	stats := wp.stats
	stats.ActiveJobs = int(atomic.LoadInt32(&wp.activeJobs))
	return stats
}

// run is the main worker loop
func (w *parseWorker) run(ctx context.Context) {
	defer w.pool.wg.Done()
	// Worker started
	
	for {
		select {
		case job, ok := <-w.jobQueue:
			if !ok {
				// Job queue closed, worker should stop
				// Job queue closed, worker should stop
				return
			}
			// Process the job
			
			result := w.processJob(job)
			
			// Update statistics
			w.pool.statsMutex.Lock()
			if result.Error == nil {
				w.pool.stats.CompletedJobs++
			} else {
				w.pool.stats.FailedJobs++
			}
			w.pool.stats.TotalTime += result.ParseDuration
			if w.pool.stats.CompletedJobs+w.pool.stats.FailedJobs > 0 {
				w.pool.stats.AverageTime = w.pool.stats.TotalTime / time.Duration(w.pool.stats.CompletedJobs+w.pool.stats.FailedJobs)
			}
			w.pool.statsMutex.Unlock()
			
			// Decrement active jobs count
			atomic.AddInt32(&w.pool.activeJobs, -1)
			
			// Send result
			select {
			case w.resultChan <- result:
				// Result sent successfully
			case <-ctx.Done():
				return
			}
			
		case <-ctx.Done():
			return
		}
	}
}

// processJob processes a single parse job
func (w *parseWorker) processJob(job *ParseJob) *ParseResult {
	startTime := time.Now()
	
	result := &ParseResult{
		ModulePath: job.ModulePath,
		WorkerID:   w.id,
		Timestamp:  startTime,
	}
	
	// Use real lexer and parser
	w.lexerInstance = lexer.NewLexerWithSource(job.Source)
	w.parserInstance = parser.NewParser(w.lexerInstance)
	
	// Parse the module
	program, parseErrs := w.parserInstance.ParseProgram()
	if len(parseErrs) > 0 {
		// Take the first error
		result.Error = fmt.Errorf("parsing failed: %s", parseErrs[0].Error())
		result.ParseDuration = time.Since(startTime)
		return result
	}
	
	// Store the AST
	result.AST = program
	
	// Extract import/export specifications from real AST
	result.ImportSpecs = extractImportSpecs(program)
	result.ExportSpecs = extractExportSpecs(program)
	
	result.ParseDuration = time.Since(startTime)
	return result
}

// extractImportSpecs extracts import specifications from the AST
// This includes both import statements and re-export statements with 'from' clauses
func extractImportSpecs(program *parser.Program) []*ImportSpec {
	var specs []*ImportSpec
	
	for _, stmt := range program.Statements {
		switch node := stmt.(type) {
		case *parser.ImportDeclaration:
			// Regular import statements
			if node.Source != nil {
				spec := &ImportSpec{
					ModulePath: node.Source.Value,
					Attributes: node.Attributes, // Capture import attributes (e.g., type: "json")
				}
				specs = append(specs, spec)
			}
		case *parser.ExportNamedDeclaration:
			// Re-export statements: export { name } from "module"
			if node.Source != nil {
				spec := &ImportSpec{
					ModulePath: node.Source.Value,
				}
				specs = append(specs, spec)
			}
		case *parser.ExportAllDeclaration:
			// Re-export all statements: export * from "module"
			if node.Source != nil {
				spec := &ImportSpec{
					ModulePath: node.Source.Value,
				}
				specs = append(specs, spec)
			}
		}
	}
	
	return specs
}

// extractExportSpecs extracts export specifications from the AST
func extractExportSpecs(program *parser.Program) []*ExportSpec {
	var specs []*ExportSpec
	
	for _, stmt := range program.Statements {
		switch node := stmt.(type) {
		case *parser.ExportNamedDeclaration:
			// Handle named exports
			for _, specifier := range node.Specifiers {
				if namedSpec, ok := specifier.(*parser.ExportNamedSpecifier); ok {
					// Get export name (can be Identifier or StringLiteral)
					var exportName, localName string
					if ident, ok := namedSpec.Exported.(*parser.Identifier); ok {
						exportName = ident.Value
					} else if strLit, ok := namedSpec.Exported.(*parser.StringLiteral); ok {
						exportName = strLit.Value
					}
					if ident, ok := namedSpec.Local.(*parser.Identifier); ok {
						localName = ident.Value
					} else if strLit, ok := namedSpec.Local.(*parser.StringLiteral); ok {
						localName = strLit.Value
					}
					spec := &ExportSpec{
						ExportName: exportName,
						LocalName:  localName,
					}
					specs = append(specs, spec)
				}
			}
		case *parser.ExportDefaultDeclaration:
			// Handle default export
			spec := &ExportSpec{
				ExportName: "default",
				IsDefault:  true,
			}
			specs = append(specs, spec)
		}
	}
	
	return specs
}

// simpleMockLexer is a basic mock lexer for testing
type simpleMockLexer struct {
	content string
}

func (sml *simpleMockLexer) Reset(source string) {
	sml.content = source
}

func (sml *simpleMockLexer) Tokenize() ([]MockToken, error) {
	// Simple tokenization for testing
	// Look for import/export keywords
	tokens := []MockToken{}
	
	if contains(sml.content, "import") {
		tokens = append(tokens, MockToken{Type: "IMPORT", Value: "import", Line: 1, Col: 1})
	}
	
	if contains(sml.content, "export") {
		tokens = append(tokens, MockToken{Type: "EXPORT", Value: "export", Line: 1, Col: 1})
	}
	
	if contains(sml.content, "function") {
		tokens = append(tokens, MockToken{Type: "FUNCTION", Value: "function", Line: 1, Col: 1})
	}
	
	if contains(sml.content, "const") {
		tokens = append(tokens, MockToken{Type: "CONST", Value: "const", Line: 1, Col: 1})
	}
	
	// Simulate some processing time
	time.Sleep(1 * time.Millisecond)
	
	return tokens, nil
}

// simpleMockParser is a basic mock parser for testing
type simpleMockParser struct {
	tokens []MockToken
}

func (smp *simpleMockParser) Reset(tokens []MockToken) {
	smp.tokens = tokens
}

func (smp *simpleMockParser) Parse() (*MockAST, error) {
	ast := &MockAST{
		Type:     "Program",
		Imports:  []*ImportSpec{},
		Exports:  []*ExportSpec{},
		Children: []*MockAST{},
	}
	
	// Simple parsing logic for testing
	for _, token := range smp.tokens {
		switch token.Type {
		case "IMPORT":
			// For testing, only create mock imports if we can parse an actual module path
			// This prevents infinite loops when testing basic functionality
			// TODO: Replace with real import parsing in the future
			continue
			
		case "EXPORT":
			// Create a mock export
			exportSpec := &ExportSpec{
				ExportName: "mockExport",
				LocalName:  "mockExport",
				IsDefault:  false,
			}
			ast.Exports = append(ast.Exports, exportSpec)
		}
	}
	
	// Simulate some processing time
	time.Sleep(2 * time.Millisecond)
	
	return ast, nil
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && 
			(s[:len(substr)] == substr || 
			 s[len(s)-len(substr):] == substr ||
			 containsHelper(s, substr))))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
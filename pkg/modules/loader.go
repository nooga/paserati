package modules

import (
	"context"
	"fmt"
	"paserati/pkg/source"
	"sort"
	"sync"
	"time"
)

// moduleLoader implements ModuleLoader interface
type moduleLoader struct {
	resolvers    []ModuleResolver
	registry     ModuleRegistry
	config       *LoaderConfig
	
	// Parallel processing components
	workerPool   ParseWorkerPool
	parseQueue   *parseQueue
	depAnalyzer  DependencyAnalyzer
	
	// State
	mutex        sync.RWMutex
	initialized  bool
}

// NewModuleLoader creates a new module loader
func NewModuleLoader(config *LoaderConfig, resolvers ...ModuleResolver) ModuleLoader {
	if config == nil {
		config = DefaultLoaderConfig()
	}
	
	// Sort resolvers by priority (lower = higher priority)
	sort.Slice(resolvers, func(i, j int) bool {
		return resolvers[i].Priority() < resolvers[j].Priority()
	})
	
	return &moduleLoader{
		resolvers:   resolvers,
		registry:    NewRegistry(config),
		config:      config,
		depAnalyzer: NewDependencyAnalyzer(),
	}
}

// LoadModule loads a module using sequential processing
func (ml *moduleLoader) LoadModule(specifier string, fromPath string) (*ModuleRecord, error) {
	// Use sequential loading for now
	return ml.loadModuleSequential(specifier, fromPath)
}

// LoadModuleParallel loads a module using parallel processing
func (ml *moduleLoader) LoadModuleParallel(specifier string, fromPath string) (*ModuleRecord, error) {
	ml.mutex.Lock()
	if !ml.initialized {
		err := ml.initializeParallelComponents()
		if err != nil {
			ml.mutex.Unlock()
			return nil, fmt.Errorf("failed to initialize parallel components: %w", err)
		}
		ml.initialized = true
	}
	ml.mutex.Unlock()
	
	// Start the parallel loading process
	return ml.loadModuleParallelImpl(specifier, fromPath)
}

// loadModuleSequential implements sequential module loading
func (ml *moduleLoader) loadModuleSequential(specifier string, fromPath string) (*ModuleRecord, error) {
	// Check cache first
	if record := ml.registry.Get(specifier); record != nil {
		return record, nil
	}
	
	// Resolve the module
	resolved, err := ml.resolveModule(specifier, fromPath)
	if err != nil {
		return nil, err
	}
	
	// Create module record
	record := &ModuleRecord{
		Specifier:    specifier,
		ResolvedPath: resolved.ResolvedPath,
		State:        ModuleResolved,
		LoadTime:     time.Now(),
	}
	
	// Store in registry
	ml.registry.Set(specifier, record)
	
	// For now, just mark as loaded (we'll implement actual parsing later)
	record.State = ModuleLoaded
	record.CompleteTime = time.Now()
	
	return record, nil
}

// loadModuleParallelImpl implements parallel module loading
func (ml *moduleLoader) loadModuleParallelImpl(specifier string, fromPath string) (*ModuleRecord, error) {
	// Initialize parse queue and start discovery
	ml.parseQueue = NewParseQueue(ml.config.JobBufferSize)
	
	// Create context for the loading operation
	ctx, cancel := context.WithTimeout(context.Background(), ml.config.ResolveTimeout)
	defer cancel()
	
	// Start the worker pool
	err := ml.workerPool.Start(ctx, ml.config.NumWorkers)
	if err != nil {
		return nil, fmt.Errorf("failed to start worker pool: %w", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		ml.workerPool.Shutdown(shutdownCtx)
	}()
	
	// Queue the entry point for parsing
	entryJob, err := ml.createParseJob(specifier, fromPath, 0)
	if err != nil {
		return nil, err
	}
	
	// Mark the entry point as discovered
	ml.depAnalyzer.MarkDiscovered(entryJob.ModulePath)
	
	err = ml.parseQueue.Enqueue(entryJob)
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue entry point: %w", err)
	}
	
	// Submit initial job to worker pool
	err = ml.workerPool.Submit(entryJob)
	if err != nil {
		return nil, fmt.Errorf("failed to submit initial job: %w", err)
	}
	
	// Main processing loop
	for !ml.parseQueue.IsEmpty() || ml.workerPool.HasActiveJobs() {
		select {
		case result := <-ml.workerPool.Results():
			err := ml.processParseResult(result)
			if err != nil {
				return nil, err
			}
			
		case err := <-ml.workerPool.Errors():
			return nil, fmt.Errorf("worker error: %w", err)
			
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		
		// Safety check: if no new dependencies are discovered, break the loop
		// This prevents infinite waiting when there are no more jobs to process
		if ml.parseQueue.IsEmpty() && !ml.workerPool.HasActiveJobs() {
			break
		}
	}
	
	// Return the main module record
	return ml.registry.Get(specifier), nil
}

// resolveModule resolves a module specifier using the resolver chain
func (ml *moduleLoader) resolveModule(specifier string, fromPath string) (*ResolvedModule, error) {
	for _, resolver := range ml.resolvers {
		if resolver.CanResolve(specifier) {
			resolved, err := resolver.Resolve(specifier, fromPath)
			if err == nil {
				return resolved, nil
			}
			// Continue to next resolver if this one fails
		}
	}
	
	return nil, fmt.Errorf("no resolver could handle specifier: %s", specifier)
}

// createParseJob creates a parse job for a module
func (ml *moduleLoader) createParseJob(specifier string, fromPath string, priority int) (*ParseJob, error) {
	// Resolve the module
	resolved, err := ml.resolveModule(specifier, fromPath)
	if err != nil {
		return nil, err
	}
	
	// Read the source content
	defer resolved.Source.Close()
	
	content := make([]byte, 0, 1024)
	buf := make([]byte, 512)
	for {
		n, err := resolved.Source.Read(buf)
		if n > 0 {
			content = append(content, buf[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("failed to read source: %w", err)
		}
	}
	
	// Create source file using the real source package
	sourceFile := &source.SourceFile{
		Name:    resolved.ResolvedPath,
		Path:    resolved.ResolvedPath,
		Content: string(content),
	}
	
	return &ParseJob{
		ModulePath: resolved.ResolvedPath,
		Source:     sourceFile,
		Priority:   priority,
		Timestamp:  time.Now(),
	}, nil
}

// processParseResult processes a parse result and queues dependencies
func (ml *moduleLoader) processParseResult(result *ParseResult) error {
	// Mark as completed in queue
	ml.parseQueue.MarkCompleted(result.ModulePath, result)
	
	// Update registry
	ml.registry.SetParsed(result.ModulePath, result)
	
	// Mark as parsed in dependency analyzer
	ml.depAnalyzer.MarkParsed(result.ModulePath, result)
	
	if result.Error != nil {
		return nil // Don't process dependencies if parsing failed
	}
	
	// Queue dependencies for parsing
	for _, importSpec := range result.ImportSpecs {
		if !ml.depAnalyzer.IsDiscovered(importSpec.ModulePath) {
			ml.depAnalyzer.MarkDiscovered(importSpec.ModulePath)
			
			// Calculate priority for dependency
			priority := ml.calculatePriority(importSpec.ModulePath, result.ModulePath)
			
			// Create and queue parse job
			job, err := ml.createParseJob(importSpec.ModulePath, result.ModulePath, priority)
			if err != nil {
				// Log error but continue with other dependencies
				continue
			}
			
			err = ml.parseQueue.Enqueue(job)
			if err != nil {
				continue
			}
			
			// Submit to worker pool
			err = ml.workerPool.Submit(job)
			if err != nil {
				continue
			}
			
			// Add dependency relationship
			ml.depAnalyzer.AddDependency(result.ModulePath, importSpec.ModulePath)
		}
	}
	
	return nil
}

// calculatePriority calculates the priority for a module
func (ml *moduleLoader) calculatePriority(modulePath, dependentPath string) int {
	depth := ml.depAnalyzer.GetDependencyDepth(modulePath)
	importCount := ml.depAnalyzer.GetImportCount(modulePath)
	
	// Base priority from depth
	priority := depth * 10
	
	// Boost priority for frequently imported modules
	frequencyBoost := max(0, 5-importCount)
	priority -= frequencyBoost
	
	// Ensure minimum priority of 1
	if priority < 1 {
		priority = 1
	}
	
	return priority
}

// initializeParallelComponents initializes the parallel processing components
func (ml *moduleLoader) initializeParallelComponents() error {
	// Initialize worker pool
	ml.workerPool = NewWorkerPool(ml.config)
	
	return nil
}

// AddResolver adds a module resolver to the chain
func (ml *moduleLoader) AddResolver(resolver ModuleResolver) {
	ml.mutex.Lock()
	defer ml.mutex.Unlock()
	
	ml.resolvers = append(ml.resolvers, resolver)
	
	// Re-sort by priority
	sort.Slice(ml.resolvers, func(i, j int) bool {
		return ml.resolvers[i].Priority() < ml.resolvers[j].Priority()
	})
}

// GetModule retrieves a cached module record
func (ml *moduleLoader) GetModule(specifier string) *ModuleRecord {
	return ml.registry.Get(specifier)
}

// ClearCache clears the module cache
func (ml *moduleLoader) ClearCache() {
	ml.registry.Clear()
	ml.depAnalyzer.Clear()
	
	ml.mutex.Lock()
	defer ml.mutex.Unlock()
	
	if ml.parseQueue != nil {
		ml.parseQueue.Clear()
	}
}

// GetStats returns loader statistics
func (ml *moduleLoader) GetStats() LoaderStats {
	stats := LoaderStats{
		Registry: ml.registry.GetStats(),
	}
	
	if ml.workerPool != nil {
		stats.WorkerPool = ml.workerPool.GetStats()
	}
	
	// Calculate average load time
	registryStats := ml.registry.GetStats()
	if registryStats.LoadedModules > 0 {
		// This is a rough approximation - would need to track actual load times
		stats.AverageLoadTime = time.Duration(registryStats.LoadedModules) * time.Millisecond
	}
	
	return stats
}

// GetDependencyStats returns dependency analysis statistics
func (ml *moduleLoader) GetDependencyStats() DependencyStats {
	if da, ok := ml.depAnalyzer.(*dependencyAnalyzer); ok {
		return da.GetStats()
	}
	return DependencyStats{}
}


// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
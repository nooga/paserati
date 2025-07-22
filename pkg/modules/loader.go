package modules

import (
	"context"
	"fmt"
	"io"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/source"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"sort"
	"sync"
	"time"
)

const moduleLoaderDebug = true

func debugPrintf(format string, args ...interface{}) {
	if moduleLoaderDebug {
		fmt.Printf(format, args...)
	}
}

// moduleLoader implements ModuleLoader interface
type moduleLoader struct {
	resolvers    []ModuleResolver
	registry     ModuleRegistry
	config       *LoaderConfig
	
	// Parallel processing components
	workerPool   ParseWorkerPool
	parseQueue   *parseQueue
	depAnalyzer  DependencyAnalyzer
	
	// Type checking and compilation
	checkerFactory func() TypeChecker
	compilerFactory func() Compiler
	
	// VM instance for native module initialization
	vmInstance *vm.VM
	
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
func (ml *moduleLoader) LoadModule(specifier string, fromPath string) (vm.ModuleRecord, error) {
	debugPrintf("// [ModuleLoader] LoadModule called: %s from %s\n", specifier, fromPath)
	// Use sequential loading for now
	result, err := ml.loadModuleSequential(specifier, fromPath)
	debugPrintf("// [ModuleLoader] LoadModule finished: %s, error=%v\n", specifier, err)
	return result, err
}

// LoadModuleParallel loads a module using parallel processing
func (ml *moduleLoader) LoadModuleParallel(specifier string, fromPath string) (vm.ModuleRecord, error) {
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
func (ml *moduleLoader) loadModuleSequential(specifier string, fromPath string) (vm.ModuleRecord, error) {
	debugPrintf("// [ModuleLoader] loadModuleSequential START: %s from %s\n", specifier, fromPath)
	// Check cache first
	cachedRecord := ml.registry.Get(specifier)
	debugPrintf("// [ModuleLoader] Cache check for %s: %v\n", specifier, cachedRecord != nil)
	if cachedRecord != nil {
		debugPrintf("// [ModuleLoader] Returning cached module: %s (has error: %v)\n", specifier, cachedRecord.GetError() != nil)
		return cachedRecord, nil
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
	debugPrintf("// [ModuleLoader] Storing in registry: specifier=%s, resolvedPath=%s\n", specifier, record.ResolvedPath)
	ml.registry.Set(specifier, record)
	
	// Actually parse the module
	err = ml.parseModuleSequential(record, resolved)
	if err != nil {
		record.Error = err
		record.State = ModuleError
		debugPrintf("// [ModuleLoader] loadModuleSequential EARLY RETURN (parse error): %s - %v\n", specifier, err)
		return record, nil // Return record with error, don't fail completely
	}
	
	// Extract and load dependencies before type checking
	debugPrintf("// [ModuleLoader] About to extract imports for: %s (AST=%v)\n", specifier, record.AST != nil)
	importSpecs := extractImportSpecs(record.AST)
	debugPrintf("// [ModuleLoader] Found %d import specs in %s\n", len(importSpecs), record.ResolvedPath)
	
	// Load dependencies recursively
	for _, importSpec := range importSpecs {
		debugPrintf("// [ModuleLoader] Loading dependency: %s from %s\n", importSpec.ModulePath, record.ResolvedPath)
		debugPrintf("// [ModuleLoader] About to make recursive call for dependency\n")
		_, err := ml.loadModuleSequential(importSpec.ModulePath, record.ResolvedPath)
		if err != nil {
			debugPrintf("// [ModuleLoader] Failed to load dependency %s: %v\n", importSpec.ModulePath, err)
			// Continue with other dependencies rather than failing completely
		} else {
			debugPrintf("// [ModuleLoader] Successfully loaded dependency: %s\n", importSpec.ModulePath)
		}
	}
	
	debugPrintf("// [ModuleLoader] Finished loading dependencies for: %s\n", specifier)
	
	// Add type checking and compilation to sequential loading
	debugPrintf("// [ModuleLoader] Sequential loading checkerFactory: %v, compilerFactory: %v\n", 
		ml.checkerFactory != nil, ml.compilerFactory != nil)
	if ml.checkerFactory != nil {
		// Type check the module
		record.State = ModuleChecking
		record.CheckTime = time.Now()
		
		moduleChecker := ml.checkerFactory()
		moduleChecker.EnableModuleMode(record.ResolvedPath, ml)
		
		checkErrors := moduleChecker.Check(record.AST)
		if len(checkErrors) > 0 {
			record.Error = fmt.Errorf("type checking failed: %s", checkErrors[0].Error())
			record.State = ModuleError
			debugPrintf("// [ModuleLoader] loadModuleSequential EARLY RETURN (type check error): %s\n", specifier)
			return record, nil
		}
		
		// Extract exported types
		if moduleChecker.IsModuleMode() {
			// Skip type extraction for native modules - they already have their exports set
			if !record.isNative {
				record.Exports = moduleChecker.GetModuleExports()
				debugPrintf("// [ModuleLoader] Extracted %d exports from module %s\n", len(record.Exports), record.ResolvedPath)
				for name, typ := range record.Exports {
					debugPrintf("// [ModuleLoader]   Export '%s': %s\n", name, typ.String())
				}
			} else {
				debugPrintf("// [ModuleLoader] Skipping export extraction for native module %s (already has %d exports)\n", record.ResolvedPath, len(record.Exports))
			}
		}
		
		// Compile the module
		if ml.compilerFactory != nil {
			debugPrintf("// [ModuleLoader] Starting compilation for module: %s\n", record.ResolvedPath)
			record.State = ModuleCompiling
			record.CompileTime = time.Now()
			
			moduleCompiler := ml.compilerFactory()
			moduleCompiler.EnableModuleMode(record.ResolvedPath, ml)
			moduleCompiler.SetChecker(moduleChecker)
			
			chunk, compileErrors := moduleCompiler.Compile(record.AST)
			if len(compileErrors) > 0 {
				debugPrintf("// [ModuleLoader] Compilation error: %s\n", compileErrors[0].Error())
				record.Error = fmt.Errorf("compilation failed: %s", compileErrors[0].Error())
				record.State = ModuleError
				return record, nil
			}
			
			if chunk == nil {
				record.Error = fmt.Errorf("compilation returned nil chunk")
				record.State = ModuleError
				return record, nil
			}
			
			// Type assert to vm.Chunk
			vmChunk, ok := chunk.(*vm.Chunk)
			if !ok {
				record.Error = fmt.Errorf("compilation returned invalid chunk type")
				record.State = ModuleError
				return record, nil
			}
			
			record.CompiledChunk = vmChunk
		}
		record.State = ModuleCompiled
	} else {
		// No checker factory, just mark as parsed
		record.State = ModuleLoaded
	}
	
	record.CompleteTime = time.Now()
	debugPrintf("// [ModuleLoader] loadModuleSequential END: %s (state=%v)\n", specifier, record.State)
	return record, nil
}

// parseModuleSequential parses a single module synchronously
func (ml *moduleLoader) parseModuleSequential(record *ModuleRecord, resolved *ResolvedModule) error {
	debugPrintf("// [ModuleLoader] parseModuleSequential: %s\n", record.ResolvedPath)
	
	// Check if this is a native module - need to import the driver package functions
	// This is a temporary approach until we can restructure the imports
	if nativeModule := ml.checkForNativeModule(resolved.Source); nativeModule != nil {
		debugPrintf("// [ModuleLoader] Detected native module: %s\n", record.ResolvedPath)
		return ml.handleNativeModuleSource(record, nativeModule)
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
			return fmt.Errorf("failed to read source: %w", err)
		}
	}
	
	// Create source file using the real source package
	sourceFile := &source.SourceFile{
		Name:    resolved.ResolvedPath,
		Path:    resolved.ResolvedPath,
		Content: string(content),
	}
	
	// Parse using real lexer and parser
	lexerInstance := lexer.NewLexerWithSource(sourceFile)
	parserInstance := parser.NewParser(lexerInstance)
	
	program, parseErrs := parserInstance.ParseProgram()
	if len(parseErrs) > 0 {
		return fmt.Errorf("parsing failed: %s", parseErrs[0].Error())
	}
	
	// Store the parsed AST and source
	record.AST = program
	record.Source = sourceFile
	
	return nil
}

// loadModuleParallelImpl implements parallel module loading
func (ml *moduleLoader) loadModuleParallelImpl(specifier string, fromPath string) (*ModuleRecord, error) {
	debugPrintf("// [ModuleLoader] Starting parallel load for: %s from %s\n", specifier, fromPath)
	
	// Initialize parse queue and start discovery
	ml.parseQueue = NewParseQueue(ml.config.JobBufferSize)
	
	// Create context for the loading operation
	ctx, cancel := context.WithTimeout(context.Background(), ml.config.ResolveTimeout)
	defer cancel()
	
	debugPrintf("// [ModuleLoader] Starting worker pool with %d workers\n", ml.config.NumWorkers)
	
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
	debugPrintf("// [ModuleLoader] Creating parse job for entry point\n")
	entryJob, err := ml.createParseJob(specifier, fromPath, 0)
	if err != nil {
		debugPrintf("// [ModuleLoader] Failed to create parse job: %v\n", err)
		return nil, err
	}
	debugPrintf("// [ModuleLoader] Created parse job for: %s\n", entryJob.ModulePath)
	
	// Mark the entry point as discovered
	ml.depAnalyzer.MarkDiscovered(entryJob.ModulePath)
	
	err = ml.parseQueue.Enqueue(entryJob)
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue entry point: %w", err)
	}
	
	// Mark as in-flight before submitting to worker pool
	ml.parseQueue.MarkInFlight(entryJob.ModulePath)
	
	// Submit initial job to worker pool
	debugPrintf("// [ModuleLoader] Submitting entry job to worker pool\n")
	err = ml.workerPool.Submit(entryJob)
	if err != nil {
		debugPrintf("// [ModuleLoader] Failed to submit job: %v\n", err)
		return nil, fmt.Errorf("failed to submit initial job: %w", err)
	}
	debugPrintf("// [ModuleLoader] Job submitted successfully\n")
	
	// Main processing loop
	debugPrintf("// [ModuleLoader] Entering main processing loop\n")
	for !ml.parseQueue.IsEmpty() || ml.workerPool.HasActiveJobs() {
		debugPrintf("// [ModuleLoader] Loop: queue empty=%v, active jobs=%v\n", ml.parseQueue.IsEmpty(), ml.workerPool.HasActiveJobs())
		select {
		case result := <-ml.workerPool.Results():
			debugPrintf("// [ModuleLoader] Received result for: %s\n", result.ModulePath)
			err := ml.processParseResult(result)
			if err != nil {
				debugPrintf("// [ModuleLoader] Error processing result: %v\n", err)
				return nil, err
			}
			debugPrintf("// [ModuleLoader] Processed result successfully\n")
			
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
	
	// After parallel parsing is complete, perform dependency-ordered type checking
	entryModule, err := ml.performDependencyOrderedTypeChecking(specifier)
	if err != nil {
		return nil, fmt.Errorf("type checking failed: %w", err)
	}
	
	return entryModule, nil
}

// resolveModule resolves a module specifier using the resolver chain
func (ml *moduleLoader) resolveModule(specifier string, fromPath string) (*ResolvedModule, error) {
	debugPrintf("// [ModuleLoader] resolveModule: %s from %s\n", specifier, fromPath)
	for _, resolver := range ml.resolvers {
		if resolver.CanResolve(specifier) {
			debugPrintf("// [ModuleLoader] Trying resolver: %T\n", resolver)
			resolved, err := resolver.Resolve(specifier, fromPath)
			if err == nil {
				debugPrintf("// [ModuleLoader] Resolved to: %s\n", resolved.ResolvedPath)
				return resolved, nil
			}
			debugPrintf("// [ModuleLoader] Resolver failed: %v\n", err)
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

// SetCheckerFactory sets the factory function for creating type checkers
func (ml *moduleLoader) SetCheckerFactory(factory func() TypeChecker) {
	ml.mutex.Lock()
	defer ml.mutex.Unlock()
	
	ml.checkerFactory = factory
}

// SetCompilerFactory sets the factory function for creating compilers
func (ml *moduleLoader) SetCompilerFactory(factory func() Compiler) {
	ml.mutex.Lock()
	defer ml.mutex.Unlock()
	
	ml.compilerFactory = factory
}

// SetVMInstance sets the VM instance for native module initialization
func (ml *moduleLoader) SetVMInstance(vm *vm.VM) {
	ml.mutex.Lock()
	defer ml.mutex.Unlock()
	
	ml.vmInstance = vm
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


// performDependencyOrderedTypeChecking performs type checking in dependency order
// after all modules have been parsed in parallel
func (ml *moduleLoader) performDependencyOrderedTypeChecking(entryPoint string) (*ModuleRecord, error) {
	// Get the topologically sorted list of modules for type checking
	checkingOrder, err := ml.depAnalyzer.GetTopologicalOrder()
	if err != nil {
		return nil, fmt.Errorf("failed to determine type checking order: %w", err)
	}
	
	// Perform type checking in dependency order
	for _, modulePath := range checkingOrder {
		record := ml.registry.Get(modulePath)
		if record == nil {
			continue // Skip modules that weren't loaded
		}
		
		if record.Error != nil {
			continue // Skip modules that failed to parse
		}
		
		// Skip if no checker factory is set
		if ml.checkerFactory == nil {
			debugPrintf("// [ModuleLoader] No checker factory set, skipping type checking for %s\n", modulePath)
			record.State = ModuleLoaded
			record.CompleteTime = time.Now()
			continue
		}
		
		// Create a new checker for this module
		moduleChecker := ml.checkerFactory()
		
		// Enable module mode with this loader
		moduleChecker.EnableModuleMode(modulePath, ml)
		
		// Perform type checking on this module
		errors := moduleChecker.Check(record.AST)
		if len(errors) > 0 {
			// Store the first error (can be enhanced to store all errors)
			record.Error = fmt.Errorf("type checking failed: %s", errors[0].Error())
			record.State = ModuleError
			continue
		}
		
		// Extract exported types from the checked module
		if moduleChecker.IsModuleMode() {
			// Skip type extraction for native modules - they already have their exports set
			if !record.isNative {
				record.Exports = moduleChecker.GetModuleExports()
			}
		}
		
		// Phase 5: Compile the module to bytecode
		if ml.compilerFactory != nil {
			record.State = ModuleCompiling
			record.CompileTime = time.Now()
			
			// Create a compiler for this module
			moduleCompiler := ml.compilerFactory()
			moduleCompiler.EnableModuleMode(modulePath, ml)
			moduleCompiler.SetChecker(moduleChecker)
			
			// Compile the module to bytecode
			chunk, compileErrors := moduleCompiler.Compile(record.AST)
			if len(compileErrors) > 0 {
				record.Error = fmt.Errorf("compilation failed: %s", compileErrors[0].Error())
				record.State = ModuleError
				continue
			}
			
			if chunk == nil {
				record.Error = fmt.Errorf("compilation returned nil chunk")
				record.State = ModuleError
				continue
			}
			
			// Type assert to vm.Chunk
			vmChunk, ok := chunk.(*vm.Chunk)
			if !ok {
				record.Error = fmt.Errorf("compilation returned invalid chunk type")
				record.State = ModuleError
				continue
			}
			
			// Store the compiled chunk
			record.CompiledChunk = vmChunk
			debugPrintf("// [ModuleLoader] Module '%s' compiled successfully\n", modulePath)
		}
		
		record.State = ModuleCompiled
		record.CompleteTime = time.Now()
	}
	
	// Return the entry point module
	return ml.registry.Get(entryPoint), nil
}


// NativeModuleSource interface to detect native modules without circular imports
type NativeModuleSource interface {
	io.ReadCloser
	IsNativeModule() bool
	GetNativeModule() interface{}
}

// NativeModuleInterface provides access to native module functionality without importing driver
type NativeModuleInterface interface {
	GetName() string
	InitializeExports(vmInstance *vm.VM) map[string]vm.Value
	GetTypeExports() map[string]types.Type // Get type information for exports
}

// checkForNativeModule checks if the source is a native module
func (ml *moduleLoader) checkForNativeModule(source io.ReadCloser) NativeModuleInterface {
	fmt.Printf("// [ModuleLoader] checkForNativeModule: Checking source type: %T\n", source)
	if nativeSource, ok := source.(NativeModuleSource); ok {
		fmt.Printf("// [ModuleLoader] checkForNativeModule: Source is NativeModuleSource, IsNativeModule=%v\n", nativeSource.IsNativeModule())
		if nativeSource.IsNativeModule() {
			nativeModuleInterface := nativeSource.GetNativeModule()
			fmt.Printf("// [ModuleLoader] checkForNativeModule: Found native module: %T\n", nativeModuleInterface)
			if nativeModule, ok := nativeModuleInterface.(NativeModuleInterface); ok {
				return nativeModule
			} else {
				fmt.Printf("// [ModuleLoader] checkForNativeModule: Native module doesn't implement NativeModuleInterface\n")
			}
		}
	} else {
		fmt.Printf("// [ModuleLoader] checkForNativeModule: Source is NOT NativeModuleSource\n")
	}
	return nil
}

// handleNativeModuleSource processes a native module and populates the module record
func (ml *moduleLoader) handleNativeModuleSource(record *ModuleRecord, nativeModule NativeModuleInterface) error {
	debugPrintf("// [ModuleLoader] Processing native module: %s\n", nativeModule.GetName())
	
	// Use the VM instance if available, otherwise create a temporary one
	vmInstance := ml.vmInstance
	if vmInstance == nil {
		debugPrintf("// [ModuleLoader] Warning: No VM instance set, creating temporary VM for native module\n")
		vmInstance = vm.NewVM()
	}
	
	// Initialize the native module to get runtime values and type information
	runtimeValues := nativeModule.InitializeExports(vmInstance)
	debugPrintf("// [ModuleLoader] Native module exported %d runtime values\n", len(runtimeValues))
	
	// Get type information from the native module
	typeExports := nativeModule.GetTypeExports()
	debugPrintf("// [ModuleLoader] Native module has %d type exports\n", len(typeExports))
	
	// Directly populate both type and runtime exports
	record.Exports = typeExports        // Type information for type checker
	record.ExportValues = runtimeValues  // Runtime values for VM
	record.nativeModule = nativeModule   // Store native module reference
	record.isNative = true               // Mark as native module
	
	record.State = ModuleCompiled // Mark as compiled since no compilation needed
	
	// Create a simple AST to satisfy the module system requirements
	// This is just a placeholder since we don't need real parsing for native modules
	record.AST = &parser.Program{
		Statements: []parser.Statement{}, // Empty program is fine
	}
	
	// Create a synthetic source file for debugging purposes
	record.Source = &source.SourceFile{
		Name:    record.ResolvedPath,
		Path:    record.ResolvedPath,
		Content: fmt.Sprintf("// Native module: %s", nativeModule.GetName()),
	}
	
	debugPrintf("// [ModuleLoader] Native module processing complete\n")
	return nil
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
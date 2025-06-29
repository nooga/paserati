package modules

import (
	"sync"
	"time"
)

// registry implements the ModuleRegistry interface
type registry struct {
	modules   map[string]*ModuleRecord // Map of specifier -> module record
	mutex     sync.RWMutex             // Protects concurrent access
	stats     RegistryStats            // Performance statistics
	config    *LoaderConfig            // Configuration
}

// NewRegistry creates a new module registry
func NewRegistry(config *LoaderConfig) ModuleRegistry {
	if config == nil {
		config = DefaultLoaderConfig()
	}
	
	return &registry{
		modules: make(map[string]*ModuleRecord),
		config:  config,
		stats:   RegistryStats{},
	}
}

// Get retrieves a module record by specifier
func (r *registry) Get(specifier string) *ModuleRecord {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	record := r.modules[specifier]
	if record != nil {
		r.stats.CacheHits++
		
		// Check TTL if configured
		if r.config.CacheTTL > 0 {
			if time.Since(record.LoadTime) > r.config.CacheTTL {
				// Module has expired, treat as cache miss
				r.stats.CacheMisses++
				return nil
			}
		}
	} else {
		r.stats.CacheMisses++
	}
	
	return record
}

// Set stores a module record
func (r *registry) Set(specifier string, record *ModuleRecord) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	// Check cache size limits
	if r.config.CacheSize > 0 && len(r.modules) >= r.config.CacheSize {
		// Remove oldest module if at capacity
		r.evictOldest()
	}
	
	// Update statistics
	if r.modules[specifier] == nil {
		r.stats.TotalModules++
	}
	
	if record.State == ModuleCompiled {
		r.stats.LoadedModules++
	} else if record.State == ModuleError {
		r.stats.FailedModules++
	}
	
	r.modules[specifier] = record
}

// SetParsed updates a module record with parse results
func (r *registry) SetParsed(specifier string, result *ParseResult) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	record := r.modules[specifier]
	if record == nil {
		// Create new record if it doesn't exist
		record = &ModuleRecord{
			Specifier:    specifier,
			ResolvedPath: result.ModulePath,
			State:        ModuleLoading,
			LoadTime:     time.Now(),
		}
		r.modules[specifier] = record
		r.stats.TotalModules++
	}
	
	// Update with parse results
	record.AST = result.AST
	record.ParseDuration = result.ParseDuration
	record.WorkerID = result.WorkerID
	record.Error = result.Error
	
	if result.Error == nil {
		record.State = ModuleParsed
		
		// Extract dependencies from import specs
		record.Dependencies = make([]string, len(result.ImportSpecs))
		for i, importSpec := range result.ImportSpecs {
			record.Dependencies[i] = importSpec.ModulePath
		}
	} else {
		record.State = ModuleError
		r.stats.FailedModules++
	}
}

// Remove removes a module from the cache
func (r *registry) Remove(specifier string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	if record := r.modules[specifier]; record != nil {
		delete(r.modules, specifier)
		r.stats.TotalModules--
		
		if record.State == ModuleCompiled {
			r.stats.LoadedModules--
		} else if record.State == ModuleError {
			r.stats.FailedModules--
		}
	}
}

// Clear clears all cached modules
func (r *registry) Clear() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.modules = make(map[string]*ModuleRecord)
	r.stats = RegistryStats{}
}

// List returns all cached module specifiers
func (r *registry) List() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	specifiers := make([]string, 0, len(r.modules))
	for specifier := range r.modules {
		specifiers = append(specifiers, specifier)
	}
	
	return specifiers
}

// Size returns the number of cached modules
func (r *registry) Size() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	return len(r.modules)
}

// GetStats returns current registry statistics
func (r *registry) GetStats() RegistryStats {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	// Calculate approximate memory usage
	memoryUsage := int64(len(r.modules) * 1000) // Rough estimate per module
	for _, record := range r.modules {
		if record.Source != nil {
			memoryUsage += int64(len(record.Source.Content))
		}
	}
	
	stats := r.stats
	stats.MemoryUsage = memoryUsage
	return stats
}

// UpdateState updates the state of a module
func (r *registry) UpdateState(specifier string, state ModuleState) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	if record := r.modules[specifier]; record != nil {
		record.State = state
		
		// Update completion time if module is fully processed
		if state == ModuleCompiled || state == ModuleError {
			record.CompleteTime = time.Now()
		}
	}
}

// GetDependents returns all modules that depend on the given module
func (r *registry) GetDependents(specifier string) []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	var dependents []string
	for _, record := range r.modules {
		for _, dep := range record.Dependencies {
			if dep == specifier {
				dependents = append(dependents, record.Specifier)
				break
			}
		}
	}
	
	return dependents
}

// evictOldest removes the oldest module from the cache (called with lock held)
func (r *registry) evictOldest() {
	var oldestSpecifier string
	var oldestTime time.Time
	
	first := true
	for specifier, record := range r.modules {
		if first || record.LoadTime.Before(oldestTime) {
			oldestSpecifier = specifier
			oldestTime = record.LoadTime
			first = false
		}
	}
	
	if oldestSpecifier != "" {
		delete(r.modules, oldestSpecifier)
		r.stats.TotalModules--
	}
}

// IsStale returns true if a module should be reloaded due to TTL expiry
func (r *registry) IsStale(specifier string) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	record := r.modules[specifier]
	if record == nil {
		return true // Not cached, needs loading
	}
	
	if r.config.CacheTTL > 0 {
		return time.Since(record.LoadTime) > r.config.CacheTTL
	}
	
	return false // No TTL configured, never stale
}

// GetByState returns all modules in a specific state
func (r *registry) GetByState(state ModuleState) []*ModuleRecord {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	var modules []*ModuleRecord
	for _, record := range r.modules {
		if record.State == state {
			modules = append(modules, record)
		}
	}
	
	return modules
}
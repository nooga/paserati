package modules

import (
	"fmt"
	"sync"
)

// dependencyAnalyzer implements DependencyAnalyzer interface
type dependencyAnalyzer struct {
	discovered   map[string]bool            // Already discovered modules
	parsing      map[string]bool            // Currently being parsed
	parsed       map[string]*ParseResult    // Completed parses
	depGraph     map[string][]string        // Module → dependencies
	depCounts    map[string]int             // Module → import count
	depDepths    map[string]int             // Module → dependency depth
	mutex        sync.RWMutex               // Thread safety
}

// NewDependencyAnalyzer creates a new dependency analyzer
func NewDependencyAnalyzer() DependencyAnalyzer {
	return &dependencyAnalyzer{
		discovered: make(map[string]bool),
		parsing:    make(map[string]bool),
		parsed:     make(map[string]*ParseResult),
		depGraph:   make(map[string][]string),
		depCounts:  make(map[string]int),
		depDepths:  make(map[string]int),
	}
}

// MarkDiscovered marks a module as discovered
func (da *dependencyAnalyzer) MarkDiscovered(modulePath string) {
	da.mutex.Lock()
	defer da.mutex.Unlock()
	
	da.discovered[modulePath] = true
}

// IsDiscovered returns true if a module has been discovered
func (da *dependencyAnalyzer) IsDiscovered(modulePath string) bool {
	da.mutex.RLock()
	defer da.mutex.RUnlock()
	
	return da.discovered[modulePath]
}

// GetDependencyDepth returns how deep a module is in the dependency tree
func (da *dependencyAnalyzer) GetDependencyDepth(modulePath string) int {
	da.mutex.RLock()
	if depth, exists := da.depDepths[modulePath]; exists {
		da.mutex.RUnlock()
		return depth
	}
	da.mutex.RUnlock()
	
	// If not calculated, compute it
	da.mutex.Lock()
	defer da.mutex.Unlock()
	
	// Double-check after acquiring write lock
	if depth, exists := da.depDepths[modulePath]; exists {
		return depth
	}
	
	depth := da.calculateDepth(modulePath, make(map[string]bool))
	da.depDepths[modulePath] = depth
	return depth
}

// calculateDepth recursively calculates dependency depth
func (da *dependencyAnalyzer) calculateDepth(modulePath string, visited map[string]bool) int {
	if visited[modulePath] {
		// Circular dependency - return high depth to deprioritize
		return 1000
	}
	
	visited[modulePath] = true
	defer func() { visited[modulePath] = false }()
	
	deps := da.depGraph[modulePath]
	if len(deps) == 0 {
		return 0 // Leaf node
	}
	
	maxDepth := 0
	for _, dep := range deps {
		depDepth := da.calculateDepth(dep, visited)
		if depDepth > maxDepth {
			maxDepth = depDepth
		}
	}
	
	return maxDepth + 1
}

// GetImportCount returns how many times a module is imported
func (da *dependencyAnalyzer) GetImportCount(modulePath string) int {
	da.mutex.RLock()
	defer da.mutex.RUnlock()
	
	return da.depCounts[modulePath]
}

// AddDependency adds a dependency relationship
func (da *dependencyAnalyzer) AddDependency(from, to string) {
	da.mutex.Lock()
	defer da.mutex.Unlock()
	
	// Add to dependency graph
	da.depGraph[from] = append(da.depGraph[from], to)
	
	// Increment import count for the dependency
	da.depCounts[to]++
	
	// Clear cached depths since graph changed
	da.depDepths = make(map[string]int)
}

// GetDependencies returns all dependencies of a module
func (da *dependencyAnalyzer) GetDependencies(modulePath string) []string {
	da.mutex.RLock()
	defer da.mutex.RUnlock()
	
	deps := da.depGraph[modulePath]
	result := make([]string, len(deps))
	copy(result, deps)
	return result
}

// Clear resets the analyzer state
func (da *dependencyAnalyzer) Clear() {
	da.mutex.Lock()
	defer da.mutex.Unlock()
	
	da.discovered = make(map[string]bool)
	da.parsing = make(map[string]bool)
	da.parsed = make(map[string]*ParseResult)
	da.depGraph = make(map[string][]string)
	da.depCounts = make(map[string]int)
	da.depDepths = make(map[string]int)
}

// MarkParsing marks a module as currently being parsed
func (da *dependencyAnalyzer) MarkParsing(modulePath string) {
	da.mutex.Lock()
	defer da.mutex.Unlock()
	
	da.parsing[modulePath] = true
}

// MarkParsed marks a module as parsed and stores the result
func (da *dependencyAnalyzer) MarkParsed(modulePath string, result *ParseResult) {
	da.mutex.Lock()
	defer da.mutex.Unlock()
	
	da.parsing[modulePath] = false
	da.parsed[modulePath] = result
	
	// Update dependency graph based on parse results
	if result.Error == nil {
		for _, importSpec := range result.ImportSpecs {
			da.depGraph[modulePath] = append(da.depGraph[modulePath], importSpec.ModulePath)
			da.depCounts[importSpec.ModulePath]++
		}
		
		// Clear cached depths since graph changed
		da.depDepths = make(map[string]int)
	}
}

// IsParsing returns true if a module is currently being parsed
func (da *dependencyAnalyzer) IsParsing(modulePath string) bool {
	da.mutex.RLock()
	defer da.mutex.RUnlock()
	
	return da.parsing[modulePath]
}

// GetParseResult returns the parse result for a module
func (da *dependencyAnalyzer) GetParseResult(modulePath string) *ParseResult {
	da.mutex.RLock()
	defer da.mutex.RUnlock()
	
	return da.parsed[modulePath]
}

// GetAllDiscovered returns all discovered module paths
func (da *dependencyAnalyzer) GetAllDiscovered() []string {
	da.mutex.RLock()
	defer da.mutex.RUnlock()
	
	var result []string
	for modulePath := range da.discovered {
		result = append(result, modulePath)
	}
	return result
}

// GetStats returns dependency analysis statistics
func (da *dependencyAnalyzer) GetStats() DependencyStats {
	da.mutex.RLock()
	defer da.mutex.RUnlock()
	
	totalDiscovered := len(da.discovered)
	totalParsing := 0
	totalParsed := len(da.parsed)
	totalDependencies := 0
	
	for _, parsing := range da.parsing {
		if parsing {
			totalParsing++
		}
	}
	
	for _, deps := range da.depGraph {
		totalDependencies += len(deps)
	}
	
	return DependencyStats{
		TotalDiscovered:   totalDiscovered,
		TotalParsing:      totalParsing,
		TotalParsed:       totalParsed,
		TotalDependencies: totalDependencies,
		MaxDepth:          da.getMaxDepth(),
		CircularDeps:      da.detectCircularDependencies(),
	}
}

// DependencyStats contains statistics about dependency analysis
type DependencyStats struct {
	TotalDiscovered   int      // Total modules discovered
	TotalParsing      int      // Modules currently being parsed
	TotalParsed       int      // Modules successfully parsed
	TotalDependencies int      // Total dependency relationships
	MaxDepth          int      // Maximum dependency depth
	CircularDeps      []string // Modules involved in circular dependencies
}

// getMaxDepth returns the maximum dependency depth
func (da *dependencyAnalyzer) getMaxDepth() int {
	maxDepth := 0
	for modulePath := range da.discovered {
		depth := da.calculateDepth(modulePath, make(map[string]bool))
		if depth > maxDepth && depth < 1000 { // Exclude circular deps
			maxDepth = depth
		}
	}
	return maxDepth
}

// detectCircularDependencies detects modules involved in circular dependencies
func (da *dependencyAnalyzer) detectCircularDependencies() []string {
	var circularModules []string
	visited := make(map[string]bool)
	
	for modulePath := range da.discovered {
		if da.hasCircularDependency(modulePath, make(map[string]bool), visited) {
			if !visited[modulePath] {
				circularModules = append(circularModules, modulePath)
				visited[modulePath] = true
			}
		}
	}
	
	return circularModules
}

// hasCircularDependency checks if a module has circular dependencies
func (da *dependencyAnalyzer) hasCircularDependency(modulePath string, path map[string]bool, globalVisited map[string]bool) bool {
	if globalVisited[modulePath] {
		return false // Already checked
	}
	
	if path[modulePath] {
		return true // Found cycle
	}
	
	path[modulePath] = true
	defer func() { path[modulePath] = false }()
	
	deps := da.depGraph[modulePath]
	for _, dep := range deps {
		if da.hasCircularDependency(dep, path, globalVisited) {
			return true
		}
	}
	
	globalVisited[modulePath] = true
	return false
}

// GetTopologicalOrder returns modules in dependency order for type checking
// Dependencies are processed before their dependents
func (da *dependencyAnalyzer) GetTopologicalOrder() ([]string, error) {
	da.mutex.RLock()
	defer da.mutex.RUnlock()
	
	// Build a copy of the dependency graph for processing
	graph := make(map[string][]string)
	inDegree := make(map[string]int)
	
	// Initialize all discovered modules
	for modulePath := range da.discovered {
		graph[modulePath] = make([]string, len(da.depGraph[modulePath]))
		copy(graph[modulePath], da.depGraph[modulePath])
		inDegree[modulePath] = 0
	}
	
	// Calculate in-degrees
	for _, deps := range graph {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}
	
	// Kahn's algorithm for topological sorting
	var queue []string
	var result []string
	
	// Find all modules with no dependencies (in-degree 0)
	for modulePath, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, modulePath)
		}
	}
	
	// Process modules in dependency order
	for len(queue) > 0 {
		// Remove first module from queue
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)
		
		// For each module that depends on current module
		for _, dependent := range graph[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}
	
	// Check if all modules were processed (no circular dependencies)
	if len(result) != len(da.discovered) {
		// There are circular dependencies
		var remaining []string
		for modulePath := range da.discovered {
			found := false
			for _, processed := range result {
				if processed == modulePath {
					found = true
					break
				}
			}
			if !found {
				remaining = append(remaining, modulePath)
			}
		}
		
		return nil, fmt.Errorf("circular dependency detected among modules: %v", remaining)
	}
	
	return result, nil
}
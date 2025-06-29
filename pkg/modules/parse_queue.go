package modules

import (
	"container/heap"
	"fmt"
	"sync"
	"time"
)

// parseQueue implements a priority queue for module parsing jobs
type parseQueue struct {
	heap       *jobHeap
	inFlight   map[string]bool        // Currently processing
	completed  map[string]*ParseResult // Completed results
	mutex      sync.RWMutex           // Thread safety
	maxSize    int                    // Maximum queue size (0 = unlimited)
}

// jobHeap implements heap.Interface for priority queue
type jobHeap []*ParseJob

func (h jobHeap) Len() int { return len(h) }

func (h jobHeap) Less(i, j int) bool {
	// Lower priority number = higher priority
	if h[i].Priority != h[j].Priority {
		return h[i].Priority < h[j].Priority
	}
	
	// If same priority, older jobs first
	return h[i].Timestamp.Before(h[j].Timestamp)
}

func (h jobHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *jobHeap) Push(x interface{}) {
	*h = append(*h, x.(*ParseJob))
}

func (h *jobHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

// NewParseQueue creates a new priority-based parse queue
func NewParseQueue(maxSize int) *parseQueue {
	pq := &parseQueue{
		heap:      &jobHeap{},
		inFlight:  make(map[string]bool),
		completed: make(map[string]*ParseResult),
		maxSize:   maxSize,
	}
	
	heap.Init(pq.heap)
	return pq
}

// Enqueue adds a job to the priority queue
func (pq *parseQueue) Enqueue(job *ParseJob) error {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	// Check if already in flight or completed
	if pq.inFlight[job.ModulePath] {
		return nil // Already being processed
	}
	
	if pq.completed[job.ModulePath] != nil {
		return nil // Already completed
	}
	
	// Check size limit
	if pq.maxSize > 0 && pq.heap.Len() >= pq.maxSize {
		return &QueueFullError{MaxSize: pq.maxSize}
	}
	
	// Set timestamp if not set
	if job.Timestamp.IsZero() {
		job.Timestamp = time.Now()
	}
	
	// Add to priority queue
	heap.Push(pq.heap, job)
	
	return nil
}

// Dequeue removes and returns the highest priority job
func (pq *parseQueue) Dequeue() *ParseJob {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	if pq.heap.Len() == 0 {
		return nil
	}
	
	job := heap.Pop(pq.heap).(*ParseJob)
	
	// Mark as in flight
	pq.inFlight[job.ModulePath] = true
	
	return job
}

// MarkCompleted marks a job as completed and stores the result
func (pq *parseQueue) MarkCompleted(modulePath string, result *ParseResult) {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	// Remove from in-flight
	delete(pq.inFlight, modulePath)
	
	// Store result
	pq.completed[modulePath] = result
}

// IsEmpty returns true if the queue is empty and no jobs are in flight
func (pq *parseQueue) IsEmpty() bool {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return pq.heap.Len() == 0 && len(pq.inFlight) == 0
}

// MarkInFlight marks a module as being processed
func (pq *parseQueue) MarkInFlight(modulePath string) {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	pq.inFlight[modulePath] = true
}

// Size returns the number of jobs in the queue
func (pq *parseQueue) Size() int {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return pq.heap.Len()
}

// InFlightCount returns the number of jobs currently being processed
func (pq *parseQueue) InFlightCount() int {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return len(pq.inFlight)
}

// CompletedCount returns the number of completed jobs
func (pq *parseQueue) CompletedCount() int {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return len(pq.completed)
}

// GetCompleted returns the parse result for a completed module
func (pq *parseQueue) GetCompleted(modulePath string) *ParseResult {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return pq.completed[modulePath]
}

// IsInFlight returns true if a module is currently being processed
func (pq *parseQueue) IsInFlight(modulePath string) bool {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return pq.inFlight[modulePath]
}

// IsCompleted returns true if a module has been completed
func (pq *parseQueue) IsCompleted(modulePath string) bool {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return pq.completed[modulePath] != nil
}

// GetStats returns queue statistics
func (pq *parseQueue) GetStats() ParseQueueStats {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	stats := ParseQueueStats{
		QueueSize:      pq.heap.Len(),
		InFlightCount:  len(pq.inFlight),
		CompletedCount: len(pq.completed),
		MaxSize:        pq.maxSize,
	}
	
	// Calculate priority distribution
	stats.PriorityDistribution = make(map[int]int)
	for _, job := range *pq.heap {
		stats.PriorityDistribution[job.Priority]++
	}
	
	// Calculate average wait time for completed jobs
	if len(pq.completed) > 0 {
		totalWaitTime := time.Duration(0)
		count := 0
		
		for _, result := range pq.completed {
			if !result.Timestamp.IsZero() {
				// This would need the original job timestamp to calculate properly
				// For now, just use parse duration as approximation
				totalWaitTime += result.ParseDuration
				count++
			}
		}
		
		if count > 0 {
			stats.AverageWaitTime = totalWaitTime / time.Duration(count)
		}
	}
	
	return stats
}

// Clear removes all jobs and results
func (pq *parseQueue) Clear() {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	// Clear the heap
	*pq.heap = (*pq.heap)[:0]
	heap.Init(pq.heap)
	
	// Clear maps
	pq.inFlight = make(map[string]bool)
	pq.completed = make(map[string]*ParseResult)
}

// Peek returns the highest priority job without removing it
func (pq *parseQueue) Peek() *ParseJob {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	if pq.heap.Len() == 0 {
		return nil
	}
	
	return (*pq.heap)[0]
}

// GetAllInFlight returns all modules currently being processed
func (pq *parseQueue) GetAllInFlight() []string {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	var result []string
	for modulePath := range pq.inFlight {
		result = append(result, modulePath)
	}
	return result
}

// GetAllCompleted returns all completed module paths
func (pq *parseQueue) GetAllCompleted() []string {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	var result []string
	for modulePath := range pq.completed {
		result = append(result, modulePath)
	}
	return result
}

// UpdatePriority updates the priority of a job in the queue
func (pq *parseQueue) UpdatePriority(modulePath string, newPriority int) bool {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	// Find the job in the heap
	for i, job := range *pq.heap {
		if job.ModulePath == modulePath {
			job.Priority = newPriority
			heap.Fix(pq.heap, i)
			return true
		}
	}
	
	return false // Job not found
}

// ParseQueueStats contains statistics about the parse queue
type ParseQueueStats struct {
	QueueSize             int                    // Jobs waiting in queue
	InFlightCount         int                    // Jobs currently being processed
	CompletedCount        int                    // Jobs completed
	MaxSize               int                    // Maximum queue size
	PriorityDistribution  map[int]int            // Count of jobs by priority
	AverageWaitTime       time.Duration          // Average time jobs wait in queue
}

// QueueFullError is returned when trying to enqueue to a full queue
type QueueFullError struct {
	MaxSize int
}

func (e *QueueFullError) Error() string {
	return fmt.Sprintf("parse queue is full (max size: %d)", e.MaxSize)
}

// PriorityCalculator provides methods for calculating job priorities
type PriorityCalculator struct {
	entryPoints    map[string]bool // Entry point modules get highest priority
	dependencyDepth map[string]int // Dependency depth for each module
	importCounts   map[string]int  // How many times each module is imported
}

// NewPriorityCalculator creates a new priority calculator
func NewPriorityCalculator(entryPoints []string) *PriorityCalculator {
	entryMap := make(map[string]bool)
	for _, entry := range entryPoints {
		entryMap[entry] = true
	}
	
	return &PriorityCalculator{
		entryPoints:     entryMap,
		dependencyDepth: make(map[string]int),
		importCounts:    make(map[string]int),
	}
}

// CalculatePriority calculates the priority for a module
func (pc *PriorityCalculator) CalculatePriority(modulePath string, fromPath string) int {
	// Priority rules (lower number = higher priority):
	// 0 = Entry points (highest)
	// 1-10 = Direct dependencies of entry points, boosted by import frequency
	// 11-100 = Lower level dependencies
	// 100+ = Deep dependencies
	
	if pc.entryPoints[modulePath] {
		return 0 // Highest priority for entry points
	}
	
	depth := pc.dependencyDepth[modulePath]
	importCount := pc.importCounts[modulePath]
	
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

// UpdateDependencyInfo updates dependency information for priority calculation
func (pc *PriorityCalculator) UpdateDependencyInfo(modulePath string, depth int, importCount int) {
	pc.dependencyDepth[modulePath] = depth
	pc.importCounts[modulePath] = importCount
}


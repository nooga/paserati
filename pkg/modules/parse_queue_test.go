package modules

import (
	"fmt"
	"testing"
	"time"
)

func TestParseQueueBasic(t *testing.T) {
	queue := NewParseQueue(0) // Unlimited size
	
	if !queue.IsEmpty() {
		t.Error("Expected queue to be empty initially")
	}
	
	if queue.Size() != 0 {
		t.Errorf("Expected size 0, got %d", queue.Size())
	}
	
	// Test enqueue
	job := &ParseJob{
		ModulePath: "test.ts",
		Priority:   1,
		Timestamp:  time.Now(),
	}
	
	err := queue.Enqueue(job)
	if err != nil {
		t.Errorf("Expected successful enqueue, got error: %v", err)
	}
	
	if queue.IsEmpty() {
		t.Error("Expected queue not to be empty after enqueue")
	}
	
	if queue.Size() != 1 {
		t.Errorf("Expected size 1, got %d", queue.Size())
	}
	
	// Test peek
	peeked := queue.Peek()
	if peeked == nil {
		t.Error("Expected to peek a job")
	} else if peeked.ModulePath != "test.ts" {
		t.Errorf("Expected peeked job path 'test.ts', got '%s'", peeked.ModulePath)
	}
	
	// Size should remain the same after peek
	if queue.Size() != 1 {
		t.Errorf("Expected size 1 after peek, got %d", queue.Size())
	}
	
	// Test dequeue
	dequeued := queue.Dequeue()
	if dequeued == nil {
		t.Error("Expected to dequeue a job")
	} else if dequeued.ModulePath != "test.ts" {
		t.Errorf("Expected dequeued job path 'test.ts', got '%s'", dequeued.ModulePath)
	}
	
	// Should be marked as in flight
	if !queue.IsInFlight("test.ts") {
		t.Error("Expected job to be marked as in flight after dequeue")
	}
	
	// Queue should be empty but not completely empty due to in-flight job
	if queue.Size() != 0 {
		t.Errorf("Expected size 0 after dequeue, got %d", queue.Size())
	}
}

func TestParseQueuePriority(t *testing.T) {
	queue := NewParseQueue(0)
	
	// Add jobs with different priorities
	jobs := []*ParseJob{
		{ModulePath: "low.ts", Priority: 10, Timestamp: time.Now()},
		{ModulePath: "high.ts", Priority: 1, Timestamp: time.Now()},
		{ModulePath: "medium.ts", Priority: 5, Timestamp: time.Now()},
	}
	
	// Enqueue in random order
	for _, job := range jobs {
		err := queue.Enqueue(job)
		if err != nil {
			t.Errorf("Expected successful enqueue, got error: %v", err)
		}
	}
	
	// Should dequeue in priority order (lower number = higher priority)
	expectedOrder := []string{"high.ts", "medium.ts", "low.ts"}
	
	for i, expected := range expectedOrder {
		job := queue.Dequeue()
		if job == nil {
			t.Errorf("Expected job at position %d", i)
			continue
		}
		
		if job.ModulePath != expected {
			t.Errorf("Expected job %s at position %d, got %s", expected, i, job.ModulePath)
		}
	}
}

func TestParseQueueTimestamp(t *testing.T) {
	queue := NewParseQueue(0)
	
	baseTime := time.Now()
	
	// Add jobs with same priority but different timestamps
	jobs := []*ParseJob{
		{ModulePath: "second.ts", Priority: 1, Timestamp: baseTime.Add(1 * time.Second)},
		{ModulePath: "first.ts", Priority: 1, Timestamp: baseTime},
		{ModulePath: "third.ts", Priority: 1, Timestamp: baseTime.Add(2 * time.Second)},
	}
	
	// Enqueue in random order
	for _, job := range jobs {
		err := queue.Enqueue(job)
		if err != nil {
			t.Errorf("Expected successful enqueue, got error: %v", err)
		}
	}
	
	// Should dequeue in timestamp order when priority is equal
	expectedOrder := []string{"first.ts", "second.ts", "third.ts"}
	
	for i, expected := range expectedOrder {
		job := queue.Dequeue()
		if job == nil {
			t.Errorf("Expected job at position %d", i)
			continue
		}
		
		if job.ModulePath != expected {
			t.Errorf("Expected job %s at position %d, got %s", expected, i, job.ModulePath)
		}
	}
}

func TestParseQueueMarkCompleted(t *testing.T) {
	queue := NewParseQueue(0)
	
	job := &ParseJob{
		ModulePath: "test.ts",
		Priority:   1,
		Timestamp:  time.Now(),
	}
	
	err := queue.Enqueue(job)
	if err != nil {
		t.Errorf("Expected successful enqueue, got error: %v", err)
	}
	
	dequeued := queue.Dequeue()
	if dequeued == nil {
		t.Error("Expected to dequeue a job")
		return
	}
	
	// Should be in flight
	if !queue.IsInFlight("test.ts") {
		t.Error("Expected job to be in flight")
	}
	
	if queue.IsCompleted("test.ts") {
		t.Error("Expected job not to be completed yet")
	}
	
	// Mark as completed
	result := &ParseResult{
		ModulePath: "test.ts",
		WorkerID:   1,
		Timestamp:  time.Now(),
	}
	
	queue.MarkCompleted("test.ts", result)
	
	// Should no longer be in flight
	if queue.IsInFlight("test.ts") {
		t.Error("Expected job not to be in flight after completion")
	}
	
	// Should be completed
	if !queue.IsCompleted("test.ts") {
		t.Error("Expected job to be completed")
	}
	
	// Should be able to get the result
	retrievedResult := queue.GetCompleted("test.ts")
	if retrievedResult == nil {
		t.Error("Expected to retrieve completed result")
	} else if retrievedResult.ModulePath != "test.ts" {
		t.Errorf("Expected result path 'test.ts', got '%s'", retrievedResult.ModulePath)
	}
	
	// Queue should now be completely empty
	if !queue.IsEmpty() {
		t.Error("Expected queue to be empty after completion")
	}
}

func TestParseQueueMaxSize(t *testing.T) {
	queue := NewParseQueue(2) // Limited size
	
	// Should be able to enqueue up to max size
	for i := 0; i < 2; i++ {
		job := &ParseJob{
			ModulePath: fmt.Sprintf("test%d.ts", i),
			Priority:   1,
			Timestamp:  time.Now(),
		}
		
		err := queue.Enqueue(job)
		if err != nil {
			t.Errorf("Expected successful enqueue for job %d, got error: %v", i, err)
		}
	}
	
	// Third job should fail
	job := &ParseJob{
		ModulePath: "overflow.ts",
		Priority:   1,
		Timestamp:  time.Now(),
	}
	
	err := queue.Enqueue(job)
	if err == nil {
		t.Error("Expected error when exceeding max size")
	}
	
	if _, ok := err.(*QueueFullError); !ok {
		t.Errorf("Expected QueueFullError, got %T", err)
	}
}

func TestParseQueueDuplicates(t *testing.T) {
	queue := NewParseQueue(0)
	
	job1 := &ParseJob{
		ModulePath: "test.ts",
		Priority:   1,
		Timestamp:  time.Now(),
	}
	
	job2 := &ParseJob{
		ModulePath: "test.ts", // Same path
		Priority:   2,
		Timestamp:  time.Now(),
	}
	
	// First job should succeed
	err := queue.Enqueue(job1)
	if err != nil {
		t.Errorf("Expected successful enqueue for first job, got error: %v", err)
	}
	
	// Dequeue to mark as in flight
	dequeued := queue.Dequeue()
	if dequeued == nil {
		t.Error("Expected to dequeue first job")
		return
	}
	
	// Second job with same path should be ignored (already in flight)
	err = queue.Enqueue(job2)
	if err != nil {
		t.Errorf("Expected no error for duplicate job (should be ignored), got: %v", err)
	}
	
	// Queue should still be empty (duplicate was ignored)
	if queue.Size() != 0 {
		t.Errorf("Expected queue size 0 (duplicate ignored), got %d", queue.Size())
	}
}

func TestParseQueueUpdatePriority(t *testing.T) {
	queue := NewParseQueue(0)
	
	jobs := []*ParseJob{
		{ModulePath: "test1.ts", Priority: 10, Timestamp: time.Now()},
		{ModulePath: "test2.ts", Priority: 5, Timestamp: time.Now()},
	}
	
	for _, job := range jobs {
		err := queue.Enqueue(job)
		if err != nil {
			t.Errorf("Expected successful enqueue, got error: %v", err)
		}
	}
	
	// Update priority of test1.ts to be higher priority
	success := queue.UpdatePriority("test1.ts", 1)
	if !success {
		t.Error("Expected successful priority update")
	}
	
	// test1.ts should now be dequeued first
	first := queue.Dequeue()
	if first == nil || first.ModulePath != "test1.ts" {
		t.Error("Expected test1.ts to be dequeued first after priority update")
	}
	
	second := queue.Dequeue()
	if second == nil || second.ModulePath != "test2.ts" {
		t.Error("Expected test2.ts to be dequeued second")
	}
}

func TestParseQueueStats(t *testing.T) {
	queue := NewParseQueue(10)
	
	// Add some jobs
	for i := 0; i < 3; i++ {
		job := &ParseJob{
			ModulePath: fmt.Sprintf("test%d.ts", i),
			Priority:   i,
			Timestamp:  time.Now(),
		}
		
		err := queue.Enqueue(job)
		if err != nil {
			t.Errorf("Expected successful enqueue for job %d, got error: %v", i, err)
		}
	}
	
	// Dequeue one to mark as in flight
	queue.Dequeue()
	
	// Complete one
	result := &ParseResult{
		ModulePath: "test0.ts",
		WorkerID:   1,
		Timestamp:  time.Now(),
	}
	queue.MarkCompleted("test0.ts", result)
	
	// Check stats
	stats := queue.GetStats()
	
	if stats.QueueSize != 2 {
		t.Errorf("Expected queue size 2, got %d", stats.QueueSize)
	}
	
	if stats.InFlightCount != 0 {
		t.Errorf("Expected in flight count 0, got %d", stats.InFlightCount)
	}
	
	if stats.CompletedCount != 1 {
		t.Errorf("Expected completed count 1, got %d", stats.CompletedCount)
	}
	
	if stats.MaxSize != 10 {
		t.Errorf("Expected max size 10, got %d", stats.MaxSize)
	}
	
	// Check priority distribution
	if len(stats.PriorityDistribution) == 0 {
		t.Error("Expected priority distribution to be populated")
	}
}

func TestParseQueueClear(t *testing.T) {
	queue := NewParseQueue(0)
	
	// Add some jobs
	for i := 0; i < 3; i++ {
		job := &ParseJob{
			ModulePath: fmt.Sprintf("test%d.ts", i),
			Priority:   1,
			Timestamp:  time.Now(),
		}
		
		err := queue.Enqueue(job)
		if err != nil {
			t.Errorf("Expected successful enqueue for job %d, got error: %v", i, err)
		}
	}
	
	// Dequeue and complete one
	dequeued := queue.Dequeue()
	if dequeued != nil {
		result := &ParseResult{
			ModulePath: dequeued.ModulePath,
			WorkerID:   1,
			Timestamp:  time.Now(),
		}
		queue.MarkCompleted(dequeued.ModulePath, result)
	}
	
	// Clear the queue
	queue.Clear()
	
	// Everything should be reset
	if !queue.IsEmpty() {
		t.Error("Expected queue to be empty after clear")
	}
	
	if queue.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", queue.Size())
	}
	
	if queue.InFlightCount() != 0 {
		t.Errorf("Expected in flight count 0 after clear, got %d", queue.InFlightCount())
	}
	
	if queue.CompletedCount() != 0 {
		t.Errorf("Expected completed count 0 after clear, got %d", queue.CompletedCount())
	}
}
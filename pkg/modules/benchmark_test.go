package modules

import (
	"testing"
	"time"

	"github.com/nooga/paserati/pkg/source"
)

func BenchmarkMemoryResolver(b *testing.B) {
	resolver := NewMemoryResolver("benchmark")

	// Add test modules
	for i := 0; i < 100; i++ {
		moduleName := "module" + string(rune('0'+i%10)) + ".ts"
		content := "export const value = " + string(rune('0'+i%10)) + ";"
		resolver.AddModule(moduleName, content)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		moduleName := "module" + string(rune('0'+i%10)) + ".ts"
		_, err := resolver.Resolve(moduleName, "")
		if err != nil {
			b.Errorf("Resolution failed: %v", err)
		}
	}
}

func BenchmarkModuleRegistry(b *testing.B) {
	config := DefaultLoaderConfig()
	registry := NewRegistry(config)

	// Pre-populate with test modules
	for i := 0; i < 100; i++ {
		specifier := "module" + string(rune('0'+i%10))
		record := &ModuleRecord{
			Specifier:    specifier,
			ResolvedPath: specifier + ".ts",
			State:        ModuleLoaded,
			LoadTime:     time.Now(),
			CompleteTime: time.Now(),
		}
		registry.Set(specifier, record)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		specifier := "module" + string(rune('0'+i%10))
		record := registry.Get(specifier)
		if record == nil {
			b.Errorf("Expected to find module %s", specifier)
		}
	}
}

func BenchmarkParseQueue(b *testing.B) {
	queue := NewParseQueue(0) // Unlimited size

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		job := &ParseJob{
			ModulePath: "test" + string(rune('0'+i%10)) + ".ts",
			Source: &source.SourceFile{
				Name:    "test.ts",
				Path:    "test.ts",
				Content: "export const test = true;",
			},
			Priority:  i % 10,
			Timestamp: time.Now(),
		}

		err := queue.Enqueue(job)
		if err != nil {
			b.Errorf("Enqueue failed: %v", err)
		}

		// Dequeue to keep queue from growing
		if i%2 == 0 {
			dequeued := queue.Dequeue()
			if dequeued != nil {
				queue.MarkCompleted(dequeued.ModulePath, &ParseResult{
					ModulePath: dequeued.ModulePath,
					Timestamp:  time.Now(),
				})
			}
		}
	}
}

func BenchmarkDependencyAnalyzer(b *testing.B) {
	analyzer := NewDependencyAnalyzer()

	// Setup dependency graph
	modules := []string{"a", "b", "c", "d", "e"}
	for _, module := range modules {
		analyzer.MarkDiscovered(module + ".ts")
	}

	// Add some dependencies
	analyzer.AddDependency("a.ts", "b.ts")
	analyzer.AddDependency("a.ts", "c.ts")
	analyzer.AddDependency("b.ts", "d.ts")
	analyzer.AddDependency("c.ts", "e.ts")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		module := modules[i%len(modules)] + ".ts"

		// Test dependency depth calculation
		depth := analyzer.GetDependencyDepth(module)
		if depth < 0 {
			b.Errorf("Invalid depth for %s: %d", module, depth)
		}

		// Test import count
		count := analyzer.GetImportCount(module)
		if count < 0 {
			b.Errorf("Invalid import count for %s: %d", module, count)
		}
	}
}

func BenchmarkWorkerPoolThroughput(b *testing.B) {
	config := DefaultLoaderConfig()
	config.NumWorkers = 4
	config.JobBufferSize = 100

	pool := NewWorkerPool(config)

	// Start the worker pool
	ctx := testContext(b)
	err := pool.Start(ctx, config.NumWorkers)
	if err != nil {
		b.Fatalf("Failed to start worker pool: %v", err)
	}
	defer func() {
		shutdownCtx := testContext(b)
		_ = pool.Shutdown(shutdownCtx)
	}()

	b.ResetTimer()

	// Submit jobs and wait for completion
	jobsSubmitted := 0
	jobsCompleted := 0

	for i := 0; i < b.N && jobsSubmitted < b.N; i++ {
		job := &ParseJob{
			ModulePath: "bench" + string(rune('0'+i%10)) + ".ts",
			Source: &source.SourceFile{
				Name:    "bench.ts",
				Path:    "bench.ts",
				Content: "export const bench = " + string(rune('0'+i%10)) + ";",
			},
			Priority:  1,
			Timestamp: time.Now(),
		}

		err := pool.Submit(job)
		if err != nil {
			b.Errorf("Job submission failed: %v", err)
			continue
		}
		jobsSubmitted++

		// Collect completed results non-blockingly
		select {
		case result := <-pool.Results():
			if result.Error != nil {
				b.Errorf("Job failed: %v", result.Error)
			}
			jobsCompleted++
		case err := <-pool.Errors():
			b.Errorf("Worker error: %v", err)
		default:
			// No result ready yet
		}
	}

	// Wait for remaining jobs to complete
	for jobsCompleted < jobsSubmitted {
		select {
		case result := <-pool.Results():
			if result.Error != nil {
				b.Errorf("Job failed: %v", result.Error)
			}
			jobsCompleted++
		case err := <-pool.Errors():
			b.Errorf("Worker error: %v", err)
		case <-time.After(5 * time.Second):
			b.Errorf("Timeout waiting for jobs to complete. Submitted: %d, Completed: %d", jobsSubmitted, jobsCompleted)
			return
		}
	}
}

// Helper function to create test context with timeout
func testContext(b *testing.B) *testCtx {
	return &testCtx{
		deadline: time.Now().Add(10 * time.Second),
		done:     make(chan struct{}),
	}
}

// Simple context implementation for tests
type testCtx struct {
	deadline time.Time
	done     chan struct{}
}

func (ctx *testCtx) Deadline() (deadline time.Time, ok bool) {
	return ctx.deadline, true
}

func (ctx *testCtx) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *testCtx) Err() error {
	select {
	case <-ctx.done:
		return nil
	default:
		if time.Now().After(ctx.deadline) {
			return &timeoutError{}
		}
		return nil
	}
}

func (ctx *testCtx) Value(key interface{}) interface{} {
	return nil
}

type timeoutError struct{}

func (e *timeoutError) Error() string {
	return "context deadline exceeded"
}

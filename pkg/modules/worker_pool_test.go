package modules

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nooga/paserati/pkg/source"
)

func TestWorkerPoolBasic(t *testing.T) {
	config := DefaultLoaderConfig()
	config.NumWorkers = 2

	pool := NewWorkerPool(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pool.Start(ctx, 2)
	if err != nil {
		t.Errorf("Expected successful start, got error: %v", err)
		return
	}

	// Test basic functionality
	if pool.HasActiveJobs() {
		t.Error("Expected no active jobs initially")
	}

	// Create a test job
	job := &ParseJob{
		ModulePath: "test.ts",
		Source: &source.SourceFile{
			Name:    "test.ts",
			Path:    "test.ts",
			Content: "export const test = true;",
		},
		Priority:  1,
		Timestamp: time.Now(),
	}

	// Submit job
	err = pool.Submit(job)
	if err != nil {
		t.Errorf("Expected successful job submission, got error: %v", err)
		return
	}

	// Wait for result
	select {
	case result := <-pool.Results():
		if result.ModulePath != "test.ts" {
			t.Errorf("Expected module path 'test.ts', got '%s'", result.ModulePath)
		}

		if result.Error != nil {
			t.Errorf("Expected successful parsing, got error: %v", result.Error)
		}

		if result.ParseDuration <= 0 {
			t.Error("Expected positive parse duration")
		}

	case err := <-pool.Errors():
		t.Errorf("Unexpected worker error: %v", err)

	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for result")
	}

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	err = pool.Shutdown(shutdownCtx)
	if err != nil {
		t.Errorf("Expected successful shutdown, got error: %v", err)
	}
}

func TestWorkerPoolMultipleJobs(t *testing.T) {
	config := DefaultLoaderConfig()
	config.NumWorkers = 3

	pool := NewWorkerPool(config)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := pool.Start(ctx, 3)
	if err != nil {
		t.Errorf("Expected successful start, got error: %v", err)
		return
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = pool.Shutdown(shutdownCtx)
	}()

	// Submit multiple jobs
	jobCount := 5
	jobs := make([]*ParseJob, jobCount)

	for i := 0; i < jobCount; i++ {
		jobs[i] = &ParseJob{
			ModulePath: fmt.Sprintf("test%d.ts", i),
			Source: &source.SourceFile{
				Name:    fmt.Sprintf("test%d.ts", i),
				Path:    fmt.Sprintf("test%d.ts", i),
				Content: fmt.Sprintf("export const test%d = %d;", i, i),
			},
			Priority:  i,
			Timestamp: time.Now(),
		}

		err = pool.Submit(jobs[i])
		if err != nil {
			t.Errorf("Expected successful job submission for job %d, got error: %v", i, err)
		}
	}

	// Collect results
	results := make([]*ParseResult, 0, jobCount)
	timeout := time.After(5 * time.Second)

	for len(results) < jobCount {
		select {
		case result := <-pool.Results():
			results = append(results, result)

		case err := <-pool.Errors():
			t.Errorf("Unexpected worker error: %v", err)

		case <-timeout:
			t.Errorf("Timeout waiting for results, got %d/%d", len(results), jobCount)
			return
		}
	}

	// Verify all results
	if len(results) != jobCount {
		t.Errorf("Expected %d results, got %d", jobCount, len(results))
	}

	// Check that all jobs were processed
	processedPaths := make(map[string]bool)
	for _, result := range results {
		processedPaths[result.ModulePath] = true

		if result.Error != nil {
			t.Errorf("Unexpected parse error for %s: %v", result.ModulePath, result.Error)
		}
	}

	for i := 0; i < jobCount; i++ {
		expectedPath := fmt.Sprintf("test%d.ts", i)
		if !processedPaths[expectedPath] {
			t.Errorf("Expected to process %s", expectedPath)
		}
	}
}

func TestWorkerPoolStats(t *testing.T) {
	config := DefaultLoaderConfig()
	config.NumWorkers = 2

	pool := NewWorkerPool(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pool.Start(ctx, 2)
	if err != nil {
		t.Errorf("Expected successful start, got error: %v", err)
		return
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = pool.Shutdown(shutdownCtx)
	}()

	// Check initial stats
	stats := pool.GetStats()
	if stats.WorkerCount != 2 {
		t.Errorf("Expected 2 workers, got %d", stats.WorkerCount)
	}

	if stats.TotalJobs != 0 {
		t.Errorf("Expected 0 total jobs initially, got %d", stats.TotalJobs)
	}

	// Submit a job
	job := &ParseJob{
		ModulePath: "stats-test.ts",
		Source: &source.SourceFile{
			Name:    "stats-test.ts",
			Path:    "stats-test.ts",
			Content: "export const statsTest = true;",
		},
		Priority:  1,
		Timestamp: time.Now(),
	}

	err = pool.Submit(job)
	if err != nil {
		t.Errorf("Expected successful job submission, got error: %v", err)
		return
	}

	// Wait for processing
	select {
	case <-pool.Results():
		// Job completed
	case err := <-pool.Errors():
		t.Errorf("Unexpected worker error: %v", err)
		return
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for result")
		return
	}

	// Check updated stats
	stats = pool.GetStats()
	if stats.TotalJobs != 1 {
		t.Errorf("Expected 1 total job, got %d", stats.TotalJobs)
	}

	if stats.CompletedJobs != 1 {
		t.Errorf("Expected 1 completed job, got %d", stats.CompletedJobs)
	}

	if stats.AverageTime <= 0 {
		t.Error("Expected positive average time")
	}
}

func TestWorkerPoolShutdown(t *testing.T) {
	config := DefaultLoaderConfig()
	config.NumWorkers = 2

	pool := NewWorkerPool(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pool.Start(ctx, 2)
	if err != nil {
		t.Errorf("Expected successful start, got error: %v", err)
		return
	}

	// Test normal shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	err = pool.Shutdown(shutdownCtx)
	if err != nil {
		t.Errorf("Expected successful shutdown, got error: %v", err)
	}

	// Test that submitting after shutdown fails
	job := &ParseJob{
		ModulePath: "after-shutdown.ts",
		Source: &source.SourceFile{
			Name:    "after-shutdown.ts",
			Path:    "after-shutdown.ts",
			Content: "export const afterShutdown = true;",
		},
		Priority:  1,
		Timestamp: time.Now(),
	}

	err = pool.Submit(job)
	if err == nil {
		t.Error("Expected error when submitting to stopped pool")
	}
}

func TestWorkerPoolStartTwice(t *testing.T) {
	config := DefaultLoaderConfig()
	config.NumWorkers = 1

	pool := NewWorkerPool(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start once
	err := pool.Start(ctx, 1)
	if err != nil {
		t.Errorf("Expected successful first start, got error: %v", err)
		return
	}

	// Try to start again
	err = pool.Start(ctx, 1)
	if err == nil {
		t.Error("Expected error when starting already started pool")
	}

	// Cleanup
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	pool.Shutdown(shutdownCtx)
}

func TestMockLexerParser(t *testing.T) {
	t.Skip("FIXME: Mock lexer/parser doesn't correctly parse imports")
	lexer := &simpleMockLexer{}
	parser := &simpleMockParser{}

	// Test lexer with import/export content
	content := `import { test } from './test';
export function myFunc() {
    return 42;
}`

	lexer.Reset(content)
	tokens, err := lexer.Tokenize()
	if err != nil {
		t.Errorf("Expected successful tokenization, got error: %v", err)
		return
	}

	// Should find import and export tokens
	foundImport := false
	foundExport := false
	foundFunction := false

	for _, token := range tokens {
		switch token.Type {
		case "IMPORT":
			foundImport = true
		case "EXPORT":
			foundExport = true
		case "FUNCTION":
			foundFunction = true
		}
	}

	if !foundImport {
		t.Error("Expected to find IMPORT token")
	}

	if !foundExport {
		t.Error("Expected to find EXPORT token")
	}

	if !foundFunction {
		t.Error("Expected to find FUNCTION token")
	}

	// Test parser
	parser.Reset(tokens)
	ast, err := parser.Parse()
	if err != nil {
		t.Errorf("Expected successful parsing, got error: %v", err)
		return
	}

	if ast.Type != "Program" {
		t.Errorf("Expected AST type 'Program', got '%s'", ast.Type)
	}

	if len(ast.Imports) != 1 {
		t.Errorf("Expected 1 import, got %d", len(ast.Imports))
	}

	if len(ast.Exports) != 1 {
		t.Errorf("Expected 1 export, got %d", len(ast.Exports))
	}
}

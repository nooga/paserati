package modules

import (
	"testing"
	"time"
)

func TestRegistryBasicOperations(t *testing.T) {
	registry := NewRegistry(DefaultLoaderConfig())
	
	// Test initial state
	if registry.Size() != 0 {
		t.Errorf("Expected empty registry, got size %d", registry.Size())
	}
	
	// Test Set and Get
	record := &ModuleRecord{
		Specifier:    "./test.ts",
		ResolvedPath: "/path/to/test.ts",
		State:        ModuleLoaded,
		LoadTime:     time.Now(),
	}
	
	registry.Set("./test.ts", record)
	
	if registry.Size() != 1 {
		t.Errorf("Expected registry size 1, got %d", registry.Size())
	}
	
	retrieved := registry.Get("./test.ts")
	if retrieved == nil {
		t.Error("Expected to retrieve record, got nil")
	} else if retrieved.Specifier != "./test.ts" {
		t.Errorf("Expected specifier './test.ts', got '%s'", retrieved.Specifier)
	}
	
	// Test Get non-existent
	nonExistent := registry.Get("./nonexistent.ts")
	if nonExistent != nil {
		t.Error("Expected nil for non-existent module, got record")
	}
}

func TestRegistryList(t *testing.T) {
	registry := NewRegistry(DefaultLoaderConfig())
	
	// Add multiple modules
	modules := []string{"./a.ts", "./b.ts", "./c.ts"}
	for _, spec := range modules {
		record := &ModuleRecord{
			Specifier: spec,
			State:     ModuleLoaded,
			LoadTime:  time.Now(),
		}
		registry.Set(spec, record)
	}
	
	list := registry.List()
	if len(list) != 3 {
		t.Errorf("Expected 3 modules in list, got %d", len(list))
	}
	
	// Check all modules are present
	for _, spec := range modules {
		found := false
		for _, listed := range list {
			if listed == spec {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Module %s not found in list", spec)
		}
	}
}

func TestRegistryRemove(t *testing.T) {
	registry := NewRegistry(DefaultLoaderConfig())
	
	record := &ModuleRecord{
		Specifier: "./test.ts",
		State:     ModuleLoaded,
		LoadTime:  time.Now(),
	}
	
	registry.Set("./test.ts", record)
	
	if registry.Size() != 1 {
		t.Errorf("Expected size 1 after set, got %d", registry.Size())
	}
	
	registry.Remove("./test.ts")
	
	if registry.Size() != 0 {
		t.Errorf("Expected size 0 after remove, got %d", registry.Size())
	}
	
	retrieved := registry.Get("./test.ts")
	if retrieved != nil {
		t.Error("Expected nil after remove, got record")
	}
}

func TestRegistryClear(t *testing.T) {
	registry := NewRegistry(DefaultLoaderConfig())
	
	// Add multiple modules
	for i := 0; i < 5; i++ {
		record := &ModuleRecord{
			Specifier: "./test" + string(rune('0'+i)) + ".ts",
			State:     ModuleLoaded,
			LoadTime:  time.Now(),
		}
		registry.Set(record.Specifier, record)
	}
	
	if registry.Size() != 5 {
		t.Errorf("Expected size 5 before clear, got %d", registry.Size())
	}
	
	registry.Clear()
	
	if registry.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", registry.Size())
	}
}

func TestRegistrySetParsed(t *testing.T) {
	registry := NewRegistry(DefaultLoaderConfig())
	
	parseResult := &ParseResult{
		ModulePath:    "./test.ts",
		ParseDuration: 50 * time.Millisecond,
		WorkerID:      1,
		Error:         nil,
		Timestamp:     time.Now(),
	}
	
	registry.SetParsed("./test.ts", parseResult)
	
	record := registry.Get("./test.ts")
	if record == nil {
		t.Error("Expected record to be created by SetParsed")
		return
	}
	
	if record.State != ModuleParsed {
		t.Errorf("Expected state ModuleParsed, got %s", record.State)
	}
	
	if record.ParseDuration != parseResult.ParseDuration {
		t.Errorf("Expected parse duration %v, got %v", parseResult.ParseDuration, record.ParseDuration)
	}
	
	if record.WorkerID != parseResult.WorkerID {
		t.Errorf("Expected worker ID %d, got %d", parseResult.WorkerID, record.WorkerID)
	}
}

func TestRegistryTTL(t *testing.T) {
	config := DefaultLoaderConfig()
	config.CacheTTL = 10 * time.Millisecond // Very short TTL for testing
	
	registry := NewRegistry(config)
	
	record := &ModuleRecord{
		Specifier: "./test.ts",
		State:     ModuleLoaded,
		LoadTime:  time.Now(),
	}
	
	registry.Set("./test.ts", record)
	
	// Should be available immediately
	retrieved := registry.Get("./test.ts")
	if retrieved == nil {
		t.Error("Expected record to be available immediately")
	}
	
	// Wait for TTL to expire
	time.Sleep(15 * time.Millisecond)
	
	// Should now return nil due to TTL expiry
	expired := registry.Get("./test.ts")
	if expired != nil {
		t.Error("Expected record to be expired due to TTL")
	}
}

func TestRegistryCacheSize(t *testing.T) {
	config := DefaultLoaderConfig()
	config.CacheSize = 2 // Limit cache to 2 modules
	
	registry := NewRegistry(config)
	
	// Add 3 modules (should evict the oldest)
	modules := []string{"./a.ts", "./b.ts", "./c.ts"}
	for i, spec := range modules {
		record := &ModuleRecord{
			Specifier: spec,
			State:     ModuleLoaded,
			LoadTime:  time.Now().Add(time.Duration(i) * time.Millisecond), // Different times
		}
		registry.Set(spec, record)
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}
	
	// Should only have 2 modules (oldest evicted)
	if registry.Size() != 2 {
		t.Errorf("Expected size 2 due to cache limit, got %d", registry.Size())
	}
	
	// First module should be evicted
	first := registry.Get("./a.ts")
	if first != nil {
		t.Error("Expected first module to be evicted")
	}
	
	// Other modules should still be present
	second := registry.Get("./b.ts")
	if second == nil {
		t.Error("Expected second module to be present")
	}
	
	third := registry.Get("./c.ts")
	if third == nil {
		t.Error("Expected third module to be present")
	}
}

func TestRegistryStats(t *testing.T) {
	registry := NewRegistry(DefaultLoaderConfig())
	
	// Add successful module
	successRecord := &ModuleRecord{
		Specifier: "./success.ts",
		State:     ModuleCompiled,
		LoadTime:  time.Now(),
	}
	registry.Set("./success.ts", successRecord)
	
	// Add failed module
	failRecord := &ModuleRecord{
		Specifier: "./fail.ts",
		State:     ModuleError,
		LoadTime:  time.Now(),
	}
	registry.Set("./fail.ts", failRecord)
	
	// Test cache hit
	registry.Get("./success.ts")
	
	// Test cache miss
	registry.Get("./nonexistent.ts")
	
	// Get the stats using the interface method
	stats := registry.GetStats()
	
	if stats.TotalModules != 2 {
		t.Errorf("Expected 2 total modules, got %d", stats.TotalModules)
	}
	
	if stats.LoadedModules != 1 {
		t.Errorf("Expected 1 loaded module, got %d", stats.LoadedModules)
	}
	
	if stats.FailedModules != 1 {
		t.Errorf("Expected 1 failed module, got %d", stats.FailedModules)
	}
	
	if stats.CacheHits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", stats.CacheHits)
	}
	
	if stats.CacheMisses != 1 {
		t.Errorf("Expected 1 cache miss, got %d", stats.CacheMisses)
	}
}

func TestModuleStateString(t *testing.T) {
	states := []struct {
		state    ModuleState
		expected string
	}{
		{ModuleUnknown, "unknown"},
		{ModuleResolving, "resolving"},
		{ModuleResolved, "resolved"},
		{ModuleLoading, "loading"},
		{ModuleLoaded, "loaded"},
		{ModuleParsing, "parsing"},
		{ModuleParsed, "parsed"},
		{ModuleChecking, "checking"},
		{ModuleChecked, "checked"},
		{ModuleCompiling, "compiling"},
		{ModuleCompiled, "compiled"},
		{ModuleError, "error"},
		{ModuleState(999), "invalid"},
	}
	
	for _, test := range states {
		result := test.state.String()
		if result != test.expected {
			t.Errorf("Expected state %d to string as '%s', got '%s'", test.state, test.expected, result)
		}
	}
}

func TestImportTypeString(t *testing.T) {
	types := []struct {
		importType ImportType
		expected   string
	}{
		{ImportDefault, "default"},
		{ImportNamed, "named"},
		{ImportNamespace, "namespace"},
		{ImportSideEffect, "side-effect"},
		{ImportType(999), "unknown"},
	}
	
	for _, test := range types {
		result := test.importType.String()
		if result != test.expected {
			t.Errorf("Expected import type %d to string as '%s', got '%s'", test.importType, test.expected, result)
		}
	}
}

func TestDefaultLoaderConfig(t *testing.T) {
	config := DefaultLoaderConfig()
	
	if !config.EnableParallel {
		t.Error("Expected parallel processing to be enabled by default")
	}
	
	if config.NumWorkers != 0 {
		t.Error("Expected NumWorkers to be 0 (auto-detect) by default")
	}
	
	if !config.CacheEnabled {
		t.Error("Expected caching to be enabled by default")
	}
	
	if config.CacheSize != 0 {
		t.Error("Expected unlimited cache size by default")
	}
	
	if config.CacheTTL != 0 {
		t.Error("Expected no cache TTL by default")
	}
	
	if !config.PrewarmLexers {
		t.Error("Expected lexer prewarming to be enabled by default")
	}
}
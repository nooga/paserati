package modules

import (
	"io"
	"strings"
	"testing"
)

func TestMemoryResolverBasic(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	if resolver.Name() != "TestMemory" {
		t.Errorf("Expected name 'TestMemory', got '%s'", resolver.Name())
	}
	
	if resolver.Priority() != 50 {
		t.Errorf("Expected priority 50, got %d", resolver.Priority())
	}
}

func TestMemoryResolverAddModule(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	content := `export function greet(name: string): string {
    return "Hello, " + name + "!";
}`
	
	resolver.AddModule("./greet.ts", content)
	
	modules := resolver.ListModules()
	if len(modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(modules))
	}
	
	if modules[0] != "./greet.ts" {
		t.Errorf("Expected module './greet.ts', got '%s'", modules[0])
	}
	
	module := resolver.GetModule("./greet.ts")
	if module == nil {
		t.Error("Expected to find module, got nil")
		return
	}
	
	if module.Content != content {
		t.Errorf("Expected content to match, got different content")
	}
}

func TestMemoryResolverCanResolve(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	// Add test modules
	resolver.AddModule("test.ts", "export const test = true;")
	resolver.AddModule("utils/index.ts", "export * from './helper';")
	resolver.AddModule("utils/helper.ts", "export function help() {}")
	
	tests := []struct {
		specifier string
		canResolve bool
	}{
		{"./test.ts", true},     // Exact match
		{"./test", true},        // Without extension
		{"./utils", true},       // Directory with index
		{"./nonexistent", false}, // Doesn't exist
		{"./utils/helper.ts", true}, // Nested module
		{"./utils/helper", true},    // Nested without extension
	}
	
	for _, test := range tests {
		result := resolver.CanResolve(test.specifier)
		if result != test.canResolve {
			t.Errorf("CanResolve('%s') = %v, expected %v", test.specifier, result, test.canResolve)
		}
	}
}

func TestMemoryResolverResolve(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	content := `export function greet(name: string): string {
    return "Hello, " + name + "!";
}`
	
	resolver.AddModule("greet.ts", content)
	
	resolved, err := resolver.Resolve("./greet.ts", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.Specifier != "./greet.ts" {
		t.Errorf("Expected specifier './greet.ts', got '%s'", resolved.Specifier)
	}
	
	if resolved.ResolvedPath != "greet.ts" {
		t.Errorf("Expected resolved path 'greet.ts', got '%s'", resolved.ResolvedPath)
	}
	
	if resolved.Resolver != "TestMemory" {
		t.Errorf("Expected resolver 'TestMemory', got '%s'", resolved.Resolver)
	}
	
	// Test reading the source
	sourceBytes, err := io.ReadAll(resolved.Source)
	if err != nil {
		t.Errorf("Expected to read source, got error: %v", err)
		return
	}
	
	if string(sourceBytes) != content {
		t.Errorf("Expected source content to match")
	}
	
	// Test closing the source
	err = resolved.Source.Close()
	if err != nil {
		t.Errorf("Expected successful close, got error: %v", err)
	}
}

func TestMemoryResolverResolveWithExtension(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	resolver.AddModule("test.ts", "export const test = true;")
	
	// Should resolve without extension
	resolved, err := resolver.Resolve("./test", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "test.ts" {
		t.Errorf("Expected resolved path 'test.ts', got '%s'", resolved.ResolvedPath)
	}
}

func TestMemoryResolverResolveDirectory(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	resolver.AddModule("utils/index.ts", "export * from './helper';")
	
	// Should resolve directory to index file
	resolved, err := resolver.Resolve("./utils", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "utils/index.ts" {
		t.Errorf("Expected resolved path 'utils/index.ts', got '%s'", resolved.ResolvedPath)
	}
}

func TestMemoryResolverRelativeResolution(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	resolver.AddModule("src/utils/helper.ts", "export function help() {}")
	
	// Resolve relative import from ./src/main.ts
	resolved, err := resolver.Resolve("./utils/helper", "src/main.ts")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "src/utils/helper.ts" {
		t.Errorf("Expected resolved path 'src/utils/helper.ts', got '%s'", resolved.ResolvedPath)
	}
}

func TestMemoryResolverUpdateModule(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	originalContent := "export const version = '1.0.0';"
	resolver.AddModule("./config.ts", originalContent)
	
	module := resolver.GetModule("./config.ts")
	if module == nil {
		t.Error("Expected to find module")
		return
	}
	
	originalModified := module.Modified
	
	// Update the module
	newContent := "export const version = '2.0.0';"
	err := resolver.UpdateModule("./config.ts", newContent)
	if err != nil {
		t.Errorf("Expected successful update, got error: %v", err)
		return
	}
	
	// Check updated content
	updatedModule := resolver.GetModule("./config.ts")
	if updatedModule.Content != newContent {
		t.Errorf("Expected updated content, got original content")
	}
	
	// Check modified time was updated
	if !updatedModule.Modified.After(originalModified) {
		t.Error("Expected modified time to be updated")
	}
}

func TestMemoryResolverUpdateNonExistent(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	err := resolver.UpdateModule("./nonexistent.ts", "new content")
	if err == nil {
		t.Error("Expected error when updating non-existent module")
	}
	
	if !strings.Contains(err.Error(), "module not found") {
		t.Errorf("Expected 'module not found' error, got: %v", err)
	}
}

func TestMemoryResolverRemoveModule(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	resolver.AddModule("temp.ts", "export const temp = true;")
	
	// Verify module exists
	if !resolver.CanResolve("./temp.ts") {
		t.Error("Expected module to exist before removal")
	}
	
	resolver.RemoveModule("temp.ts")
	
	// Verify module was removed
	if resolver.CanResolve("./temp.ts") {
		t.Error("Expected module to be removed")
	}
	
	modules := resolver.ListModules()
	if len(modules) != 0 {
		t.Errorf("Expected 0 modules after removal, got %d", len(modules))
	}
}

func TestMemoryResolverClear(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	// Add multiple modules
	resolver.AddModule("./a.ts", "export const a = 1;")
	resolver.AddModule("./b.ts", "export const b = 2;")
	resolver.AddModule("./c.ts", "export const c = 3;")
	
	if len(resolver.ListModules()) != 3 {
		t.Error("Expected 3 modules before clear")
	}
	
	resolver.Clear()
	
	modules := resolver.ListModules()
	if len(modules) != 0 {
		t.Errorf("Expected 0 modules after clear, got %d", len(modules))
	}
}

func TestMemoryResolverSetPriority(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	originalPriority := resolver.Priority()
	if originalPriority != 50 {
		t.Errorf("Expected default priority 50, got %d", originalPriority)
	}
	
	resolver.SetPriority(25)
	
	newPriority := resolver.Priority()
	if newPriority != 25 {
		t.Errorf("Expected priority 25 after setting, got %d", newPriority)
	}
}

func TestMemoryResolverFailureCase(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	// Try to resolve non-existent module
	_, err := resolver.Resolve("./nonexistent.ts", "")
	if err == nil {
		t.Error("Expected error when resolving non-existent module")
	}
	
	if !strings.Contains(err.Error(), "module not found") {
		t.Errorf("Expected 'module not found' error, got: %v", err)
	}
}

func TestMemoryResolverAddTestModules(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	resolver.AddTestModules()
	
	modules := resolver.ListModules()
	if len(modules) < 5 {
		t.Errorf("Expected at least 5 test modules, got %d", len(modules))
	}
	
	// Test that we can resolve the test modules
	testCases := []string{
		"test/module-a.ts",
		"test/module-b.ts", 
		"test/utils/index.ts",
		"test/utils/helper.ts",
		"test/utils/config.ts",
	}
	
	for _, testCase := range testCases {
		if !resolver.CanResolve(testCase) {
			t.Errorf("Expected to be able to resolve test module: %s", testCase)
		}
	}
	
	// Test resolving the utils directory
	resolved, err := resolver.Resolve("test/utils", "")
	if err != nil {
		t.Errorf("Expected to resolve test/utils directory, got error: %v", err)
	} else if resolved.ResolvedPath != "test/utils/index.ts" {
		t.Errorf("Expected test/utils to resolve to index.ts, got: %s", resolved.ResolvedPath)
	}
}

func TestMemoryFS(t *testing.T) {
	resolver := NewMemoryResolver("TestMemory")
	
	content := "export const test = true;"
	resolver.AddModule("test.ts", content)
	
	// Get the memory FS
	_, err := resolver.Resolve("./test.ts", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	// Test the memory FS directly
	memFS := &memoryFS{resolver: resolver}
	
	// Test ReadFile
	fileContent, err := memFS.ReadFile("test.ts")
	if err != nil {
		t.Errorf("Expected to read file, got error: %v", err)
		return
	}
	
	if string(fileContent) != content {
		t.Errorf("Expected file content to match")
	}
	
	// Test Open
	file, err := memFS.Open("test.ts")
	if err != nil {
		t.Errorf("Expected to open file, got error: %v", err)
		return
	}
	defer file.Close()
	
	openContent, err := io.ReadAll(file)
	if err != nil {
		t.Errorf("Expected to read opened file, got error: %v", err)
		return
	}
	
	if string(openContent) != content {
		t.Errorf("Expected opened file content to match")
	}
}
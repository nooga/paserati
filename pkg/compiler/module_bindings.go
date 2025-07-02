package compiler

import (
	"paserati/pkg/modules"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

// ModuleBindings handles runtime module binding resolution during compilation
// This parallels the type checker's ModuleEnvironment but for runtime values
type ModuleBindings struct {
	// Module-specific information
	ModulePath    string                    // Current module's resolved path
	ModuleLoader  modules.ModuleLoader      // Reference to module loader
	
	// Import/Export tracking (parallel to type checker)
	ImportedNames map[string]*ImportReference // local_name -> import info
	ExportedNames map[string]*ExportReference // export_name -> export info
	DefaultExport *ExportReference            // Default export info
	
	// Module dependencies (for circular dependency detection)
	Dependencies  map[string]bool             // Set of module paths this module depends on
}

// ImportReference represents an imported name's runtime binding information
// Parallels type checker's ImportBinding but for runtime values
type ImportReference struct {
	LocalName    string                 // Name used locally in this module
	SourceModule string                 // Path of the source module
	SourceName   string                 // Name in the source module ("default" for default imports)
	ImportType   ImportReferenceType    // Type of import binding
	ResolvedValue vm.Value               // Resolved value from source module
	GlobalIndex  int                    // Global heap index where this import resolves to (-1 if not resolved)
}

// ExportReference represents an exported name's runtime binding information  
// Parallels type checker's ExportBinding but for runtime values
type ExportReference struct {
	LocalName    string               // Name used locally in this module
	ExportName   string               // Name when exported (may differ due to aliases)
	ExportedValue vm.Value             // Value being exported
	Declaration  parser.Statement     // Original declaration (if any)
	IsReExport   bool                 // True if this is a re-export from another module
	SourceModule string               // For re-exports, the source module path
	GlobalIndex  int                  // Global heap index where this export is stored (-1 if not stored as global)
}

// ImportReferenceType represents different kinds of import bindings
// Same as type checker's ImportBindingType
type ImportReferenceType int

const (
	ImportDefaultRef   ImportReferenceType = iota // import defaultName from "module"
	ImportNamedRef                                // import { name } from "module"  
	ImportNamespaceRef                            // import * as name from "module"
)

// NewModuleBindings creates a new module-aware binding resolver
func NewModuleBindings(modulePath string, loader modules.ModuleLoader) *ModuleBindings {
	return &ModuleBindings{
		ModulePath:    modulePath,
		ModuleLoader:  loader,
		ImportedNames: make(map[string]*ImportReference),
		ExportedNames: make(map[string]*ExportReference),
		Dependencies:  make(map[string]bool),
	}
}

// DefineImport adds an import binding to the module bindings
// Parallels type checker's ModuleEnvironment.DefineImport
func (mb *ModuleBindings) DefineImport(localName, sourceModule, sourceName string, importType ImportReferenceType, globalIndex int) {
	reference := &ImportReference{
		LocalName:     localName,
		SourceModule:  sourceModule, 
		SourceName:    sourceName,
		ImportType:    importType,
		ResolvedValue: vm.Undefined, // Will be resolved later
		GlobalIndex:   globalIndex,
	}
	
	mb.ImportedNames[localName] = reference
	mb.Dependencies[sourceModule] = true
}

// DefineExport adds an export binding to the module bindings
// Parallels type checker's ModuleEnvironment.DefineExport
func (mb *ModuleBindings) DefineExport(localName, exportName string, exportedValue vm.Value, decl parser.Statement, globalIndex int) {
	reference := &ExportReference{
		LocalName:     localName,
		ExportName:    exportName,
		ExportedValue: exportedValue,
		Declaration:   decl,
		IsReExport:    false,
		GlobalIndex:   globalIndex,
	}
	
	if exportName == "default" {
		mb.DefaultExport = reference
	} else {
		mb.ExportedNames[exportName] = reference
	}
}

// DefineReExport adds a re-export binding (export { name } from "module")
// Parallels type checker's ModuleEnvironment.DefineReExport
func (mb *ModuleBindings) DefineReExport(exportName, sourceModule, sourceName string) {
	reference := &ExportReference{
		LocalName:     sourceName,    // In re-exports, we use the source name
		ExportName:    exportName,
		ExportedValue: vm.Undefined,  // Will be resolved from source module
		Declaration:   nil,           // No local declaration
		IsReExport:    true,
		SourceModule:  sourceModule,
	}
	
	mb.ExportedNames[exportName] = reference
	mb.Dependencies[sourceModule] = true
}

// ResolveImportedValue resolves the actual runtime value of an imported name
// Parallels type checker's ModuleEnvironment.ResolveImportedType
func (mb *ModuleBindings) ResolveImportedValue(localName string) vm.Value {
	reference, exists := mb.ImportedNames[localName]
	if !exists {
		return vm.Undefined
	}
	
	// If we already resolved this value, return it
	if reference.ResolvedValue != vm.Undefined {
		return reference.ResolvedValue
	}
	
	// Try to resolve the value from the source module using ModuleLoader
	if mb.ModuleLoader != nil {
		if sourceModule := mb.ModuleLoader.GetModule(reference.SourceModule); sourceModule != nil {
			// Look up the exported value from the source module
			if reference.ImportType == ImportNamespaceRef {
				// For namespace imports, create a namespace object with all exports
				reference.ResolvedValue = mb.createNamespaceObject(sourceModule.ExportValues)
			} else {
				// For named/default imports, look up the specific export
				if exportValue, exists := sourceModule.ExportValues[reference.SourceName]; exists {
					reference.ResolvedValue = exportValue
				} else {
					// Export not found in source module - leave as undefined
					// This will be caught during runtime as an error
				}
			}
		}
	}
	
	return reference.ResolvedValue
}

// createNamespaceObject creates a namespace object for "import * as name" imports
// Parallels type checker's ModuleEnvironment.createNamespaceType
func (mb *ModuleBindings) createNamespaceObject(exports map[string]vm.Value) vm.Value {
	// Create a new namespace object that contains all the module's exports
	namespace := vm.NewDictObject(vm.DefaultObjectPrototype)
	
	// Convert to DictObject to use SetOwn method
	if namespace.Type() == vm.TypeObject {
		namespaceDict := namespace.AsDictObject()
		for exportName, exportValue := range exports {
			namespaceDict.SetOwn(exportName, exportValue)
		}
	}
	
	return namespace
}

// GetExportedValue gets the runtime value of an exported name
// Parallels type checker's ModuleEnvironment.GetExportedType
func (mb *ModuleBindings) GetExportedValue(exportName string) (vm.Value, bool) {
	if exportName == "default" && mb.DefaultExport != nil {
		return mb.DefaultExport.ExportedValue, true
	}
	
	if reference, exists := mb.ExportedNames[exportName]; exists {
		return reference.ExportedValue, true
	}
	
	return vm.Undefined, false
}

// GetAllExports returns all exported names and their runtime values
// Parallels type checker's ModuleEnvironment.GetAllExports
func (mb *ModuleBindings) GetAllExports() map[string]vm.Value {
	exports := make(map[string]vm.Value)
	
	// Add named exports
	for exportName, reference := range mb.ExportedNames {
		exports[exportName] = reference.ExportedValue
	}
	
	// Add default export if it exists
	if mb.DefaultExport != nil {
		exports["default"] = mb.DefaultExport.ExportedValue
	}
	
	return exports
}

// HasCircularDependency checks if adding a dependency would create a cycle
// Parallels type checker's ModuleEnvironment.HasCircularDependency
func (mb *ModuleBindings) HasCircularDependency(targetModule string) bool {
	// This is a simplified check - a full implementation would use the
	// dependency graph from the ModuleLoader
	return mb.Dependencies[targetModule]
}

// UpdateExportValue updates the runtime value of an exported binding
// Parallels type checker's ModuleEnvironment.UpdateExportType
func (mb *ModuleBindings) UpdateExportValue(exportName string, newValue vm.Value) {
	if exportName == "default" && mb.DefaultExport != nil {
		mb.DefaultExport.ExportedValue = newValue
	} else if reference, exists := mb.ExportedNames[exportName]; exists {
		reference.ExportedValue = newValue
	}
}

// IsImported checks if a name is an imported binding
func (mb *ModuleBindings) IsImported(name string) bool {
	_, exists := mb.ImportedNames[name]
	return exists
}

// IsExported checks if a name is an exported binding
func (mb *ModuleBindings) IsExported(name string) bool {
	if name == "default" && mb.DefaultExport != nil {
		return true
	}
	_, exists := mb.ExportedNames[name]
	return exists
}
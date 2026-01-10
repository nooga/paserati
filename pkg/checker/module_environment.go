package checker

import (
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// debugPrintf is imported from checker.go

// ModuleEnvironment extends Environment with module-aware type resolution
type ModuleEnvironment struct {
	*Environment // Embed base environment

	// Module-specific information
	ModulePath   string               // Current module's resolved path
	ModuleLoader modules.ModuleLoader // Reference to module loader

	// Import/Export tracking
	ImportedNames map[string]*ImportBinding // local_name -> import info
	ExportedNames map[string]*ExportBinding // export_name -> export info
	DefaultExport *ExportBinding            // Default export info

	// Module dependencies (for circular dependency detection)
	Dependencies map[string]bool // Set of module paths this module depends on
}

// ImportBinding represents an imported name's binding information
type ImportBinding struct {
	LocalName    string            // Name used locally in this module
	SourceModule string            // Path of the source module
	SourceName   string            // Name in the source module ("default" for default imports)
	ImportType   ImportBindingType // Type of import binding
	ResolvedType types.Type        // Resolved type from source module
}

// ExportBinding represents an exported name's binding information
type ExportBinding struct {
	LocalName    string           // Name used locally in this module
	ExportName   string           // Name when exported (may differ due to aliases)
	ExportedType types.Type       // Type being exported
	Declaration  parser.Statement // Original declaration (if any)
	IsReExport   bool             // True if this is a re-export from another module
	SourceModule string           // For re-exports, the source module path
	IsTypeOnly   bool             // True if this is a type-only export
}

// ImportBindingType represents different kinds of import bindings
type ImportBindingType int

const (
	ImportDefault   ImportBindingType = iota // import defaultName from "module"
	ImportNamed                              // import { name } from "module"
	ImportNamespace                          // import * as name from "module"
)

// NewModuleEnvironment creates a new module-aware environment
func NewModuleEnvironment(parent *Environment, modulePath string, loader modules.ModuleLoader) *ModuleEnvironment {
	return &ModuleEnvironment{
		Environment:   parent,
		ModulePath:    modulePath,
		ModuleLoader:  loader,
		ImportedNames: make(map[string]*ImportBinding),
		ExportedNames: make(map[string]*ExportBinding),
		Dependencies:  make(map[string]bool),
	}
}

// DefineImport adds an import binding to the module environment
func (me *ModuleEnvironment) DefineImport(localName, sourceModule, sourceName string, importType ImportBindingType) {
	binding := &ImportBinding{
		LocalName:    localName,
		SourceModule: sourceModule,
		SourceName:   sourceName,
		ImportType:   importType,
		ResolvedType: types.Any, // Will be resolved later
	}

	me.ImportedNames[localName] = binding
	me.Dependencies[sourceModule] = true

	// Also define in the base environment so normal resolution works
	me.Environment.Define(localName, types.Any, false)
}

// DefineExport adds an export binding to the module environment
func (me *ModuleEnvironment) DefineExport(localName, exportName string, exportedType types.Type, decl parser.Statement) {
	binding := &ExportBinding{
		LocalName:    localName,
		ExportName:   exportName,
		ExportedType: exportedType,
		Declaration:  decl,
		IsReExport:   false,
	}

	if exportName == "default" {
		me.DefaultExport = binding
	} else {
		me.ExportedNames[exportName] = binding
	}
}

// DefineReExport adds a re-export binding (export { name } from "module")
func (me *ModuleEnvironment) DefineReExport(exportName, sourceModule, sourceName string, isTypeOnly bool) {
	binding := &ExportBinding{
		LocalName:    sourceName, // In re-exports, we use the source name
		ExportName:   exportName,
		ExportedType: types.Any, // Will be resolved from source module
		Declaration:  nil,       // No local declaration
		IsReExport:   true,
		SourceModule: sourceModule,
		IsTypeOnly:   isTypeOnly, // Set the type-only flag
	}

	me.ExportedNames[exportName] = binding
	me.Dependencies[sourceModule] = true
}

// ResolveImportedType resolves the actual type of an imported name
func (me *ModuleEnvironment) ResolveImportedType(localName string) types.Type {
	binding, exists := me.ImportedNames[localName]
	if !exists {
		return nil
	}

	// If we already resolved this type, return it
	if binding.ResolvedType != types.Any {
		return binding.ResolvedType
	}

	// Try to resolve the type from the source module using ModuleLoader
	if me.ModuleLoader != nil {
		if sourceModule := me.ModuleLoader.GetModule(binding.SourceModule); sourceModule != nil {
			// Look up the exported type from the source module
			if binding.ImportType == ImportNamespace {
				// For namespace imports, create a namespace type with all exports
				binding.ResolvedType = me.createNamespaceType(sourceModule.Exports)
			} else {
				// For named/default imports, look up the specific export
				if exportType, exists := sourceModule.Exports[binding.SourceName]; exists {
					binding.ResolvedType = exportType
				} else {
					// Export not found in source module
					debugPrintf("// [ModuleEnv] Export '%s' not found in module '%s'\n", binding.SourceName, binding.SourceModule)
				}
			}
		} else {
			debugPrintf("// [ModuleEnv] Source module '%s' not loaded or not found\n", binding.SourceModule)
		}
	}

	return binding.ResolvedType
}

// createNamespaceType creates a namespace type for "import * as name" imports
func (me *ModuleEnvironment) createNamespaceType(exports map[string]types.Type) types.Type {
	// For now, return a generic object type
	// In a full implementation, this would be a special namespace type
	// that contains all the module's exports
	properties := make(map[string]types.Type)

	for exportName, exportType := range exports {
		properties[exportName] = exportType
	}

	return &types.ObjectType{
		Properties: properties,
	}
}

// GetExportedType gets the type of an exported name
func (me *ModuleEnvironment) GetExportedType(exportName string) (types.Type, bool) {
	if exportName == "default" && me.DefaultExport != nil {
		// Handle default export re-exports
		if me.DefaultExport.IsReExport && me.DefaultExport.ExportedType == types.Any {
			if resolvedType := me.resolveReExportType(me.DefaultExport); resolvedType != nil {
				me.DefaultExport.ExportedType = resolvedType
			}
		}
		return me.DefaultExport.ExportedType, true
	}

	if binding, exists := me.ExportedNames[exportName]; exists {
		// Handle named export re-exports
		if binding.IsReExport && binding.ExportedType == types.Any {
			if resolvedType := me.resolveReExportType(binding); resolvedType != nil {
				binding.ExportedType = resolvedType
			}
		}
		return binding.ExportedType, true
	}

	return nil, false
}

// GetAllExports returns all exported names and their types
func (me *ModuleEnvironment) GetAllExports() map[string]types.Type {
	exports := make(map[string]types.Type)

	// Add named exports
	for exportName, binding := range me.ExportedNames {
		// Resolve re-exports if necessary
		if binding.IsReExport && binding.ExportedType == types.Any {
			if resolvedType := me.resolveReExportType(binding); resolvedType != nil {
				binding.ExportedType = resolvedType
			}
		}
		exports[exportName] = binding.ExportedType
	}

	// Add default export if it exists
	if me.DefaultExport != nil {
		// Resolve default re-export if necessary
		if me.DefaultExport.IsReExport && me.DefaultExport.ExportedType == types.Any {
			if resolvedType := me.resolveReExportType(me.DefaultExport); resolvedType != nil {
				me.DefaultExport.ExportedType = resolvedType
			}
		}
		exports["default"] = me.DefaultExport.ExportedType
	}

	return exports
}

// HasCircularDependency checks if adding a dependency would create a cycle
func (me *ModuleEnvironment) HasCircularDependency(targetModule string) bool {
	// This is a simplified check - a full implementation would use the
	// dependency graph from the ModuleLoader
	return me.Dependencies[targetModule]
}

// UpdateExportType updates the type of an exported binding (used when type is refined)
func (me *ModuleEnvironment) UpdateExportType(exportName string, newType types.Type) {
	if exportName == "default" && me.DefaultExport != nil {
		me.DefaultExport.ExportedType = newType
	} else if binding, exists := me.ExportedNames[exportName]; exists {
		binding.ExportedType = newType
	}
}

// resolveReExportType resolves the type of a re-exported binding from its source module
func (me *ModuleEnvironment) resolveReExportType(binding *ExportBinding) types.Type {
	if !binding.IsReExport || me.ModuleLoader == nil {
		return nil
	}

	debugPrintf("// [ModuleEnv] Resolving re-export: %s from %s\n", binding.LocalName, binding.SourceModule)

	// Get the source module
	sourceModule := me.ModuleLoader.GetModule(binding.SourceModule)
	if sourceModule == nil {
		debugPrintf("// [ModuleEnv] Source module '%s' not found for re-export\n", binding.SourceModule)
		return nil
	}

	// Look up the exported type from the source module
	if exportType, exists := sourceModule.Exports[binding.LocalName]; exists {
		debugPrintf("// [ModuleEnv] Resolved re-export %s as: %s\n", binding.LocalName, exportType.String())
		return exportType
	}

	debugPrintf("// [ModuleEnv] Export '%s' not found in source module '%s'\n", binding.LocalName, binding.SourceModule)
	return nil
}

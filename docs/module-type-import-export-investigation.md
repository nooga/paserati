# Module Type Import/Export Investigation

## Executive Summary

This document investigates failures in the Paserati TypeScript runtime's module system, specifically related to importing and exporting types (interfaces and type aliases) across modules. The investigation identifies root causes and provides actionable steps for resolution.

## Current Status

### Test Results
- **Scripts Test**: Only 1 expected failure (class_FIXME_abstract.ts)
- **Module Tests**: 3 failures related to type imports
  - `cross_module_types` - Compiles but has runtime namespace import error
  - `type_alias_export_import` - Compile error: "unknown type name: StringOrNumber"
  - `type_reexport` - Compile error: "unknown type name: StringOrNumber"

### Key Finding
A fix was implemented for exported interfaces in `ExportNamedDeclaration`, but type aliases still fail to export/import correctly.

## Problem Symptoms

### Symptom 1: Type Aliases Not Found During Import
```
// [ModuleEnv] Export 'StringOrNumber' not found in module './test_type_alias_export'
// [Checker] Imported StringOrNumber: StringOrNumber = any (unresolved, type-only: false)
Type Error at 5:12: unknown type name: StringOrNumber
```

### Symptom 2: Module Processing Order Issues
- Only `main.ts` is processed in failing tests
- The dependency module (`test_type_alias_export.ts`) is not loaded/processed
- Working tests show both modules being processed

### Symptom 3: Runtime Namespace Import Failures
```
[VM Runtime Error]: Cannot read property 'add' of null
```
- Occurs when accessing properties on namespace imports (`math.add`)
- The namespace object (`math`) is null at runtime

## Root Cause Analysis

### Root Cause 1: Type Checker Pass 1 Export Handling

The type checker's Pass 1 was not processing exported type declarations wrapped in `ExportNamedDeclaration`.

**Evidence:**
```go
// Before fix - Pass 1 only handled direct declarations:
if aliasStmt, ok := stmt.(*parser.TypeAliasStatement); ok {
    // Process type alias
} else if interfaceStmt, ok := stmt.(*parser.InterfaceDeclaration); ok {
    // Process interface
}
// Missing: ExportNamedDeclaration handling
```

**Partial Fix Applied:**
```go
} else if exportStmt, ok := stmt.(*parser.ExportNamedDeclaration); ok {
    // Handle exported type declarations
    if exportStmt.Declaration != nil {
        if interfaceStmt, ok := exportStmt.Declaration.(*parser.InterfaceDeclaration); ok {
            // Process exported interface
        } else if aliasStmt, ok := exportStmt.Declaration.(*parser.TypeAliasStatement); ok {
            // Process exported type alias
        }
    }
}
```

This fix resolved interface exports but type aliases still fail.

### Root Cause 2: Module Export Registration

Type aliases may not be properly registered in the module's export map after being defined.

**Investigation Steps:**

1. **Check Type Alias Definition**
   ```bash
   # Enable debug in checker
   # Look for: "Defined type alias 'StringOrNumber'"
   ```

2. **Check Export Registration**
   ```bash
   # Look for: "Exported type alias: StringOrNumber"
   # Check if DefineExport is called for type aliases
   ```

3. **Check Module Export Map**
   ```bash
   # Look for: "Module exports: {StringOrNumber: ...}"
   # Verify type is in export map
   ```

### Root Cause 3: Module Loading Order

The dependency module isn't being loaded before the main module attempts to import from it.

**Evidence:**
```
// Failing test:
// [Checker] Enabled module mode for: main.ts
// (missing test_type_alias_export.ts)

// Working test:
// [Checker] Enabled module mode for: math.ts
// [Checker] Enabled module mode for: main.ts
```

**Investigation Steps:**

1. **Trace Module Loading**
   - Add debug to module loader's LoadModule method
   - Track when each module is requested and loaded
   - Check if imports trigger module loading

2. **Check Import Processing**
   - Verify `checkImportDeclaration` triggers module loading
   - Check if type-only imports are handled differently

### Root Cause 4: Type Alias vs Interface Export Handling

Interfaces work but type aliases don't, suggesting different handling in export logic.

**Key Differences to Investigate:**

1. **checkInterfaceDeclaration vs checkTypeAliasStatement**
   - Do they call DefineExport differently?
   - Are type aliases stored in a different registry?

2. **Module Environment Storage**
   - Are type aliases stored in typeAliases map?
   - Are interfaces stored in types map?
   - Is GetExportedType checking both?

## Debugging Strategy

### Step 1: Enable Targeted Debug Output

```go
// In checker.go
const checkerDebug = true

// Add specific debug for type aliases:
func (c *Checker) checkTypeAliasStatement(node *parser.TypeAliasStatement) {
    debugPrintf("// [Checker TypeAlias] Defining type alias: %s\n", node.Name.Value)
    // ... existing code ...
    debugPrintf("// [Checker TypeAlias] Defined type alias: %s = %s\n", node.Name.Value, resolvedType)
}

// In module_environment.go
func (me *ModuleEnvironment) DefineExport(...) {
    debugPrintf("// [ModuleEnv] DefineExport: %s (type: %T)\n", exportName, exportedType)
    // ... existing code ...
}
```

### Step 2: Test Minimal Reproduction

Create a minimal test case:

```typescript
// export.ts
export type MyType = string;
export interface MyInterface { x: number; }

// import.ts  
import { MyType, MyInterface } from './export';
let a: MyType = "test";
let b: MyInterface = { x: 1 };
```

Run with debug enabled and compare interface vs type alias handling.

### Step 3: Trace Export Path

1. Set breakpoints or add debug at:
   - `checkTypeAliasStatement` - when type alias is defined
   - `processExportDeclaration` - when export is processed
   - `DefineExport` - when export is registered
   - `GetExportedType` - when import tries to resolve

2. Verify the complete path for both interfaces and type aliases

### Step 4: Check Module Export Retrieval

In the importing module, trace:
1. How imports are resolved
2. What's in the source module's export map
3. Why type aliases aren't found

## Proposed Fixes

### Fix 1: Ensure Type Aliases Are Exported

Check if `processExportDeclaration` needs to handle type aliases:

```go
func (c *Checker) processExportDeclaration(decl parser.Statement) {
    switch d := decl.(type) {
    case *parser.TypeAliasStatement:
        if c.IsModuleMode() && d.Name != nil {
            // Get the defined type
            aliasType, _ := c.env.ResolveType(d.Name.Value)
            if aliasType != nil {
                c.moduleEnv.DefineExport(d.Name.Value, d.Name.Value, aliasType, d)
                debugPrintf("// [Checker] Exported type alias: %s\n", d.Name.Value)
            }
        }
    // ... other cases ...
    }
}
```

### Fix 2: Fix Module Loading Order

Ensure import declarations trigger module loading during type checking:

```go
func (c *Checker) checkImportDeclaration(node *parser.ImportDeclaration) {
    // Ensure the source module is loaded and type-checked first
    if c.moduleLoader != nil {
        sourceModule := c.moduleLoader.LoadModule(node.Source.Value, c.moduleEnv.ModulePath)
        if sourceModule == nil {
            c.addError("Module not found: " + node.Source.Value)
            return
        }
    }
    // ... rest of import processing ...
}
```

### Fix 3: Fix Namespace Import Runtime Resolution

The namespace import creates a namespace object but it's null at runtime. Need to ensure:

1. Namespace objects are properly created
2. Module exports are available when namespace is accessed
3. The compiler generates correct bytecode for namespace access

## ROOT CAUSE IDENTIFIED 

**The core issue is that the Paserati parser does not support `export type` syntax.**

### Investigation Results

Through systematic debugging with module loader and checker debug output, we discovered:

1. **Module loading works correctly** - both main.ts and dependency modules are found and processed
2. **The dependency module fails to parse** with error: `Syntax Error at 2:28: no prefix parse function for = found`
3. **The failing line is**: `export type StringOrNumber = string | number;`
4. **Root cause**: The parser doesn't recognize `type` keyword in export statements

### Evidence

```bash
# Debug output showing parse failure:
// [ModuleLoader] loadModuleSequential EARLY RETURN (parse error): ./test_type_alias_export - parsing failed: Syntax Error at 2:28: no prefix parse function for = found
```

The error occurs at character 28 of line 2, which is the `=` in the type alias declaration. The parser expects an expression after `export type` but doesn't understand type alias syntax.

### Why Interfaces Work But Type Aliases Don't

- **Interfaces**: `export interface Foo { ... }` - parser recognizes `interface` keyword
- **Type aliases**: `export type Foo = Bar` - parser doesn't recognize `type` keyword

### Current Status

1. ✅ **Fixed**: Interface exports work correctly with our Pass 1 fix
2. ❌ **Missing**: Parser support for `export type` declarations  
3. ❌ **Missing**: Parser support for standalone `type` declarations
4. ⚠️  **Unresolved**: Runtime namespace import issues (separate from parsing)

## SOLUTION IMPLEMENTED ✅

### Fix 1: Parser Support for Type Aliases - **COMPLETED**

**Root Cause**: The export parsing logic incorrectly consumed the `type` keyword for all cases, breaking `export type TypeAlias = ...` declarations.

**Solution Applied**: Modified `parseExportDeclaration()` in `/Users/nooga/lab/paserati/pkg/parser/parser.go` to distinguish between:
- `export type { ... }` (re-exports) - consume `type` keyword  
- `export type TypeAlias = ...` (type alias declarations) - don't consume `type` keyword

```go
// Before: Always consumed 'type' keyword
if p.curToken.Type == lexer.TYPE {
    isTypeOnly = true
    p.nextToken() // consume 'type' keyword
}

// After: Only consume for re-exports
if p.curToken.Type == lexer.TYPE {
    if p.peekTokenIs(lexer.LBRACE) {
        // This is a re-export: export type { ... }
        isTypeOnly = true
        p.nextToken() // consume 'type' keyword
    }
    // If peek is not '{', this is a type alias, don't consume 'type'
}
```

**Infrastructure Already Present**: 
- ✅ `TYPE` token defined in lexer
- ✅ `parseTypeAliasStatement()` implemented
- ✅ `TypeAliasStatement` AST node defined
- ✅ Type checker integration via Pass 1 fix

### Fix 2: Type Checker Integration - **VERIFIED**

The existing type checker integration works correctly:
- ✅ Type aliases processed in Pass 1 (with our previous interface export fix)
- ✅ Exported type aliases registered in module exports  
- ✅ Cross-module type alias imports work correctly

### Fix 3: Runtime Namespace Imports - **SEPARATE ISSUE**

The namespace import runtime error (`math.add` is null) is unrelated to type alias parsing and remains to be addressed.

## Test Results After Fix

**Module Tests: 19/20 PASSING** ✅
- ✅ `type_alias_export_import` - **FIXED**  
- ✅ `type_reexport` - **FIXED**
- ❌ `cross_module_types` - Runtime namespace import issue (unrelated)

**Scripts Test: STABLE** ✅
- Only expected failure: `class_FIXME_abstract.ts`

## Verification Commands

```bash
# Test the fix
./paserati -e "export type StringOrNumber = string | number"  # ✅ Works
go test ./tests -run TestModules/type_alias_export_import     # ✅ Passes  
go test ./tests -run TestModules/type_reexport                # ✅ Passes
```

## Remaining Work

1. **Runtime Namespace Imports**: Fix `Cannot read property 'add' of null` in cross_module_types
2. **Comprehensive Testing**: Verify no regressions in other module functionality

## Test Verification

After fixes, all these should pass:
```bash
go test ./tests -run TestModules/type_alias_export_import
go test ./tests -run TestModules/type_reexport  
go test ./tests -run TestModules/cross_module_types
```

Success criteria:
- No "unknown type name" errors
- All type imports resolve correctly
- Runtime namespace access works
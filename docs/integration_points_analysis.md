# Builtin System Integration Points Analysis

This document provides a comprehensive analysis of where the old builtin system is integrated throughout the Paserati codebase and what needs to be changed during migration to the new initializer system.

## Summary

| Package | Complexity | Dependencies | Ready for Migration |
|---------|------------|--------------|-------------------|
| **VM** | ✅ **READY** | None! Already modernized | Yes - No changes needed |
| **Driver** | 🟨 **MODERATE** | VM callback registration | Yes - Simple updates |
| **Compiler** | 🟨 **MODERATE** | 2 builtin lookup calls | Yes - Replace lookups |
| **Checker** | 🔴 **COMPLEX** | 6 integration points | **PRIORITY** - Major refactor needed |

## 1. Checker Package Integration Points (CRITICAL)

### Location: `pkg/checker/environment.go`

#### Global Environment Initialization (Lines 54, 70-78)
```go
func NewGlobalEnvironment() *Environment {
    builtins.InitializeRegistry()  // Line 54 - NEEDS REPLACEMENT
    // ...
    
    // Populate with built-in function types
    builtinTypes := builtins.GetAllTypes()  // Line 70 - NEEDS REPLACEMENT
    for name, typ := range builtinTypes {   // Lines 71-78 - NEEDS UPDATE
        if !env.Define(name, typ, true) {
            fmt.Printf("Warning: Failed to define built-in '%s' in global environment (already exists?).\n", name)
        }
    }
}
```

**Change Required**: Replace with new initializer system:
- Remove `builtins.InitializeRegistry()` call
- Replace `builtins.GetAllTypes()` with new TypeContext-based initialization
- Use primitive prototype registry for property type resolution

### Location: `pkg/checker/type_utils.go`

#### Prototype Method Resolver Setup (Line 11)
```go
func init() {
    types.SetPrototypeMethodResolver(builtins.GetPrototypeMethodType)  // NEEDS REPLACEMENT
}
```

#### Individual Type Lookup (Line 16)
```go
func (c *Checker) getBuiltinType(name string) types.Type {
    return builtins.GetType(name)  // NEEDS REPLACEMENT
}
```

**Change Required**: 
- Replace `builtins.GetPrototypeMethodType` with new primitive prototype lookup
- Update `getBuiltinType()` to use global environment instead

### Location: `pkg/checker/expressions.go`

#### Multiple Prototype Method Lookups (8+ calls)

**String prototype methods (Lines 373-376)**
```go
if methodType := builtins.GetPrototypeMethodType("string", propertyName); methodType != nil {
    resultType = methodType
} else {
    c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type 'string'", propertyName))
}
```

**Array prototype methods (Lines 388-392)**
```go
if methodType := builtins.GetPrototypeMethodType("array", propertyName); methodType != nil {
    resultType = methodType
} else {
    c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
}
```

**Function prototype methods (Lines 414-417)**
```go
if methodType := builtins.GetPrototypeMethodType("function", propertyName); methodType != nil {
    resultType = methodType
    debugPrintf("// [Checker MemberExpr] Found function prototype method '%s': %s\n", propertyName, methodType.String())
}
```

**Object prototype methods (Lines 419-422, 430-433)**
```go
if methodType := builtins.GetPrototypeMethodType("object", propertyName); methodType != nil {
    resultType = methodType
    debugPrintf("// [Checker MemberExpr] Found object prototype method '%s': %s\n", propertyName, methodType.String())
}
```

**Optional chaining cases (Lines 589-592, 656-659)**
```go
// String methods in optional chaining
if methodType := builtins.GetPrototypeMethodType("string", propertyName); methodType != nil {
    baseResultType = methodType
} else {
    baseResultType = types.Undefined
}

// Function methods in optional chaining  
if methodType := builtins.GetPrototypeMethodType("function", propertyName); methodType != nil {
    baseResultType = methodType
    debugPrintf("// [Checker OptionalChaining] Found function prototype method '%s': %s\n", propertyName, methodType.String())
}
```

**Change Required**: Replace all `builtins.GetPrototypeMethodType()` calls with new primitive prototype registry lookup.

### Location: `pkg/checker/checker.go`

#### Checker Creation (Line 47)
```go
func NewChecker() *Checker {
    return &Checker{
        env:    NewGlobalEnvironment(),   // This transitively calls all the above functions
        // ...
    }
}
```

**Change Required**: Ensure `NewGlobalEnvironment()` uses new initializer system.

## 2. Compiler Package Integration Points (MODERATE)

### Location: `pkg/compiler/compiler.go`

#### Import Statement (Line 6)
```go
import (
    "paserati/pkg/builtins" // Will need to remain for new system
    // ...
)
```

#### Builtin Function Lookup (Lines 484-494)
```go
if builtinFunc := builtins.GetFunc(node.Value); builtinFunc != nil {
    // It's a built-in function.
    debugPrintf("// DEBUG Identifier '%s': Resolved as Builtin\n", node.Value)
    builtinValue := vm.NewNativeFunction(builtinFunc.Arity, builtinFunc.Variadic, builtinFunc.Name, builtinFunc.Fn)
    constIdx := c.chunk.AddConstant(builtinValue) // Add vm.Value to constant pool
    
    // Allocate register and load the constant
    c.emitLoadConstant(hint, constIdx, node.Token.Line) // Use existing emitter
    
    return hint, nil // Built-in handled successfully
}
```

#### Builtin Object Lookup (Lines 497-506)
```go
if builtinObj := builtins.GetObject(node.Value); !builtinObj.Is(vm.Undefined) {
    // It's a built-in object (like console).
    debugPrintf("// DEBUG Identifier '%s': Resolved as Builtin Object\n", node.Value)
    constIdx := c.chunk.AddConstant(builtinObj) // Add the object to constant pool
    
    // Allocate register and load the constant
    c.emitLoadConstant(hint, constIdx, node.Token.Line) // Use existing emitter
    
    return hint, nil // Built-in object handled successfully
}
```

#### Commented Builtin Global Definition (Line 166)
```go
// c.defineBuiltinGlobals() // TODO: Define built-ins if any
```

**Change Required**: 
- Replace `builtins.GetFunc()` and `builtins.GetObject()` with new registry access
- Consider implementing `defineBuiltinGlobals()` if global variable approach is preferred
- Update to work with new builtin constructor/object model

## 3. Driver Package Integration Points (SIMPLE)

### Location: `pkg/driver/driver.go`

#### VM Callback Registration (Line 34, 177)
```go
vmInstance.AddStandardCallbacks(builtins.GetStandardInitCallbacks())
```

**Change Required**: Replace `builtins.GetStandardInitCallbacks()` with new initializer system callback.

### Location: `cmd/paserati/main.go`

#### Entry Points
- REPL Mode (line 104): `paserati := driver.NewPaserati()`
- Expression Mode (line 60): `paserati := driver.NewPaserati()`
- File Mode: Uses `driver.RunStringWithOptions()`

**Change Required**: No direct changes needed - will work through driver updates.

## 4. VM Package Integration Points (ALREADY READY!)

### Status: ✅ **NO MIGRATION NEEDED**

The VM package has already been modernized and has:
- VM instance-owned prototypes
- Callback-based initialization system  
- Modern value constructors
- No dependencies on old builtin system

### Current Architecture
- **File**: `pkg/vm/vm_init.go` 
- **Modern prototype creation**: Lines 163-184 in `initializePrototypes()`
- **Callback system**: Lines 11-49 for `VMInitCallback` registration
- **Instance prototypes**: Lines 72-79 in `vm.go` 

## Migration Strategy Recommendations

### Phase 1: Foundation (✅ COMPLETED)
- Core interface types
- Object and Function initializers
- Testing infrastructure

### Phase 2: Checker Migration (🔴 CRITICAL NEXT STEP)
1. **Update Global Environment**: Replace old registry with new initializer system
2. **Primitive Prototype Registry**: Store prototype types for property resolution
3. **Update Property Access**: Replace `builtins.GetPrototypeMethodType()` calls
4. **Update Type Resolution**: Replace `builtins.GetType()` calls

### Phase 3: Compiler Migration
1. **Replace Builtin Lookups**: Update identifier resolution
2. **Test Constant Generation**: Ensure builtin objects compile correctly

### Phase 4: Driver Migration  
1. **Update Callback System**: Replace old callbacks with new initializers
2. **Test Integration**: Ensure REPL and file execution work

### Phase 5: Cleanup
1. **Remove Old Registry**: Delete old builtin registration system
2. **Clean Up Imports**: Remove unused old builtin references

## Risk Assessment

### High Risk
- **Checker Property Access**: Complex logic with many integration points
- **Type System Coupling**: Deep integration between types package and builtins

### Medium Risk  
- **Global Environment Setup**: Critical for type checking functionality
- **Compiler Identifier Resolution**: Affects runtime behavior

### Low Risk
- **Driver Callbacks**: Simple function replacement
- **VM Integration**: Already modernized

## Testing Strategy

1. **Unit Tests**: Each component migration should have comprehensive tests
2. **Integration Tests**: Test checker + compiler + VM together
3. **Regression Tests**: Ensure existing TypeScript examples still work
4. **REPL Testing**: Manual testing of interactive functionality

## Files That Need Changes

### Critical (Must Change)
- `pkg/checker/environment.go` - Global environment initialization
- `pkg/checker/type_utils.go` - Type resolution and prototype method resolver
- `pkg/checker/expressions.go` - Property access type checking (8+ locations)

### Moderate (Should Change)
- `pkg/compiler/compiler.go` - Builtin identifier resolution (2 locations)
- `pkg/driver/driver.go` - VM callback registration (2 locations)

### Optional (Nice to Have)
- `pkg/compiler/compiler.go` - Implement `defineBuiltinGlobals()` function

### No Changes Needed
- `pkg/vm/` - Already modernized and ready
- `cmd/paserati/main.go` - Works through driver layer
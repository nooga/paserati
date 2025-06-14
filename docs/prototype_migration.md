# Prototype System Migration Plan

## Overview

This document outlines the plan to migrate from the current dual prototype system (global and VM-specific) to a single, VM-specific prototype system. The goal is to improve consistency, thread safety, and maintainability while preserving functionality.

## Current State

### Global Prototypes (`prototypes.go`)

- Package-level variables: `StringPrototype`, `ArrayPrototype`, `FunctionPrototype`
- Used for method registration via `Register*PrototypeMethod` functions
- Lazy initialization via `initPrototypes()`
- Used in property lookup for primitive types
- Used in type checking and method registration

### VM-specific Prototypes (`vm.go`)

- Instance-specific prototypes owned by each VM
- Initialized during VM creation
- Used in runtime property lookup and method calls
- Used in prototype chain traversal
- Used in object creation and inheritance

## Migration Steps

### Phase 1: Preparation (No Breaking Changes)

1. **Add VM Prototype Access Methods**

   ```go
   // Add to vm.go
   func (vm *VM) GetStringPrototype() *PlainObject {
       return vm.StringPrototype.AsPlainObject()
   }

   func (vm *VM) GetArrayPrototype() *PlainObject {
       return vm.ArrayPrototype.AsPlainObject()
   }

   func (vm *VM) GetFunctionPrototype() *PlainObject {
       return vm.FunctionPrototype.AsPlainObject()
   }
   ```

2. **Add Deprecation Warnings**

   ```go
   // Add to prototypes.go
   func RegisterStringPrototypeMethod(methodName string, method Value) {
       fmt.Fprintf(os.Stderr, "Warning: RegisterStringPrototypeMethod is deprecated. Use VM.GetStringPrototype() instead.\n")
       initPrototypes()
       StringPrototype.SetOwn(methodName, method)
   }
   ```

3. **Update Documentation**
   - Add deprecation notices to all global prototype functions
   - Document new VM-specific methods
   - Update examples to use VM-specific prototypes

### Phase 2: Type System Migration

1. **Create Type System Interface**

   ```go
   // Add to types/prototype.go
   type PrototypeProvider interface {
       GetStringPrototype() *PlainObject
       GetArrayPrototype() *PlainObject
       GetFunctionPrototype() *PlainObject
   }
   ```

2. **Update Type Checker**

   - Modify type checker to accept a `PrototypeProvider`
   - Update type resolution to use VM-specific prototypes
   - Keep backward compatibility with global prototypes

3. **Update Built-in Registration**
   - Modify built-in registration to use VM-specific prototypes
   - Keep global registration as fallback
   - Add tests to verify both systems work

### Phase 3: Runtime System Migration

1. **Update Property Lookup**

   ```go
   // Modify property_helpers.go
   func (vm *VM) handlePrimitiveMethod(objVal Value, propName string) (Value, bool) {
       switch objVal.Type() {
       case TypeString:
           if method, exists := vm.GetStringPrototype().GetOwn(propName); exists {
               return createBoundMethod(objVal, method), true
           }
       // ... similar for other types
       }
       return Undefined, false
   }
   ```

2. **Update Object Creation**

   - Ensure all object creation uses VM-specific prototypes
   - Update factory functions to accept VM instance
   - Add tests for object creation with different prototypes

3. **Update Method Registration**
   - Create new registration system using VM instance
   - Update all built-in methods to use new system
   - Keep old system as fallback

### Phase 4: Cleanup

1. **Remove Global Prototypes**

   - Remove global prototype variables
   - Remove global registration functions
   - Remove deprecated code

2. **Update Tests**

   - Update all tests to use VM-specific prototypes
   - Add tests for prototype isolation between VMs
   - Add tests for thread safety

3. **Final Documentation**
   - Update all documentation to reflect new system
   - Add migration guide for users
   - Document thread safety guarantees

## Implementation Guidelines

### For Each Change:

1. Create a new branch
2. Write tests first
3. Implement changes
4. Run full test suite
5. Update documentation
6. Create pull request

### Testing Strategy:

1. Unit tests for each component
2. Integration tests for prototype chain
3. Performance tests for property lookup
4. Thread safety tests
5. Backward compatibility tests

### Performance Considerations:

1. Monitor property lookup performance
2. Check memory usage
3. Verify thread safety
4. Measure initialization time

## Rollback Plan

If issues are discovered:

1. Keep both systems functional
2. Add feature flags to control which system is used
3. Maintain backward compatibility
4. Document known issues

## Success Criteria

1. All tests pass
2. No performance regression
3. Thread safety verified
4. Documentation updated
5. No breaking changes for users
6. Clean codebase without deprecated code

## Timeline

1. Phase 1: 1 week
2. Phase 2: 2 weeks
3. Phase 3: 2 weeks
4. Phase 4: 1 week

Total: 6 weeks

## Future Considerations

1. Prototype chain optimization
2. Method caching improvements
3. Thread-safe property access
4. Better type system integration
5. Performance monitoring tools

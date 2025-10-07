# Proper Fix for Function Constructor

## The Real Problem

When `Function` constructor calls `RunString()` on the parent Paserati instance:

1. `RunString` → `runAsModule()` → `EnableModuleMode(moduleName, moduleLoader)`
2. `EnableModuleMode` **creates NEW ModuleBindings**, overwriting the compiler's current module state
3. This corrupts the parent compilation context that was in progress

## Real JavaScript Engine Behavior

In V8, SpiderMonkey, JavaScriptCore:

1. Function constructor compiles code in the **same realm/global scope**
2. Dynamically created functions have access to **all globals** from that realm
3. Creating a function **does not affect** any ongoing compilation or execution
4. The function is compiled **independently** but shares the same global environment

## Proper Solution

The Function constructor needs its own compilation path that:
- ✅ Shares the same heap (global variables)
- ✅ Shares the same builtins
- ❌ Does NOT modify the parent compiler's state
- ❌ Does NOT call EnableModuleMode on the shared compiler
- ✅ Compiles in a way that allows access to globals

### Implementation Plan

**Option A: Use CompileProgram directly (PREFERRED)**

Instead of calling `RunString` which goes through `runAsModule` and changes compiler state, call `CompileProgram` directly:

```go
func functionConstructorImpl(vmInstance *vm.VM, driver interface{}, args []vm.Value) (vm.Value, error) {
	// ... build source code ...

	// Parse the code
	lx := lexer.NewLexer(source)
	p := parser.NewParser(lx)
	prog, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return vm.Undefined, fmt.Errorf("SyntaxError: %v", parseErrs[0])
	}

	// Get driver and compile WITHOUT calling EnableModuleMode
	type driverInterface interface {
		CompileProgram(*parser.Program) (*vm.Chunk, []error)
		GetVM() *vm.VM
	}

	d, ok := driver.(driverInterface)
	if !ok {
		return vm.Undefined, fmt.Errorf("SyntaxError: Function constructor requires compiler access")
	}

	// Compile the program (uses existing compiler state, doesn't modify it)
	chunk, compileErrs := d.CompileProgram(prog)
	if len(compileErrs) > 0 {
		return vm.Undefined, fmt.Errorf("SyntaxError: %v", compileErrs[0])
	}

	// Execute the chunk to get the function value
	vm := d.GetVM()
	result, runtimeErrs := vm.Interpret(chunk)
	if len(runtimeErrs) > 0 {
		return vm.Undefined, fmt.Errorf("Error: %v", runtimeErrs[0])
	}

	return result, nil
}
```

This approach:
- Uses `CompileProgram` which just calls `compiler.Compile()` without state changes
- Doesn't call `EnableModuleMode` or modify compiler state
- Still uses the same heap, so has access to all globals
- Executes the chunk in the same VM

**Option B: Save and restore compiler state**

```go
// Save state
savedModuleBindings := compiler.moduleBindings
savedModuleLoader := compiler.moduleLoader

// Call RunString
result, errs := d.RunString(source)

// Restore state
compiler.moduleBindings = savedModuleBindings
compiler.moduleLoader = savedModuleLoader
```

This is hacky and error-prone. Option A is better.

**Option C: Create an isolated Paserati instance**

Create a new Paserati instance that shares the heap with the parent:

```go
// Create new instance
isolated := driver.NewPaserati()

// Share the heap somehow???
```

This doesn't work easily because heaps aren't designed to be shared.

## Recommended Implementation: Option A

Modify `function_init.go` to use `CompileProgram` + `Interpret` instead of `RunString`.

### Code Changes

**pkg/builtins/function_init.go**:
```go
func functionConstructorImpl(vmInstance *vm.VM, driver interface{}, args []vm.Value) (vm.Value, error) {
	// ... [parameter and body handling stays the same] ...

	// Construct source: return (function(...params) { body });
	var source string
	// ... [source construction stays the same] ...

	// We need direct access to compiler and VM
	type driverInterface interface {
		CompileProgram(*parser.Program) (*vm.Chunk, []errors.PaseratiError)
		GetVM() *vm.VM
	}

	d, ok := driver.(driverInterface)
	if !ok {
		return vm.Undefined, fmt.Errorf("SyntaxError: Function constructor requires compiler access")
	}

	// Parse the source
	lx := lexer.NewLexer(source)
	p := parser.NewParser(lx)
	prog, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return vm.Undefined, fmt.Errorf("SyntaxError: %v", parseErrs[0])
	}

	// Compile without modifying compiler state
	chunk, compileErrs := d.CompileProgram(prog)
	if len(compileErrs) > 0 {
		return vm.Undefined, fmt.Errorf("SyntaxError: %v", compileErrs[0])
	}

	// Execute in the same VM to get the function
	result, runtimeErrs := vmInstance.Interpret(chunk)
	if len(runtimeErrs) > 0 {
		return vm.Undefined, fmt.Errorf("Error: %v", runtimeErrs[0])
	}

	return result, nil
}
```

**pkg/driver/driver.go** - Add GetVM accessor (already exists):
```go
func (p *Paserati) GetVM() *vm.VM {
	return p.vmInstance
}
```

This fix ensures:
1. ✅ Function constructor works correctly
2. ✅ Doesn't corrupt parent compilation state
3. ✅ Dynamically created functions have access to all globals
4. ✅ No performance overhead
5. ✅ Clean, maintainable code

## Testing

After implementing, verify:

```javascript
console.log("1. Array:", typeof Array);
const fn = new Function("return typeof Array");
console.log("2. fn():", fn());
console.log("3. Array after:", typeof Array);
```

Expected: All should print "function"

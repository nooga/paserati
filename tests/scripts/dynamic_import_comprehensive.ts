// expect: true
// Comprehensive dynamic import test

// Test 1: Import and access exports
let mod = import("./dynamic_import_helper.ts");
let test1 = mod.value === 42;

// Test 2: Call exported function
let test2 = mod.greet("World") === "Hello, World";

// Test 3: Module caching - importing same module twice returns same exports
let mod2 = import("./dynamic_import_helper.ts");
let test3 = mod2.value === 42;

// All tests pass
test1 && test2 && test3;

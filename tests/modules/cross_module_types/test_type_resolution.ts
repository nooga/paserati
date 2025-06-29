// Test that demonstrates cross-module type resolution

// Step 1: Module loader creates module records for both files
// - math.ts is parsed and type checked first (no dependencies)
// - main.ts is parsed and type checked after (depends on math.ts)

// Step 2: When type checking math.ts:
// - Checker extracts exported types: Vector2D interface, function signatures
// - Module record stores: Exports = { "Vector2D": InterfaceType, "add": FunctionType, ... }

// Step 3: When type checking main.ts:
// - Import statement creates bindings in ModuleEnvironment
// - When checking "let v1: Vector2D = ...", ResolveImportedType is called
// - It looks up math.ts module record and finds Vector2D type
// - Type checking proceeds with actual Vector2D type, not types.Any

// Expected behavior:
// 1. Vector2D resolves to the actual interface type from math.ts
// 2. add() function has proper signature checking
// 3. Type errors are caught (e.g., passing string instead of Vector2D)
// 4. Namespace imports (math.*) work with proper types

// This test file itself doesn't run - it documents the expected flow
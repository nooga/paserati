# Paserati Feature Bucket List

This document tracks implemented and planned features for the Paserati TypeScript/JavaScript runtime, based on ES2025 and TypeScript specifications.

**Test262 Language Suite: 91.9%** (21,514/23,410 tests passing)

## Test262 Weak Spots

Areas with lower pass rates that need attention:

| Subsuite | Pass Rate | Tests |
|----------|-----------|-------|
| `import/import-defer` | 14.5% | 69 |
| `module-code/namespace` | 52.8% | 36 |
| `statements/with` | 56.4% | 181 |
| `eval-code/indirect` | 63.9% | 61 |
| `statements/for-in` | 72.2% | 115 |
| `expressions/yield` | 74.6% | 63 |
| `expressions/super` | 75.5% | 94 |
| `types/reference` | 79.3% | 29 |
| `expressions/call` | 79.3% | 92 |
| `eval-code/direct` | 79.7% | 286 |

## Core Syntax & Basics

- [x] Variable declarations (`let`, `const`, `var`)
- [x] Semicolons (optional)
- [x] Comments (`//`, `/* */`)
- [x] Block scoping (`{}`)
- [x] Control flow without braces (single statement bodies)
- [x] Global variables (OpGetGlobal/OpSetGlobal)
- [x] Module system (`import`/`export`) - all patterns, runtime execution, cross-module type checking
- [x] `var` keyword with proper hoisting and function scope

## Literals

- [x] String literals (single/double quotes)
- [x] Number literals (decimal, hex, binary, octal, separators)
- [x] Boolean literals (`true`, `false`)
- [x] `null` and `undefined` literals
- [x] Array literals (`[]`)
- [x] Object literals (`{}`)
- [x] Regular expression literals (`/abc/`) - full RegExp support with flags
- [x] Template literals (backticks, `${}`)
- [x] BigInt literals (`100n`) - with constructor and arithmetic operations

## Operators

### Arithmetic
- [x] `+`, `-`, `*`, `/`, `%`, `**`
- [x] `++`, `--` (prefix/postfix)
- [x] Unary `-`, `+`

### Comparison
- [x] `==`, `!=`, `===`, `!==`
- [x] `>`, `<`, `>=`, `<=`

### Logical
- [x] `&&`, `||`, `!`

### Bitwise
- [x] `&`, `|`, `^`, `~`
- [x] `<<`, `>>`, `>>>`

### Assignment
- [x] `=` and all compound assignments (`+=`, `-=`, etc.)
- [x] `&&=`, `||=`, `??=`

### Other Operators
- [x] Ternary (`? :`)
- [x] Comma (in `for` loops, array literals)
- [x] `typeof`, `instanceof`, `in`, `delete`, `void`
- [x] Nullish coalescing (`??`)
- [x] Optional chaining (`?.`, `?.[]`, `?.()`)
- [x] Type assertions (`as`)
- [x] Satisfies operator (`satisfies`)
- [x] Spread syntax (`...`) - in calls, arrays, objects
- [x] `yield`, `yield*`
- [x] `await`
- [x] Symbols (`Symbol.iterator`, `Symbol.for`, etc.)
- [x] Destructuring assignment (arrays, objects, nested, defaults, rest)

## Control Flow

- [x] `if`/`else if`/`else`
- [x] `switch`/`case`/`default`
- [x] `while`, `do...while`
- [x] `for`, `for...in`, `for...of`, `for await...of`
- [x] `break`, `continue`
- [x] Labeled statements
- [x] `try`/`catch`/`finally` with error stack traces
- [x] `throw`

## Functions

- [x] Function declarations and expressions
- [x] Arrow functions
- [x] Default and optional parameters
- [x] Rest parameters (`...`)
- [x] `arguments` object
- [x] Closures / lexical scoping
- [x] `this` keyword with proper context
- [x] `new` operator / constructor functions
- [x] Prototypal inheritance
- [x] `Function.prototype.call()`, `.apply()`, `.bind()`
- [x] Generator functions (`function*`)
- [x] Async functions (`async function`)
- [x] Async generators (`async function*`)

## Data Structures & Built-ins

- [x] **Array** - all common methods (map, filter, reduce, sort, etc.)
- [x] **Object** - static methods (keys, values, entries, assign, fromEntries, hasOwn)
- [x] **String** - 22+ methods including regex integration
- [x] **Number** - prototype and static methods, formatting
- [x] **Math** - 30+ methods
- [x] **Date** - full implementation with getters, setters, locale methods
- [x] **JSON** - parse and stringify
- [x] **Map / Set** - with iteration
- [x] **TypedArrays & ArrayBuffer** - all types
- [x] **Promise** - constructor, static methods, microtask scheduling
- [x] **Proxy & Reflect** - all 13 handler traps
- [x] **Symbol** - well-known symbols, registry
- [x] **BigInt** - arithmetic operations
- [x] **RegExp** - literals, constructor, methods
- [x] **console** - log, error, warn, time, group, etc.
- [x] **performance** - now, mark, measure
- [x] **eval()** - direct and indirect
- [x] **Dynamic import()** - with pluggable resolution
- [ ] **WeakMap / WeakSet** - planned
- [ ] **Timer functions** (`setTimeout`, `setInterval`) - planned

## TypeScript Types

### Basic Types
- [x] `number`, `string`, `boolean`, `null`, `undefined`
- [x] `any`, `void`, `unknown`, `never`
- [x] Array types (`T[]`), tuple types
- [x] Enum types (numeric, string, const)
- [x] Literal types
- [x] Union and intersection types
- [x] Function types
- [x] Object type literals
- [x] Callable types
- [x] Interfaces with inheritance
- [x] Index signatures
- [x] Type aliases
- [x] Constructor types

### Advanced Types
- [x] Generics - functions, classes, constraints, inference
- [x] Conditional types (`T extends U ? X : Y`)
- [x] Mapped types (`{ [P in K]: T }`)
- [x] Utility types (Partial, Required, Readonly, Pick, Record, Omit, Extract, Exclude, NonNullable, ReturnType, Parameters)
- [x] `keyof` operator
- [x] Indexed access types (`T[K]`)
- [x] Type predicates (`x is T`)
- [x] Template literal types
- [x] Type-level `typeof`
- [x] `infer` keyword

### Type Checking
- [x] Assignability checks
- [x] Operator type checking
- [x] Function call checks
- [x] Structural typing
- [x] Type narrowing with `typeof`, `instanceof`, literals
- [x] Control flow analysis

## Classes

- [x] Class declarations and expressions
- [x] Constructors with overloads
- [x] Properties (with initializers, optional)
- [x] Methods
- [x] Inheritance (`extends`) with `super`
- [x] Access modifiers (`public`, `private`, `protected`)
- [x] Static members
- [x] Abstract classes/methods
- [x] `implements` clause
- [x] Generic classes (including recursive)
- [x] Getters/setters
- [x] `override` keyword
- [x] `readonly` properties
- [x] Property parameter shortcuts (`constructor(public name: string)`)
- [x] Private fields (`#private`)
- [ ] Decorators - planned

## Not Implemented

- [ ] Decorators (`@decorator`)
- [ ] Namespaces (`namespace N {}`)
- [ ] Declaration files (`.d.ts`)
- [ ] Triple-slash directives
- [ ] Path mapping
- [ ] Project references
- [ ] WeakMap/WeakSet
- [ ] Timer functions
- [ ] Strict null checks option
- [ ] Sparse arrays (large index optimization)

## VM Optimizations (Future)

- [ ] **Dynamic Stack Expansion** - Currently uses fixed 1024-frame call stack (~6MB). Could use dynamic expansion:
  - Start with smaller allocation (e.g., 64 frames) to save memory
  - Grow in chunks when needed
  - **Challenge**: Upvalues store raw pointers into register stack. Options:
    1. Change upvalues from pointers to (frame_index, register_index) pairs
    2. Use chunked allocator that doesn't move memory
    3. Close all open upvalues before resizing
  - **Note**: Crypto benchmark hits this limit with deep BigInteger recursion (~2000+ frames)

- [ ] **Tail Call Optimization** - Already implemented, could extend to more patterns

- [ ] **Inline Caching Improvements** - Current IC validates property names; could add polymorphic caching

- [x] **Smart Pinning** - Only pin registers when captured by closures, not at declaration time
  - Implemented in `emitClosure`/`emitClosureGeneric`
  - Reduces unnecessary register pinning for non-captured variables

- [ ] **Register Spilling** - Compiler panics on "ran out of registers" for very large functions
  - RegExp benchmark needs ~298 simultaneous registers (exceeds 255 limit)
  - Smart pinning doesn't help since all variables are live at the same time
  - Need `OpLoadLocal`/`OpStoreLocal` opcodes for spilling to heap

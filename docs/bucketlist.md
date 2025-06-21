# Paserati Feature Bucket List

This list tracks the implemented and planned features for the Paserati TypeScript/JavaScript compiler, based on common language features.

**Recent Major Updates (hoisting branch):**

- **Complete Destructuring Implementation** - Full support for array/object destructuring with rest elements, defaults, and nested patterns
- **Built-in System Refactor** - Modernized builtin architecture and cleaned up legacy code
- **🚀 GENERICS IMPLEMENTATION** - **COMPLETE!** Full generic types, functions, and type inference
- **Function.prototype.bind()** - Work in progress (failing tests indicate incomplete implementation)

## Core Syntax & Basics

- [x] Variable Declarations (`let`, `const`)
- [x] Semicolons (optional)
- [x] Comments (`//`, `/* */`)
- [x] Block Scoping (`{}`)
- [x] Control Flow without braces (single statement bodies)
- [x] Global Variables (implemented with OpGetGlobal/OpSetGlobal)
- [x] **Enhanced Parser Robustness** - improved function declaration parsing and error recovery
- [ ] Module System (`import`/`export`)
- [ ] `var` keyword (legacy)

## Literals

- [x] String Literals (single/double quotes)
- [x] Number Literals (decimal, hex, binary, octal, separators)
- [x] Boolean Literals (`true`, `false`)
- [x] `null` Literal
- [x] `undefined` Literal (as value and uninitialized state)
- [x] Array Literals (`[]`)
- [x] Object Literals (`{}`)
- [ ] Regular Expression Literals (`/abc/`)
- [x] Template Literals (backticks, `${}`)
- [ ] BigInt Literals (`100n`)

## Operators

### Arithmetic

- [x] Addition (`+`) (incl. string concat)
- [x] Subtraction (`-`)
- [x] Multiplication (`*`)
- [x] Division (`/`)
- [x] Remainder (`%`)
- [x] Exponentiation (`**`)
- [x] Increment (`++`) (prefix/postfix)
- [x] Decrement (`--`) (prefix/postfix)
- [x] Unary Negation (`-`)
- [x] Unary Plus (`+`) (type coercion)

### Comparison

- [x] Equal (`==`)
- [x] Not Equal (`!=`)
- [x] Strict Equal (`===`)
- [x] Strict Not Equal (`!==`)
- [x] Greater Than (`>`)
- [x] Less Than (`<`)
- [x] Greater Than or Equal (`>=`)
- [x] Less Than or Equal (`<=`)

### Logical

- [x] Logical AND (`&&`)
- [x] Logical OR (`||`)
- [x] Logical NOT (`!`)

### Bitwise

- [x] Bitwise AND (`&`)
- [x] Bitwise OR (`|`) (Note: Lexer uses `|` for Type Union)
- [x] Bitwise XOR (`^`)
- [x] Bitwise NOT (`~`)
- [x] Left Shift (`<<`)
- [x] Right Shift (`>>`)
- [x] Unsigned Right Shift (`>>>`)

### Assignment

- [x] Assignment (`=`)
- [x] Addition assignment (`+=`)
- [x] Subtraction assignment (`-=`)
- [x] Multiplication assignment (`*=`)
- [x] Division assignment (`/=`)
- [x] Remainder assignment (`%=`)
- [x] Exponentiation assignment (`**=`)
- [x] Left shift assignment (`<<=`)
- [x] Right shift assignment (`>>=`)
- [x] Unsigned right shift assignment (`>>>=`)
- [x] Bitwise AND assignment (`&=`)
- [x] Bitwise XOR assignment (`^=`)
- [x] Bitwise OR assignment (`|=`)
- [x] Logical AND assignment (`&&=`)
- [x] Logical OR assignment (`||=`)
- [x] Nullish coalescing assignment (`??=`)

### Misc

- [x] Conditional (Ternary) Operator (`? :`)
- [x] Comma Operator (in specific contexts like `for` loops, array literals)
- [x] `typeof` Operator
- [x] `instanceof` Operator - **New!**
  - [x] Basic instanceof checks (`obj instanceof Constructor`)
  - [x] Constructor function validation (callable types with construct signatures)
  - [x] TypeScript-compliant error handling for invalid constructors
  - [x] Integration with prototypal inheritance system
- [x] `in` Operator - **New!**
  - [x] Basic property existence checking (`"prop" in obj`)
  - [x] Support for string and number keys
  - [x] Array index checking (`"0" in arr`, `"length" in arr`)
  - [x] Type checking with compile-time validation
  - [x] Works with PlainObject, DictObject, and ArrayObject
  - [x] TypeScript-compliant error messages for invalid operands
- [x] `delete` Operator - **Complete Implementation!**
  - [x] Basic syntax, parsing, and type checking
  - [x] Bytecode compilation and VM execution
  - [x] Returns boolean success value correctly
  - [x] PlainObject to DictObject conversion logic implemented
  - [x] **Ersatz Solution**: Object reference semantics work correctly via dual-update approach
  - [x] All variable references to the same object see the deletion
  - [x] Comprehensive test coverage for edge cases
- [x] `void` Operator
- [x] Grouping Operator (`()`)
- [x] Nullish Coalescing Operator (`??`)
- [x] Optional Chaining (`?.`)
- [x] Type Assertions (`as` operator) - **New!**
  - [x] Basic type assertions (`value as Type`)
  - [x] Compile-time validation with TypeScript-compliant error checking
  - [x] Runtime behavior (assertions are no-ops after type checking)
  - [x] Support for primitive types, interfaces, and complex types
  - [x] Integration with union types and contextual typing
- [x] Spread Syntax (`...`) - **Major Enhancement!**
  - [x] Spread in function calls (`func(...args)`)
  - [x] Contextual typing for spread array literals (`sum(...[1, 2, 3])`)
  - [x] TypeScript-compliant error handling for non-tuple spreads
  - [x] Integration with tuple types and parameter type inference
- [ ] `yield` / `yield*` (Generators)
- [ ] `await` (Async/Await)
- [x] Destructuring Assignment - **Complete Implementation!**
  - [x] **Array Destructuring** - Full support with rest elements
    - [x] Basic array destructuring (`let [a, b] = [1, 2]`)
    - [x] Rest elements in arrays (`let [first, ...rest] = array`)
    - [x] Nested array destructuring (`let [a, [b, c]] = [1, [2, 3]]`)
    - [x] Default values in array destructuring (`let [a = 10, b = 20] = []`)
    - [x] Mixed patterns with defaults and rest elements
  - [x] **Object Destructuring** - Full support with rest elements
    - [x] Basic object destructuring (`let {name, age} = person`)
    - [x] Object rest elements (`let {name, ...rest} = person`)
    - [x] Property exclusion in rest elements (proper key filtering)
    - [x] Nested object destructuring (`let {person: {name}} = data`)
    - [x] Default values in object destructuring (`let {name = "Unknown"} = {}`)
    - [x] Mixed patterns with defaults and rest elements
  - [x] **Declaration Context Support**
    - [x] `const` declarations with destructuring
    - [x] `let` declarations with destructuring
    - [x] Variable assignment destructuring (non-declaration)
  - [x] **Advanced Features**
    - [x] Complex nested patterns (`let {a: [b, {c}]} = complex`)
    - [x] Function parameter destructuring (both object and array)
    - [x] Default values in function parameter destructuring
    - [x] Rest elements in function parameter destructuring
    - [x] TypeScript-compliant type checking for all destructuring patterns
    - [x] Comprehensive error handling and validation
  - [x] **46+ test cases** covering all destructuring scenarios and edge cases

## Control Flow

- [x] `if`/`else if`/`else` Statements/Expressions
- [x] `switch`/`case`/`default` Statements (with fallthrough and break)
- [x] `while` Loops
- [x] `do...while` Loops
- [x] `for` Loops (C-style)
- [x] `for...in` Loops - **Enhanced!**
  - [x] Basic for...in iteration over object properties
  - [x] Support for existing variable assignment (not just declaration)
  - [x] Proper global variable handling in loop assignment
- [x] `for...of` Loops - **Enhanced!**
  - [x] Basic for...of iteration over arrays
  - [x] Support for existing variable assignment (not just declaration)
  - [x] Proper global variable handling in loop assignment
- [x] `break` Statement
- [x] `continue` Statement
- [ ] Labeled Statements
- [x] `try`/`catch`/`finally` Blocks - **Complete Implementation!**
  - [x] Basic try/catch with exception handling
  - [x] Error object constructor and proper prototype chain
  - [x] Finally blocks with proper control flow
  - [x] **Advanced return statements in finally blocks** - OpReturnFinally mechanism
  - [x] **Error stack traces** - Complete call stack capture with function names and line numbers
  - [x] **Custom error types** - TypeError, ReferenceError, SyntaxError with proper inheritance
  - [x] Exception table approach with minimal bytecode changes
  - [x] Comprehensive test coverage for all exception scenarios
- [x] `throw` Statement - **Complete!**

## Functions

- [x] Function Declarations (`function name() {}`) **[Enhanced hoisting - fixed parser ambiguity]**
- [x] Function Expressions (`let x = function() {}`)
- [x] Arrow Functions (`=>`)
  - [x] Single/Multi Parameters
  - [x] Parenthesized/Unparenthesized Single Parameter
  - [x] Expression Body
  - [x] Block Body
- [x] Return Statements (`return`, implicit `undefined`)
- [x] Parameters (incl. basic type annotations)
- [x] Higher-order functions (function parameters and returns)
- [x] Curried functions
- [x] Function Overloads
  - [x] Basic overload declarations (`function f(x: string): string; function f(x: number): number;`)
  - [x] Implementation signature matching
  - [x] Integration with default/optional parameters
    - [ ] TypeScript compliant type-checking for default/optional parameters
- [x] Default Parameter Values
  - [x] Basic default values (`function f(x = 5)`)
  - [x] Multiple default parameters
  - [x] Mixed required and default parameters
  - [x] Parameter references in defaults (`function f(a, b = a + 1)`)
  - [x] Complex expressions in defaults
  - [x] Arrow functions with defaults
  - [x] Shorthand methods with defaults (`{ method(x = 5) {} }`)
  - [x] Type checking for default value assignability
  - [x] Forward reference prevention (proper error for `function f(a = b, b)`)
  - [x] Type inference from default values (`function f(x = 20)` infers `x: number`)
- [x] Optional Parameters
  - [x] Basic optional parameters (`function f(a: number, b?: string)`)
  - [x] Multiple optional parameters
  - [x] Mixed required and optional parameters
  - [x] Type checking for optional parameter usage
  - [x] Proper arity checking (minimum required arguments)
  - [x] Arrow functions with optional parameters
  - [x] Shorthand methods with optional parameters (`{ method(x?: string) {} }`)
- [x] Rest Parameters (`...`) (basic implementation, some edge cases remain)
- [ ] `arguments` Object
- [x] Closures / Lexical Scoping
- [x] `this` Keyword (comprehensive object method context)
  - [x] Basic object method context
  - [x] Explicit `this` parameter syntax (`function(this: SomeType)`) **[Enhanced error handling]**
  - [x] Context preservation in nested function literals
  - [x] Constructor function `this` binding with `new` operator
  - [x] **Robust `this` parameter validation** - proper error messages for missing type annotations
- [x] `new` Operator / Constructor Functions - **Enhanced!**
  - [x] OpNew bytecode implementation
  - [x] Constructor function prototype property creation
  - [x] Instance prototype chain establishment
  - [x] TypeScript-compliant constructor type checking
- [x] Prototypal Inheritance - **New Major Feature!**
  - [x] Function prototype property support (`.prototype`)
  - [x] Constructor property relationships
  - [x] Function.prototype methods
    - [x] `Function.prototype.call()` for explicit `this` binding **[Fixed infinite recursion]**
    - [x] `Function.prototype.apply()` for explicit `this` binding with array arguments
    - [ ] `Function.prototype.bind()` for creating bound functions **[Work in Progress]**
  - [x] Object.getPrototypeOf() static method
  - [x] Prototype chain traversal and method resolution
  - [x] Runtime prototype object management
  - [x] TypeScript-compliant prototype type checking
  - [x] Integration with instanceof operator
  - [x] **Robust method binding system** - prevents infinite recursion in built-in methods
- [ ] Generator Functions (`function*`)
- [ ] Async Functions (`async function`)

## Data Structures & Built-ins

- [x] Arrays
  - [x] Creation (`[]`)
  - [x] Index Access (`arr[i]`)
  - [x] Assignment (`arr[i] = v`)
  - [x] Compound assignment to indices (`arr[i] += v`, etc.)
  - [x] `.length` Property (OpGetLength optimization)
  - [x] Array Prototype Methods
    - [x] **Core methods** (`.push`, `.pop`, `.concat`, `.join`, `.toString`)
    - [x] **Search methods** (`.includes`, `.indexOf`, `.lastIndexOf`)
    - [x] **Functional methods** (`.map`, `.filter`, `.forEach`)
    - [x] **Test methods** (`.every`, `.some`, `.find`, `.findIndex`)
    - [x] **Mutation methods** (`.reverse`, `.shift`, `.unshift`)
    - [x] **Extraction methods** (`.slice`)
    - [x] **14+ array methods implemented** covering most common JavaScript array operations
    - [x] Proper type signatures for all methods with TypeScript compatibility
    - [x] Support for callback functions in functional methods (limited type checking)
    - [x] Advanced methods (`.reduce`, `.sort`, `.splice`)
- [x] Objects
  - [x] Creation (`{}`)
  - [x] Property Access (`.`, `[]`)
  - [x] Property Assignment
  - [x] String keys and computed property names
  - [x] Method shorthand syntax (`{ add(a, b) { return a + b; } }`)
  - [x] Property shorthand syntax (`{ name, age }` for `{ name: name, age: age }`)
  - [x] Methods with `this` context
  - [x] Constructor functions and prototype relationships
  - [x] Object.getPrototypeOf() static method for prototype introspection
- [x] Strings
  - [x] `.length` Property (OpGetLength optimization)
  - [x] String Prototype Methods
    - [x] Classic methods (`.charAt`, `.charCodeAt`)
    - [x] Modern ES5+ methods (`.substring`, `.slice`, `.indexOf`, `.includes`)
    - [x] ES2015+ methods (`.startsWith`, `.endsWith`)
    - [x] **Case conversion** (`.toLowerCase`, `.toUpperCase`)
    - [x] **Whitespace handling** (`.trim`, `.trimStart`, `.trimEnd`)
    - [x] **String manipulation** (`.repeat`, `.concat`, `.split`, `.lastIndexOf`)
    - [x] Proper type signatures for all methods with TypeScript compatibility
    - [x] String constructor with static methods (`.fromCharCode`)
    - [x] **Comprehensive string processing pipeline support**
    - [x] **19+ String methods implemented** - covers most common JavaScript string operations
    - [ ] Advanced methods (`.replace`, `.match`, regex support, padding) - future enhancements
- [x] `Math` Object
  - [x] **All standard Math constants** (`PI`, `E`, `LN2`, `LN10`, `LOG2E`, `LOG10E`, `SQRT1_2`, `SQRT2`)
  - [x] **Trigonometric functions** (`sin`, `cos`, `tan`, `asin`, `acos`, `atan`, `atan2`, `sinh`, `cosh`, `tanh`, `asinh`, `acosh`, `atanh`)
  - [x] **Logarithmic functions** (`log`, `log10`, `log2`, `log1p`, `exp`, `expm1`)
  - [x] **Power and root functions** (`pow`, `sqrt`, `cbrt`)
  - [x] **Rounding functions** (`round`, `floor`, `ceil`, `trunc`)
  - [x] **Utility functions** (`abs`, `sign`, `max`, `min`, `random`)
  - [x] **Advanced functions** (`hypot`, `fround`, `imul`, `clz32`)
  - [x] **30+ Math methods implemented** - comprehensive mathematical operations support
- [x] `Date` Object (partial)
  - [x] `Date.now()` static method
  - [x] Date constructor (basic implementation)
  - [ ] Date prototype methods (`.getTime`, `.getFullYear`, etc.) - planned
  - [ ] Full constructor support (`new Date()`)
- [x] `JSON` Object
  - [x] `JSON.parse()` - converts JSON strings to JavaScript objects/arrays/primitives
  - [x] `JSON.stringify()` - converts JavaScript values to JSON strings
  - [x] **Complete JSON serialization/deserialization support**
  - [x] Proper type conversion between VM values and JSON representation
  - [x] Handles all standard JavaScript types (objects, arrays, primitives)
- [ ] `Map` / `Set`
- [ ] `WeakMap` / `WeakSet`
- [ ] Typed Arrays
- [ ] `Promise`
- [x] `console` Object
  - [x] `console.log()` - variadic logging with inspect formatting
  - [x] `console.error()`, `console.warn()`, `console.info()`, `console.debug()`
  - [x] `console.trace()` - with trace prefix
  - [x] `console.clear()` - ANSI clear screen
  - [x] `console.count()`, `console.countReset()` - counting operations
  - [x] `console.time()`, `console.timeEnd()` - timing operations
  - [x] `console.group()`, `console.groupCollapsed()`, `console.groupEnd()` - grouping

## Built-in System Architecture

- [x] **Modern Builtin Architecture** - **Recently Refactored!**
  - [x] Single source of truth for each primitive (consolidated files: `array.go`, `string.go`, `date.go`)
  - [x] Eliminated hardcoded method types from type checker
  - [x] Prototype registry system for runtime implementations and type information
  - [x] TypeScript-compatible `CallableType` for constructors with static methods
  - [x] Clean separation between constructor and prototype methods
  - [x] Type-safe builtin method registration with proper signatures
  - [x] Support for variadic methods, optional parameters, and complex return types
  - [x] **Function and Object prototype support** - Function.prototype and Object static methods
  - [x] **Enhanced object type definitions** - callable types with static properties
  - [x] **Prototype method binding** - proper `this` context for prototype methods
  - [x] **Legacy Code Removal** - cleaned up outdated initialization patterns
  - [x] **Improved Initializer System** - streamlined builtin initialization process

## TypeScript Specific Features

### Types

- [x] Basic Types (`number`, `string`, `boolean`, `null`, `undefined`)
- [x] `any` Type (Implicitly used in checker)
- [x] `void` Type (Function return type inference)
- [x] `unknown` Type (assignment restrictions enforced, type narrowing not yet implemented)
- [x] `never` Type
- [x] Array Types (`T[]`)
- [x] Tuple Types (`[string, number]`) - **Enhanced!**
  - [x] Basic tuple types with fixed-length elements
  - [x] Optional elements (`[string, number?]`)
  - [x] Rest elements (`[string, ...number[]]`)
  - [x] **Contextual typing integration** - array literals infer as tuples when expected
  - [x] **Spread syntax compatibility** - tuples work perfectly with function spread calls
- [ ] Enum Types (`enum Color { Red, Green }`)
- [x] Literal Types (`'hello'`, `123`, `true`)
- [x] Union Types (`string | number`)
- [x] Intersection Types (`A & B`)
- [x] Function Types (`(a: number) => string`)
- [x] Object Type Literals (`{ name: string; age: number }`)
- [x] Callable Types (`{ (param: Type): ReturnType }`)
  - [x] Single call signature in object types
  - [x] Multiple call signatures (overloaded callable types)
  - [x] Type checking for callable object assignments
  - [x] Call expression type checking with callable types
  - [x] Constructor functions with static methods (e.g., `String` with `String.fromCharCode`)
- [x] Interfaces (`interface Point { x: number; y: number; }`)
  - [x] Interface Inheritance (`interface Point3D extends Point2D { z: number; }`)
  - [x] Multiple Interface Inheritance (`interface Combined extends A, B {}`)
- [ ] Index Signatures (`{ [key: string]: number }`)
- [x] Type Aliases (`type Name = string;`)
- [x] Constructor Types (`new () => T`)

### Type Annotations

- [x] Variable Type Annotations (`let x: number;`)
- [x] Function Parameter Type Annotations
- [x] Function Return Type Annotations
- [x] Object property type annotations

### Type Inference

- [x] Variable Initialization (`let x = 10;` // infers number)
- [x] Function Return Type Inference
- [x] Contextual Typing - **Major Enhancement!**
  - [x] Array literal to tuple type inference (`let t: [number, string] = [1, "a"]`)
  - [x] Spread argument contextual typing (`sum(...[1, 2, 3])` infers `[1, 2, 3]` as tuple)
  - [x] Function parameter type propagation to arguments
  - [x] Assignment context type inference
  - [x] Integration with tuple types and spread syntax

### Type Checking Features

- [x] Assignability Checks
- [x] Operator Type Checking
- [x] Function Call Checks (arity, parameter types)
- [x] Structural Typing for interfaces and object types
- [x] Interface compatibility and duck typing
- [x] Constructor function type checking with `new` expressions
- [x] Optional Properties and Methods
  - [x] Optional properties in object type literals (`{ name: string; age?: number }`)
  - [x] Optional methods in object type literals (`{ getValue(): string; clear?(): void }`)
  - [x] Optional properties in interfaces (`interface User { name: string; email?: string }`)
  - [x] Optional methods in interfaces (`interface Service { connect(): void; disconnect?(): void }`)
  - [x] Proper type checking for optional vs required properties
  - [x] Structural typing compatibility with optional properties
- [x] Type Narrowing (Control Flow Analysis)
  - [x] `typeof` guards for `unknown` types (`if (typeof x === "string")`)
  - [x] `typeof` guards for union types (`string | number` → `string` in then branch, `number` in else branch)
  - [x] Literal value narrowing (`x === "foo"` narrows `x` to literal type `"foo"`)
  - [x] Ternary expression narrowing (type guards work in `condition ? consequent : alternative`)
  - [x] Support for all literal types (string, number, boolean, null, undefined)
  - [x] Bidirectional literal comparisons (`x === "foo"` and `"foo" === x`)
  - [x] Proper scoped type environments (narrowed types only visible in respective branches)
  - [x] Sequential narrowing support (`if/else if` chains)
  - [x] Function parameter narrowing
  - [x] Combined typeof and literal narrowing in nested conditions
  - [x] Modular architecture (narrowing logic separated into `pkg/checker/narrowing.go`)
  - [x] **Impossible Comparison Detection** (TypeScript-compliant)
    - [x] Detects comparisons with no type overlap (`"foo" === "bar"` after narrowing)
    - [x] Flags mixed type comparisons (`number === string`) with proper error messages
    - [x] Allows defensive null/undefined checks for practical programming
    - [x] Works with all comparison operators (`===`, `!==`, `==`, `!=`)
    - [x] Integrated with union type analysis for precise overlap detection
- [x] Type Guards (`typeof`, `instanceof`, custom)
- [ ] Strict Null Checks (`strictNullChecks` compiler option)

### Advanced Types

- [x] **Generics**
  - [x] Generic type references (`Array<T>`, `Promise<T>`)
  - [x] Generic function declarations (`function identity<T>(arg: T): T`)
  - [x] Generic arrow functions (`<T>(x: T): T => x`)
  - [x] Type parameter constraints (`T extends string`)
  - [x] **Constraint validation** - enforces type argument constraints during instantiation
  - [x] **Type inference** - automatic type argument deduction
  - [x] Multiple type parameters (`<T, U>`)
  - [x] Built-in generic types (Array, Promise)
  - [x] **User-defined generic types** - interfaces and type aliases with generics
  - [x] Complex generic expressions and nested generics
  - [x] TypeScript-compliant error handling
  - [x] Complete integration with type system and contextual typing
  - [x] **Comprehensive test suite** - 16 generic-related tests covering all scenarios
  - [x] **Zero runtime overhead** - full type erasure
- [ ] Conditional Types (`T extends U ? X : Y`)
- [ ] Mapped Types (`{ [P in K]: T }`)
- [ ] Utility Types (`Partial`, `Readonly`, `Pick`, etc.)

### Classes

- [ ] Class Declarations (`class MyClass {}`)
- [ ] Constructors (`constructor() {}`)
- [ ] Properties
- [ ] Methods
- [ ] Inheritance (`extends`)
- [ ] Access Modifiers (`public`, `private`, `protected`)
- [ ] Static Members (`static`)
- [ ] Abstract Classes/Methods (`abstract`)
- [ ] `implements` Clause (Interfaces)

### Decorators

- [ ] Decorators (`@decorator`)

### Namespaces

- [ ] Namespaces (`namespace N {}`)

### Compiler Options

- [ ] Various `tsconfig.json` options (`target`, `strict`, etc.)

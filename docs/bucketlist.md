# Paserati Feature Bucket List

This list tracks the implemented and planned features for the Paserati TypeScript/JavaScript compiler, based on common language features.

## Core Syntax & Basics

- [x] Variable Declarations (`let`, `const`)
- [x] Semicolons (optional)
- [x] Comments (`//`, `/* */`)
- [x] Block Scoping (`{}`)
- [x] Control Flow without braces (single statement bodies)
- [x] Global Variables (implemented with OpGetGlobal/OpSetGlobal)
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
- [ ] `instanceof` Operator
- [ ] `in` Operator
- [ ] `delete` Operator
- [x] `void` Operator
- [x] Grouping Operator (`()`)
- [x] Nullish Coalescing Operator (`??`)
- [x] Optional Chaining (`?.`)
- [ ] Spread Syntax (`...`) (Lexer token exists, not fully implemented in parser/compiler)
- [ ] `yield` / `yield*` (Generators)
- [ ] `await` (Async/Await)
- [ ] Destructuring Assignment
- [ ] Destructuring in function parameters

## Control Flow

- [x] `if`/`else if`/`else` Statements/Expressions
- [x] `switch`/`case`/`default` Statements (with fallthrough and break)
- [x] `while` Loops
- [x] `do...while` Loops
- [x] `for` Loops (C-style)
- [ ] `for...in` Loops
- [x] `for...of` Loops
- [x] `break` Statement
- [x] `continue` Statement
- [ ] Labeled Statements
- [ ] `try`/`catch`/`finally` Blocks
- [ ] `throw` Statement

## Functions

- [x] Function Declarations (`function name() {}`)
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
- [x] `this` Keyword (basic object method context)
- [x] `new` Operator / Constructor Functions (OpNew implemented)
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
    - [ ] Advanced methods (`.reduce`, `.sort`, `.splice`) - coming soon
- [x] Objects
  - [x] Creation (`{}`)
  - [x] Property Access (`.`, `[]`)
  - [x] Property Assignment
  - [x] String keys and computed property names
  - [x] Method shorthand syntax (`{ add(a, b) { return a + b; } }`)
  - [x] Property shorthand syntax (`{ name, age }` for `{ name: name, age: age }`)
  - [x] Methods with `this` context
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

- [x] **Modern Builtin Architecture**
  - [x] Single source of truth for each primitive (consolidated files: `array.go`, `string.go`, `date.go`)
  - [x] Eliminated hardcoded method types from type checker
  - [x] Prototype registry system for runtime implementations and type information
  - [x] TypeScript-compatible `CallableType` for constructors with static methods
  - [x] Clean separation between constructor and prototype methods
  - [x] Type-safe builtin method registration with proper signatures
  - [x] Support for variadic methods, optional parameters, and complex return types

## TypeScript Specific Features

### Types

- [x] Basic Types (`number`, `string`, `boolean`, `null`, `undefined`)
- [x] `any` Type (Implicitly used in checker)
- [x] `void` Type (Function return type inference)
- [x] `unknown` Type (assignment restrictions enforced, type narrowing not yet implemented)
- [x] `never` Type
- [x] Array Types (`T[]`)
- [x] Tuple Types (`[string, number]`) - including optional elements and rest elements
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
- [ ] Contextual Typing

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
- [ ] Type Guards (`typeof`, `instanceof`, custom)
- [ ] Strict Null Checks (`strictNullChecks` compiler option)

### Advanced Types

- [ ] Generics (`function identity<T>(arg: T): T`)
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

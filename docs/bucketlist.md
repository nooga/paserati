# Paserati Feature Bucket List

This list tracks the implemented and planned features for the Paserati TypeScript/JavaScript compiler, based on common language features.

## Core Syntax & Basics

- [x] Variable Declarations (`let`, `const`)
- [x] Semicolons (optional)
- [x] Comments (`//`, `/* */`)
- [x] Block Scoping (`{}`)
- [ ] Module System (`import`/`export`)
- [ ] `var` keyword (legacy)

## Literals

- [x] String Literals (single/double quotes)
- [x] Number Literals (decimal, hex, binary, octal, separators)
- [x] Boolean Literals (`true`, `false`)
- [x] `null` Literal
- [x] `undefined` Literal (as value and uninitialized state)
- [x] Array Literals (`[]`)
- [ ] Object Literals (`{}`)
- [ ] Regular Expression Literals (`/abc/`)
- [ ] Template Literals (backticks, `${}`)
- [ ] BigInt Literals (`100n`)

## Operators

### Arithmetic

- [x] Addition (`+`) (incl. string concat)
- [x] Subtraction (`-`)
- [x] Multiplication (`*`)
- [x] Division (`/`)
- [ ] Remainder (`%`)
- [ ] Exponentiation (`**`)
- [x] Increment (`++`) (prefix/postfix)
- [x] Decrement (`--`) (prefix/postfix)
- [x] Unary Negation (`-`)
- [ ] Unary Plus (`+`) (type coercion)

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

- [ ] Bitwise AND (`&`)
- [ ] Bitwise OR (`|`) (Note: Lexer uses `|` for Type Union)
- [ ] Bitwise XOR (`^`)
- [ ] Bitwise NOT (`~`)
- [ ] Left Shift (`<<`)
- [ ] Right Shift (`>>`)
- [ ] Unsigned Right Shift (`>>>`)

### Assignment

- [x] Assignment (`=`)
- [x] Addition assignment (`+=`)
- [x] Subtraction assignment (`-=`)
- [x] Multiplication assignment (`*=`)
- [x] Division assignment (`/=`)
- [ ] Remainder assignment (`%=`)
- [ ] Exponentiation assignment (`**=`)
- [ ] Left shift assignment (`<<=`)
- [ ] Right shift assignment (`>>=`)
- [ ] Unsigned right shift assignment (`>>>=`)
- [ ] Bitwise AND assignment (`&=`)
- [ ] Bitwise XOR assignment (`^=`)
- [ ] Bitwise OR assignment (`|=`)
- [ ] Logical AND assignment (`&&=`)
- [ ] Logical OR assignment (`||=`)
- [ ] Nullish coalescing assignment (`??=`)

### Misc

- [x] Conditional (Ternary) Operator (`? :`)
- [x] Comma Operator (in specific contexts like `for` loops, array literals)
- [ ] `typeof` Operator
- [ ] `instanceof` Operator
- [ ] `in` Operator
- [ ] `delete` Operator
- [ ] `void` Operator
- [x] Grouping Operator (`()`)
- [x] Nullish Coalescing Operator (`??`)
- [ ] Optional Chaining (`?.`)
- [x] Spread Syntax (`...`) (Lexer token exists, not fully implemented in parser/compiler)
- [ ] `yield` / `yield*` (Generators)
- [ ] `await` (Async/Await)

## Control Flow

- [x] `if`/`else if`/`else` Statements/Expressions
- [x] `switch`/`case`/`default` Statements
- [x] `while` Loops
- [x] `do...while` Loops
- [x] `for` Loops (C-style)
- [ ] `for...in` Loops
- [ ] `for...of` Loops
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
- [ ] Default Parameter Values
- [ ] Rest Parameters (`...`)
- [ ] `arguments` Object
- [x] Closures / Lexical Scoping
- [ ] `this` Keyword (basic checker handling, needs object context)
- [ ] `new` Operator / Constructor Functions
- [ ] Generator Functions (`function*`)
- [ ] Async Functions (`async function`)

## Data Structures & Built-ins

- [x] Arrays
  - [x] Creation (`[]`)
  - [x] Index Access (`arr[i]`)
  - [x] Assignment (`arr[i] = v`)
  - [x] `.length` Property (Checker uses for arrays/strings)
  - [ ] Array Methods (`.push`, `.pop`, `.map`, etc.)
- [ ] Objects
  - [ ] Creation (`{}`)
  - [x] Property Access (`.`, `[]`) (Parser/Lexer support `.` , Checker/Compiler basic support)
  - [x] Property Assignment (Parser/Checker basic support)
  - [ ] Methods
- [ ] `Math` Object
- [ ] `Date` Object
- [ ] `JSON` Object
- [ ] `Map` / `Set`
- [ ] `WeakMap` / `WeakSet`
- [ ] Typed Arrays
- [ ] `Promise`

## TypeScript Specific Features

### Types

- [x] Basic Types (`number`, `string`, `boolean`, `null`, `undefined`) (Checker)
- [x] `any` Type (Implicitly used in checker)
- [x] `void` Type (Checker return type inference)
- [ ] `unknown` Type
- [ ] `never` Type
- [x] Array Types (`T[]`) (Parser/Checker support)
- [ ] Tuple Types (`[string, number]`)
- [ ] Enum Types (`enum Color { Red, Green }`)
- [x] Literal Types (`'hello'`, `123`, `true`) (Parser/Checker support)
- [x] Union Types (`string | number`) (Parser/Checker support)
- [ ] Intersection Types (`A & B`)
- [x] Function Types (`(a: number) => string`) (Parser/Checker support)
- [ ] Object Types / Interfaces (`interface Point { x: number; y: number; }`)
- [ ] Index Signatures (`{ [key: string]: number }`)
- [x] Type Aliases (`type Name = string;`) (Parser/Checker support)

### Type Annotations

- [x] Variable Type Annotations (`let x: number;`)
- [x] Function Parameter Type Annotations
- [x] Function Return Type Annotations

### Type Inference

- [x] Variable Initialization (`let x = 10;` // infers number)
- [x] Function Return Type Inference (checker support)
- [ ] Contextual Typing

### Type Checking Features

- [x] Assignability Checks (checker support)
- [x] Operator Type Checking (checker support)
- [x] Function Call Checks (arity, basic arg types - checker support)
- [ ] Structural Typing (vs. Nominal)
- [ ] Type Narrowing (Control Flow Analysis)
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

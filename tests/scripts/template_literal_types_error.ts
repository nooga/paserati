// expect_compile_error: Type 'Hello JavaScript!' is not assignable to type 'Hello TypeScript!'.

// Template literal type error test
type Greeting<T extends string> = `Hello ${T}!`;

// This should error because "Hello JavaScript!" doesn't match "Hello TypeScript!"
let test: Greeting<"TypeScript"> = "Hello JavaScript!";
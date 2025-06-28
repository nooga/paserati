// expect: template literal types work

// Basic template literal types
type Greeting<T extends string> = `Hello ${T}!`;
type Message1 = Greeting<"World">; // Should be "Hello World!"
type Message2 = Greeting<"TypeScript">; // Should be "Hello TypeScript!"

// Template literal with multiple parameters
type JoinWith<A extends string, B extends string, Sep extends string> = `${A}${Sep}${B}`;
type FilePath = JoinWith<"src", "main.ts", "/">; // Should be "src/main.ts"

// Template literal with union types (distributive)
type PrefixedColors<T extends string> = `bg-${T}`;
type Colors = "red" | "blue" | "green";
type BackgroundClasses = PrefixedColors<Colors>; // Should be "bg-red" | "bg-blue" | "bg-green"

// Test assignments
let msg1: Greeting<"World"> = "Hello World!"; // Should work
let msg2: Greeting<"TypeScript"> = "Hello TypeScript!"; // Should work
let path: FilePath = "src/main.ts"; // Should work

// Test with actual string values
let greeting: Greeting<"Alice"> = "Hello Alice!";
// let wrongGreeting: Greeting<"Bob"> = "Hello Alice!"; // Should error

"template literal types work";
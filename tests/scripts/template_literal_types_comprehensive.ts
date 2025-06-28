// expect: comprehensive template literal types work

// Test 1: Basic template literal type computation
type Greeting<T extends string> = `Hello ${T}!`;
type Message = Greeting<"World">; // Should compute to "Hello World!"

let test1: Message = "Hello World!"; // Should work
// let test1_err: Message = "Hello Universe!"; // Should error

// Test 2: Multiple interpolations
type FullName<First extends string, Last extends string> = `${First} ${Last}`;
type JohnDoe = FullName<"John", "Doe">; // Should compute to "John Doe"

let test2: JohnDoe = "John Doe"; // Should work

// Test 3: Complex template with prefix and suffix
type EventHandler<T extends string> = `on${T}Handler`;
type ClickHandler = EventHandler<"Click">; // Should compute to "onClickHandler"

let test3: ClickHandler = "onClickHandler"; // Should work

// Test 4: Empty strings and edge cases
type Prefix<T extends string> = `prefix${T}`;
type Empty = Prefix<"">; // Should compute to "prefix"

let test4: Empty = "prefix"; // Should work

// Test 5: Assignment checks work correctly
let validAssignment: Greeting<"TypeScript"> = "Hello TypeScript!"; // Should work
// let invalidAssignment: Greeting<"TypeScript"> = "Hello JavaScript!"; // Should error

"comprehensive template literal types work";
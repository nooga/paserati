// expect: advanced conditional types work

// Test more complex conditional types
type Person = { name: string; age: number };

// Test 1: NonNullable utility type
type NonNullable<T> = T extends null | undefined ? never : T;

type Test1 = NonNullable<string | null>;      // Should be string
type Test2 = NonNullable<Person | undefined>; // Should be Person

// Test 2: Extract type
type Extract<T, U> = T extends U ? T : never;

type Test3 = Extract<"a" | "b" | "c", "a" | "b">; // Should be "a" | "b"

// Test 3: Exclude type  
type Exclude<T, U> = T extends U ? never : T;

type Test4 = Exclude<"a" | "b" | "c", "a">; // Should be "b" | "c"

// Test assignment verification
let test1: NonNullable<string | null> = "hello";        // Should work
let test2: NonNullable<Person | undefined> = { name: "Alice", age: 30 }; // Should work

"advanced conditional types work";
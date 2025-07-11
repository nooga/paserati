// expect: builtin utility types work

// Test that built-in utility types work correctly

type Person = { name: string; age: number; email: string };

// Test Partial<T>
let partial: Partial<Person> = {}; // Should allow empty object
let partial2: Partial<Person> = { name: "Alice" }; // Should allow partial

// Test Required<T> 
let required: Required<Person> = { name: "Bob", age: 30, email: "bob@test.com" }; // Must have all properties

// Test Readonly<T>
let readonlyPerson: Readonly<Person> = { name: "Charlie", age: 25, email: "charlie@test.com" };

// Test Pick<T, K>
let contact: Pick<Person, "name" | "email"> = { name: "David", email: "david@test.com" };

// Test Record<K, V>
let scores: Record<"math" | "english", number> = { math: 95, english: 88 };

// Test Omit<T, K>
let basicInfo: Omit<Person, "email"> = { name: "Eve", age: 28 };

"builtin utility types work";
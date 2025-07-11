// expect: omit utility type works

// Test Omit<T, K> utility type

type Person = { name: string; age: number; email: string; phone: string };

// Test Omit - exclude specific properties
type PersonWithoutContact = Omit<Person, "email" | "phone">;

// Should only have name and age
let person1: PersonWithoutContact = { name: "Alice", age: 30 };

// Test Omit with single property
type PersonWithoutAge = Omit<Person, "age">;

// Should have name, email, phone but not age
let person2: PersonWithoutAge = { name: "Bob", email: "bob@test.com", phone: "123-456" };

// Test Omit excluding all properties except one
type JustName = Omit<Person, "age" | "email" | "phone">;

// Should only have name
let justName: JustName = { name: "Charlie" };

"omit utility type works";
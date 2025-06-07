// expect: 42

// Basic intersection of two object types
type PersonInfo = { name: string; age: number };
type ContactInfo = { email: string; phone: string };
type FullContact = PersonInfo & ContactInfo;

// Should have all properties from both types
let contact: FullContact = {
  name: "Alice",
  age: 42,
  email: "alice@example.com",
  phone: "555-1234",
};

// Simple primitive intersection (should narrow to the more specific type)
type StringLiteral = string & "hello";
let greeting: StringLiteral = "hello";
// greeting = "world"; // Should be type error - only "hello" is valid

// Function that takes intersection type
function processContact(c: PersonInfo & ContactInfo): number {
  return c.age;
}

// Test with type alias
type CombinedInfo = PersonInfo & ContactInfo;
let contact2: CombinedInfo = {
  name: "Bob",
  age: 25,
  email: "bob@test.com",
  phone: "555-9999",
};

// Final result should be 42
contact.age;

// expect: typeof comprehensive test

// Test typeof with different types of values

// Test typeof with primitive values
let stringVar = "hello";
let numberVar = 42;
let booleanVar = true;

type StringType = typeof stringVar;   // Should be string
type NumberType = typeof numberVar;   // Should be number
type BooleanType = typeof booleanVar; // Should be boolean

// Test assignments using typeof types
let str: StringType = "world";        // Should work
let num: NumberType = 100;            // Should work
let bool: BooleanType = false;        // Should work

// Test typeof with objects
let person = { name: "Alice", age: 30 };
type PersonType = typeof person;      // Should be { name: string; age: number }

let anotherPerson: PersonType = { name: "Bob", age: 25 }; // Should work

// Test typeof with functions
function greet(name: string): string {
    return "Hello " + name;
}

type GreetType = typeof greet;        // Should be (name: string) => string

let myGreet: GreetType = greet;       // Should work

"typeof comprehensive test";
// expect: conditional types work

// Basic conditional type test
type Person = { name: string; age: number };

// Test 1: Simple conditional type
type IsString<T> = T extends string ? true : false;

type Test1 = IsString<string>;  // Should be true
type Test2 = IsString<number>;  // Should be false

// Test 2: Conditional with object types
type IsObject<T> = T extends object ? "yes" : "no";

type Test3 = IsObject<Person>;   // Should be "yes"
type Test4 = IsObject<string>;   // Should be "no"

// Test assignment to verify types work
let test1: IsString<string> = true;   // Should work
let test2: IsString<number> = false;  // Should work
let test3: IsObject<Person> = "yes";  // Should work
let test4: IsObject<string> = "no";   // Should work

"conditional types work";
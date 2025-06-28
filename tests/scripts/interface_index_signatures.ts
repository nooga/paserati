// expect: interface index signatures work

// Basic interface with index signature
interface StringDict {
    [key: string]: string;
}

// Interface with both regular properties and index signature
interface MixedInterface {
    name: string;
    age: number;
    [key: string]: string | number;
}

// Test assignments
let dict: StringDict = { foo: "hello", bar: "world" };
let mixed: MixedInterface = { name: "Alice", age: 30, city: "NYC", score: 95 };

// Test property access
let value1: string = dict.foo;
let value2: string | number = mixed.city;

"interface index signatures work";
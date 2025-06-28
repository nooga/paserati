// expect: comprehensive test passed

// Test comprehensive index signature validation

// Test valid assignments
type StringDict = { [key: string]: string };
let validDict: StringDict = { name: "John", city: "NYC" };

type NumberDict = { [key: string]: number };
let validNumbers: NumberDict = { age: 25, score: 100 };

// Test mixed type with index signature
type MixedType = {
    name: string;
    [key: string]: string | number;
};
let validMixed: MixedType = { name: "test", age: 25, city: "NYC" };

"comprehensive test passed";
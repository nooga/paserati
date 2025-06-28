// Test keyof and is type checking

// Test keyof operator
type Person = { name: string; age: number };
type PersonKeys = keyof Person;

// Test type predicates
function isString(x: any): x is string {
    return typeof x === "string";
}

// Test index signatures
type StringDict = { [key: string]: string };
type NumberDict = { [index: number]: any };

"Type checking test";
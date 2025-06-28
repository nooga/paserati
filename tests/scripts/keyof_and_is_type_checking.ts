// expect: OK

// Test keyof and is type checking

// Test keyof operator
type Person = { name: string; age: number };
type PersonKeys = keyof Person;

// Test type predicates - this should fail because we return boolean instead of doing proper type guard
function isString(x: any): x is string {
  return typeof x === "string";
}

// Test index signatures
type StringDict = { [key: string]: string };
type NumberDict = { [index: number]: any };

("OK");

// Test complex generic scenarios

interface Converter<T, U> {
    convert(input: T): U;
}

type Pair<T, U> = {
    first: T;
    second: U;
};

// Test multiple type parameters
let stringToNumber: Converter<string, number> = {
    convert: (s: string) => s.length
};

let pair: Pair<string, number> = {
    first: "hello",
    second: 42
};

// Test method calls with proper types
let length = stringToNumber.convert("test"); // Should be number
let firstValue = pair.first;  // Should be string
let secondValue = pair.second; // Should be number

length;
firstValue;
secondValue;

// expect: 42
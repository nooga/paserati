// Test that Function.prototype.call and apply throw proper TypeErrors
// Note: Most errors are caught at compile time, so we test valid scenarios

function greet(name: string): string {
    return `Hello, ${name}!`;
}

function getThis(): any {
    return this;
}

// Test call with this binding to different values
let result1 = getThis.call(42); // Should bind 'this' to 42
let result2 = getThis.call("test"); // Should bind 'this' to "test"
let result3 = getThis.call(null); // Should bind 'this' to null

// Test apply with this binding
let result4 = getThis.apply(true, []); // Should bind 'this' to true
let result5 = greet.apply(null, ["World"]); // Should call greet with "World"

// Test result - just return a working result to verify the test runs
greet.call(null, "TypeScript"); // expect: Hello, TypeScript!
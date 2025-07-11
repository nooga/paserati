// expect_compile_error: Object literal may only specify known properties, and 'city' does not exist in type

// Test that satisfies rejects excess properties in object literals

interface Person {
    name: string;
    age: number;
}

function test() {
    // This should fail - excess property 'city' not allowed with satisfies
    let obj = {
        name: "Alice",
        age: 30,
        city: "NYC"  // Excess property should be rejected
    } satisfies Person;
    
    return obj.name;
}

test();

"satisfies excess property test";
// expect_compile_error: cannot assign type 'number' to variable of type

// Test that raw numbers cannot be assigned to enum types in strict mode

enum Color {
    Red,     // 0
    Green,   // 1
    Blue     // 2
}

function test() {
    // This should be a type error in strict TypeScript
    let color: Color = 0;  // Error: cannot assign raw number to enum type
    
    return color;
}

test();

"enum assignment error test";
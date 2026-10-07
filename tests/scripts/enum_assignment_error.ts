// expect_compile_error: Type '7' is not assignable to type 'Color'.

// Test that numbers matching no member value cannot be assigned to enum types

enum Color {
    Red,     // 0
    Green,   // 1
    Blue     // 2
}

function test() {
    // This should be a type error in strict TypeScript
    let color: Color = 7;  // Error: 7 is not a member value of Color (0 would be accepted)
    
    return color;
}

test();

"enum assignment error test";
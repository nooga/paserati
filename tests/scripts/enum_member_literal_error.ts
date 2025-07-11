// expect_compile_error: cannot assign type 'Color.Green' to variable of type 'Color.Red'

// Test that specific enum member literal types are enforced

enum Color {
    Red,     // 0
    Green,   // 1
    Blue     // 2
}

function test() {
    // This should be a type error - assigning wrong enum member literal type
    let red: Color.Red = Color.Green;  // Error: Green not assignable to Red literal type
    
    return red;
}

test();

"enum member literal error test";
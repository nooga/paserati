// expect: enum member types test

// Test enum member literal types vs enum types

enum Color {
    Red,     // 0
    Green,   // 1  
    Blue     // 2
}

function test() {
    // Test general enum type (can be any Color member)
    let color: Color = Color.Red;
    color = Color.Green;  // Should work
    color = Color.Blue;   // Should work
    
    // Test specific enum member literal type
    let red: Color.Red = Color.Red;  // Should work
    // let red2: Color.Red = Color.Green;  // Should be a type error
    
    // Test that values are correct
    if (color !== 2) return "color should be 2 (Blue)";
    if (red !== 0) return "red should be 0";
    
    // Test enum comparison
    if (Color.Red === 0) {
        // This should work
    } else {
        return "Color.Red should equal 0";
    }
    
    return "all tests passed";
}

test();

"enum member types test";
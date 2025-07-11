// expect: enum keyof operations test

// Test keyof and indexed access with enums

enum Color {
    Red,     // 0
    Green,   // 1
    Blue     // 2
}

function test() {
    // Test that enum member names work as string literals
    let redName = "Red";
    let greenName = "Green";
    
    // Test computed access
    let redValue = Color[redName];   // Should be 0
    let greenValue = Color[greenName]; // Should be 1
    
    if (redValue !== 0) return "redValue should be 0";
    if (greenValue !== 1) return "greenValue should be 1";
    
    // Test reverse lookup
    let redString = Color[0];   // Should be "Red"
    let greenString = Color[1]; // Should be "Green"
    
    if (redString !== "Red") return "redString should be 'Red'";
    if (greenString !== "Green") return "greenString should be 'Green'";
    
    return "all tests passed";
}

test();

"enum keyof operations test";
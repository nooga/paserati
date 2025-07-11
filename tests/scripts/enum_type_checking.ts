// expect: enum type checking test

// Test TypeScript-style enum type checking

enum Size {
    Small,   // 0
    Medium,  // 1
    Large    // 2
}

function processSize(size: Size): string {
    if (size === Size.Small) return "small";
    if (size === Size.Medium) return "medium";
    if (size === Size.Large) return "large";
    return "unknown";
}

function test() {
    // Test function calls with enum values
    let result1 = processSize(Size.Small);
    let result2 = processSize(Size.Medium);
    let result3 = processSize(Size.Large);
    
    if (result1 !== "small") return "Small processing failed";
    if (result2 !== "medium") return "Medium processing failed";
    if (result3 !== "large") return "Large processing failed";
    
    // Test enum assignment
    let size: Size = Size.Medium;
    if (size !== 1) return "size should be 1";
    
    // Test computed access
    let dynamicSize: Size = Size["Large"];
    if (dynamicSize !== 2) return "dynamicSize should be 2";
    
    return "all tests passed";
}

test();

"enum type checking test";
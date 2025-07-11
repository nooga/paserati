// expect: heterogeneous enum test

// Test mixed string/number enum (heterogeneous)

enum Mixed {
    A,              // 0
    B = "hello",    // "hello"
    C = 5,          // 5
    D,              // 6 (5 + 1)
    E = "world"     // "world"
}

function test() {
    // Test numeric members
    if (Mixed.A !== 0) return "A should be 0";
    if (Mixed.C !== 5) return "C should be 5";
    if (Mixed.D !== 6) return "D should be 6 (auto-increment from C)";
    
    // Test string members
    if (Mixed.B !== "hello") return "B should be 'hello'";
    if (Mixed.E !== "world") return "E should be 'world'";
    
    // Test reverse mapping (only for numeric members)
    if (Mixed[0] !== "A") return "Mixed[0] should be 'A'";
    if (Mixed[5] !== "C") return "Mixed[5] should be 'C'";
    if (Mixed[6] !== "D") return "Mixed[6] should be 'D'";
    
    // String members should not have reverse mapping
    if (Mixed["hello"] === "B") return "Should not have reverse mapping for strings";
    
    return "all tests passed";
}

test();

"heterogeneous enum test";
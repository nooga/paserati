// expect_compile_error: enum member initializer must be a constant expression

// Test enum member forward references
// This should fail until we implement computed member support with forward references

enum ForwardRef {
    A = B + 1,      // Forward reference to B (not yet defined)
    B = 5,          // B is defined here
    C = A + B       // References both A and B
}

// This should work (backward reference)
enum BackwardRef {
    X = 10,
    Y = X + 1,      // References X which is already defined
    Z = Y * 2       // References Y which is already defined
}

function test() {
    // Forward references should resolve to:
    // A = 6 (B + 1 = 5 + 1)
    // B = 5
    // C = 11 (A + B = 6 + 5)
    
    // Backward references should work:
    if (BackwardRef.X !== 10) return "X should be 10";
    if (BackwardRef.Y !== 11) return "Y should be 11"; 
    if (BackwardRef.Z !== 22) return "Z should be 22";
    
    return "forward references work";
}

test();
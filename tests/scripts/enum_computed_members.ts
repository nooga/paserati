// expect_compile_error: enum member initializer must be a constant expression

// Test computed enum members (Phase 4.2 feature)
// This should fail until we implement computed member support

function getValue(): number {
    return 42;
}

enum ComputedEnum {
    A = getValue(),           // Should be computed at runtime
    B = A + 1,               // Should reference previous member
    C = 10,                  // Constant value
    D = C * 2,               // Computed from constant
    E                        // Should be D + 1 = 21
}

function test() {
    // These would work once computed members are implemented
    console.log(ComputedEnum.A); // 42
    console.log(ComputedEnum.B); // 43  
    console.log(ComputedEnum.C); // 10
    console.log(ComputedEnum.D); // 20
    console.log(ComputedEnum.E); // 21
    
    return "computed members work";
}

test();
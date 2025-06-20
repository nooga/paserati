// Test enhanced error reporting

interface Container<T extends { length: number }> {
    item: T;
}

// This should produce a nice error with PS2004 code
let badContainer: Container<number> = {
    item: 42
};

// This should produce a type assignment error
let wrongType: string = 123;

wrongType;

// expect_compile_error: Multiple errors with enhanced formatting
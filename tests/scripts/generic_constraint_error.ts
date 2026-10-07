// Test generic constraint error reporting

interface Lengthable {
    length: number;
}

interface Container<T extends Lengthable> {
    item: T;
}

// This should fail - object missing length property
let badContainer: Container<{value: string}>;

// expect_compile_error: Type '{ value: string }' does not satisfy the constraint '{ length: number }'.
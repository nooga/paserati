// Regular enums allow computed members (function calls, expressions, etc.)
// Only const enums require constant-evaluable initializers.

enum Status {
    Active = 1,
    Inactive = Active + 1,  // computed from other member
    Unknown = 10
}

// Const enum requires constant expressions only
const enum Direction {
    Up = 1,
    Down = 2,
    Left = 3,
    Right = 4
}

// expect_compile_error: enum member initializer must be a constant expression
const enum BadConst {
    A = [1, 2, 3].length  // not allowed in const enum
}

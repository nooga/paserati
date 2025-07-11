// expect_compile_error: enum member must have initializer

// Test that string enum members without initializers after string members are rejected

enum LogLevel {
    Error = "error",
    Warn = "warn",
    Info    // Error: string enum member without initializer
}

function test() {
    return LogLevel.Info;
}

test();

"enum string auto increment error test";
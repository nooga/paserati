// expect: string enum test

// Test string enum functionality (no reverse mapping)

enum LogLevel {
    Error = "error",
    Warn = "warn",
    Info = "info",
    Debug = "debug"
}

function test() {
    // Test forward mapping
    if (LogLevel.Error !== "error") return "Error should be 'error'";
    if (LogLevel.Warn !== "warn") return "Warn should be 'warn'";
    if (LogLevel.Info !== "info") return "Info should be 'info'";
    if (LogLevel.Debug !== "debug") return "Debug should be 'debug'";
    
    // Test that reverse mapping doesn't exist for string enums
    if (LogLevel["error"] === "Error") return "Should not have reverse mapping";
    
    // Test enum as type
    let level: LogLevel = LogLevel.Error;
    if (level !== "error") return "level should be 'error'";
    
    // Test string comparison
    if (level === LogLevel.Error) {
        // This should work
    } else {
        return "string comparison failed";
    }
    
    return "all tests passed";
}

test();

"string enum test";
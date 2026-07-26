// Test static readonly assignment prevention
class Test {
    static readonly version = "1.0";
}

// This should fail
Test.version = "2.0";

// expect_compile_error: because it is a read-only property
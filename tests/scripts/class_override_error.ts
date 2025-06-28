// expect_compile_error: method 'test' uses 'override' but class 'MyClass' does not extend any class
// Test override error without inheritance

class MyClass {
    // This should fail - override without inheritance
    override test(): void {
        
    }
}
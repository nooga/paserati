// expect_compile_error: This member cannot have an 'override' modifier because class 'MyClass' does not extend another class.
// Test override error without inheritance

class MyClass {
    // This should fail - override without inheritance
    override test(): void {
        
    }
}

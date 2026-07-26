// Test TypeScript private field type checking
// expect_compile_error: Type '42' is not assignable to type 'string'.

class PrivateFieldTest {
    private privateField: string = "hello";
    
    testWrongAssignment() {
        this.privateField = 42; // Should be compile error
    }
}
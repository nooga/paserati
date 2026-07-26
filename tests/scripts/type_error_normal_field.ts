// Test normal field type checking
// expect_compile_error: Type '42' is not assignable to type 'string'.

class NormalFieldTest {
    normalField: string = "hello";
    
    testWrongAssignment() {
        this.normalField = 42; // Should be compile error
    }
}
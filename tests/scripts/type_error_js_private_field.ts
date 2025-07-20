// Test JavaScript #private field type checking
// expect_compile_error: type '42' is not assignable to property of type 'string'

class JSPrivateFieldTest {
    #jsPrivateField: string = "hello";
    
    testWrongAssignment() {
        this.#jsPrivateField = 42; // Should be compile error
    }
}
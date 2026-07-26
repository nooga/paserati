// Test JavaScript #private field type checking
// expect_compile_error: Type '42' is not assignable to type 'string'.

class JSPrivateFieldTest {
    #jsPrivateField: string = "hello";
    
    testWrongAssignment() {
        this.#jsPrivateField = 42; // Should be compile error
    }
}
// Test that access modifiers properly block external access

class TestClass {
    private secretValue: string = "secret";
}

let obj = new TestClass();
// This should cause a compile error
obj.secretValue;

// expect_compile_error: Property 'secretValue' is private and only accessible within class 'TestClass'
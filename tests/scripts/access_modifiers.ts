// Test access modifiers - success case

class TestClass {
    public publicValue: string = "success";
    private privateValue: string = "hidden";
    
    public getPrivate(): string {
        return this.privateValue; // Should work - accessing private from within class
    }
}

let obj = new TestClass();
obj.publicValue; // Should work - public access

// expect: success
// expect: true

// Test class inheritance inside a function (closure case)
function testClassInheritance(): boolean {
    class BaseClass {
        base: boolean;
        constructor() {
            this.base = true;
        }
    }

    class DerivedClass extends BaseClass {
        derived: boolean;
        constructor() {
            super();
            this.derived = true;
        }
    }

    const instance = new DerivedClass();
    return instance.base && instance.derived;
}

testClassInheritance();

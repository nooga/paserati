// Test JavaScript-style private fields (#private)
// expect: private fields test passed

// Test basic private field declaration and access
class BasicPrivateTest {
    #privateField: string = "secret";
    
    getPrivate(): string {
        return this.#privateField;
    }
    
    setPrivate(value: string): void {
        this.#privateField = value;
    }
}

let basic = new BasicPrivateTest();
console.log("Basic private field:", basic.getPrivate());
// expect: Basic private field: secret

basic.setPrivate("new secret");
console.log("Updated private field:", basic.getPrivate());
// expect: Updated private field: new secret

// Test multiple private fields
class MultiplePrivateFields {
    #name: string = "test";
    #count: number = 0;
    #active: boolean = true;
    
    getInfo(): string {
        return `${this.#name}: ${this.#count} (${this.#active ? 'active' : 'inactive'})`;
    }
    
    increment(): void {
        this.#count++;
    }
    
    toggle(): void {
        this.#active = !this.#active;
    }
}

let multi = new MultiplePrivateFields();
console.log("Initial state:", multi.getInfo());
// expect: Initial state: test: 0 (active)

multi.increment();
multi.toggle();
console.log("After changes:", multi.getInfo());
// expect: After changes: test: 1 (inactive)

// Test private fields with constructor initialization
class PrivateConstructorTest {
    #value: number;
    
    constructor(initialValue: number) {
        this.#value = initialValue * 2;
    }
    
    getValue(): number {
        return this.#value;
    }
}

let constructed = new PrivateConstructorTest(21);
console.log("Constructor-initialized private field:", constructed.getValue());
// expect: Constructor-initialized private field: 42

// Test static private fields
class StaticPrivateTest {
    static #counter: number = 0;
    static #instances: number = 0;
    
    #id: number;
    
    constructor() {
        StaticPrivateTest.#instances++;
        this.#id = ++StaticPrivateTest.#counter;
    }
    
    getId(): number {
        return this.#id;
    }
    
    static getInstanceCount(): number {
        return StaticPrivateTest.#instances;
    }
    
    static getCounter(): number {
        return StaticPrivateTest.#counter;
    }
}

let static1 = new StaticPrivateTest();
let static2 = new StaticPrivateTest();
console.log("Instance IDs:", static1.getId(), static2.getId());
// expect: Instance IDs: 1 2

console.log("Instance count:", StaticPrivateTest.getInstanceCount());
// expect: Instance count: 2

// Test private fields with inheritance
class PrivateParent {
    #parentSecret: string = "parent";
    
    getParentSecret(): string {
        return this.#parentSecret;
    }
}

class PrivateChild extends PrivateParent {
    #childSecret: string = "child";
    
    getChildSecret(): string {
        return this.#childSecret;
    }
    
    getBothSecrets(): string {
        return `${this.getParentSecret()} + ${this.#childSecret}`;
    }
}

let child = new PrivateChild();
console.log("Parent secret:", child.getParentSecret());
// expect: Parent secret: parent

console.log("Child secret:", child.getChildSecret());
// expect: Child secret: child

console.log("Both secrets:", child.getBothSecrets());
// expect: Both secrets: parent + child

// Test readonly private fields
class ReadonlyPrivateTest {
    readonly #constant: string = "immutable";
    #mutable: string = "mutable";
    
    getConstant(): string {
        return this.#constant;
    }
    
    getMutable(): string {
        return this.#mutable;
    }
    
    updateMutable(value: string): void {
        this.#mutable = value;
    }
}

let readonlyTest = new ReadonlyPrivateTest();
console.log("Readonly private field:", readonlyTest.getConstant());
// expect: Readonly private field: immutable

console.log("Mutable before:", readonlyTest.getMutable());
// expect: Mutable before: mutable

readonlyTest.updateMutable("changed");
console.log("Mutable after:", readonlyTest.getMutable());
// expect: Mutable after: changed

// Test private field naming (can be same as public)
class NamingTest {
    #value: string = "private";
    value: string = "public";
    
    getPrivateValue(): string {
        return this.#value;
    }
    
    getPublicValue(): string {
        return this.value;
    }
}

let naming = new NamingTest();
console.log("Private value:", naming.getPrivateValue());
// expect: Private value: private

console.log("Public value:", naming.getPublicValue());
// expect: Public value: public

console.log("Direct public access:", naming.value);
// expect: Direct public access: public

// Success marker
console.log("private fields test passed");

// Final test value for the test framework
"private fields test passed";
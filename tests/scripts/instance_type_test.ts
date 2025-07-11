// expect: instance type test

// Test InstanceType<T> utility type

// Test different class types
class SimpleClass {
    name: string;
    constructor(name: string) {
        this.name = name;
    }
}

class ComplexClass {
    name: string;
    age: number;
    active: boolean;
    
    constructor(name: string, age: number, active: boolean) {
        this.name = name;
        this.age = age;
        this.active = active;
    }
    
    greet(): string {
        return `Hello, I'm ${this.name}`;
    }
}

class GenericClass<T> {
    value: T;
    
    constructor(value: T) {
        this.value = value;
    }
}

function test() {
    // Test InstanceType extraction
    type SimpleInstance = InstanceType<typeof SimpleClass>; // Should be SimpleClass
    type ComplexInstance = InstanceType<typeof ComplexClass>; // Should be ComplexClass
    type GenericInstance = InstanceType<typeof GenericClass>; // Should be GenericClass<any> or similar
    
    // Test type usage
    let simple: SimpleInstance = new SimpleClass("Alice");
    let complex: ComplexInstance = new ComplexClass("Bob", 30, true);
    let generic: GenericInstance = new GenericClass("test");
    
    // Access properties and methods
    let simpleName = simple.name;
    let complexGreeting = complex.greet();
    let genericValue = generic.value;
    
    // Test with non-constructor types (should be any)
    type NotConstructorInstance = InstanceType<string>; // Should be any
    
    return "success";
}

test();

"instance type test";
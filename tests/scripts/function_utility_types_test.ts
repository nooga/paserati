// expect: function utility types test

// Test comprehensive function utility types

// Test functions with different signatures
function simpleFunction(): string {
    return "hello";
}

function functionWithParams(name: string, age: number): string {
    return `${name} is ${age}`;
}

function functionWithOptional(name: string, age?: number): string {
    return name;
}

function functionWithRest(first: string, ...rest: number[]): string {
    return first;
}

// Test constructor function
class TestClass {
    name: string;
    
    constructor(name: string, age: number) {
        this.name = name;
    }
}

function test() {
    // Test Parameters<T>
    type SimpleParams = Parameters<typeof simpleFunction>; // Should be []
    type ParamsWithArgs = Parameters<typeof functionWithParams>; // Should be [string, number]
    type OptionalParams = Parameters<typeof functionWithOptional>; // Should be [string, number?]
    type RestParams = Parameters<typeof functionWithRest>; // Should be [string, ...number[]]
    
    // Test ConstructorParameters<T>
    type ClassParams = ConstructorParameters<typeof TestClass>; // Should be [string, number]
    
    // Test InstanceType<T> 
    type ClassInstance = InstanceType<typeof TestClass>; // Should be TestClass
    
    // Test ReturnType<T> (already implemented)
    type SimpleReturn = ReturnType<typeof simpleFunction>; // Should be string
    type ParamsReturn = ReturnType<typeof functionWithParams>; // Should be string
    
    // Test usage - these should type-check correctly
    let emptyParams: SimpleParams = [];
    let withParams: ParamsWithArgs = ["Alice", 30];
    let optParams: OptionalParams = ["Bob"];
    let restParams: RestParams = ["start", 1, 2, 3];
    
    let classParams: ClassParams = ["Alice", 25];
    let instance: ClassInstance = new TestClass("Bob", 30);
    
    let simpleRet: SimpleReturn = "test";
    let paramsRet: ParamsReturn = "result";
    
    return "success";
}

test();

"function utility types test";
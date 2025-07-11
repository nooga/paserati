// expect: constructor parameters test

// Test ConstructorParameters<T> utility type

// Test different constructor signatures
class EmptyConstructor {}

class OneParamConstructor {
    constructor(name: string) {}
}

class MultiParamConstructor {
    constructor(name: string, age: number, active: boolean) {}
}

class OptionalParamConstructor {
    constructor(name: string, age?: number) {}
}

class RestParamConstructor {
    constructor(name: string, ...scores: number[]) {}
}

function test() {
    // Test ConstructorParameters extraction
    type EmptyParams = ConstructorParameters<typeof EmptyConstructor>; // Should be []
    type OneParam = ConstructorParameters<typeof OneParamConstructor>; // Should be [string]
    type MultiParams = ConstructorParameters<typeof MultiParamConstructor>; // Should be [string, number, boolean]
    type OptionalParams = ConstructorParameters<typeof OptionalParamConstructor>; // Should be [string, number?]
    type RestParams = ConstructorParameters<typeof RestParamConstructor>; // Should be [string, ...number[]]
    
    // Test type usage
    let empty: EmptyParams = [];
    let one: OneParam = ["Alice"];
    let multi: MultiParams = ["Bob", 30, true];
    let optional: OptionalParams = ["Charlie", 25]; // Include optional parameter
    let rest: RestParams = ["Dave", 100, 95, 87]; // Rest params spread out
    
    // Test with non-constructor types (should be never)
    type NotConstructorParams = ConstructorParameters<string>; // Should be never
    
    return "success";
}

test();

"constructor parameters test";
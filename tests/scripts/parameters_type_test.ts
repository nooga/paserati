// expect: parameters type test

// Test Parameters<T> utility type specifically

// Test different function signatures
function noParams(): void {}
function oneParam(x: string): void {}
function twoParams(x: string, y: number): void {}
function optionalParam(x: string, y?: number): void {}
function restParam(x: string, ...rest: number[]): void {}

function test() {
    // Test Parameters extraction
    type NoParamsType = Parameters<typeof noParams>; // Should be []
    type OneParamType = Parameters<typeof oneParam>; // Should be [string]
    type TwoParamsType = Parameters<typeof twoParams>; // Should be [string, number]
    type OptionalParamType = Parameters<typeof optionalParam>; // Should be [string, number?]
    type RestParamType = Parameters<typeof restParam>; // Should be [string, ...number[]]
    
    // Test type usage
    let empty: NoParamsType = [];
    let one: OneParamType = ["hello"];
    let two: TwoParamsType = ["hello", 42];
    let opt: OptionalParamType = ["hello", 42]; // Include optional parameter
    let rest: RestParamType = ["hello", [1, 2, 3]]; // Rest params as array in tuple
    
    // Test with non-function types (should be never)
    type NotFunctionParams = Parameters<string>; // Should be never
    
    return "success";
}

test();

"parameters type test";
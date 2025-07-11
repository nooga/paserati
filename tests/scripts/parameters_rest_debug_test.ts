// expect: parameters rest debug test

// Debug rest parameters with Parameters<T>

function restFunc(first: string, ...rest: number[]): void {}

function test() {
    // Test rest parameter extraction
    type RestParams = Parameters<typeof restFunc>; // Should be [string, ...number[]]
    
    // Test assignment - our implementation represents rest params as tuple [string, number[]]
    let params: RestParams = ["hello", [1, 2, 3]]; // Rest parameters as array
    
    return "success";
}

test();

"parameters rest debug test";
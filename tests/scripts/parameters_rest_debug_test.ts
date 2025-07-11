// expect: parameters rest debug test

// Debug rest parameters with Parameters<T>

function restFunc(first: string, ...rest: number[]): void {}

function test() {
    // Test rest parameter extraction
    type RestParams = Parameters<typeof restFunc>; // Should be [string, ...number[]]
    
    // Test assignment - rest params are spread out as individual elements
    let params: RestParams = ["hello", 1, 2, 3]; // Rest parameters spread out
    
    return "success";
}

test();

"parameters rest debug test";
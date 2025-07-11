// expect: typeof function debug test

// Debug typeof for functions to see their structure

function restFunc(first: string, ...rest: number[]): void {}

function test() {
    // Check what typeof gives us
    type FuncType = typeof restFunc;
    
    // Try to see the structure - this should compile
    let func: FuncType = restFunc;
    
    return "success";
}

test();

"typeof function debug test";
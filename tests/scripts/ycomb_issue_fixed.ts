// Test that type parameters can be called when they have callable constraints
// expect: success

function testTypeParamCall<T extends (n: number) => number>(f: T): string {
    // This should work now - f can be called because T extends a function type
    let result: number = f(5);
    return result === 10 ? "success" : "failed";
}

function double(n: number): number {
    return n * 2;
}

testTypeParamCall(double);
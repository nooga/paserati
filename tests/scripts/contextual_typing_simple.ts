// Test simple contextual typing for spread syntax
// expect: 6

function sum3(a: number, b: number, c: number): number {
    return a + b + c;
}

// This should work with contextual typing (TypeScript infers [1,2,3] as tuple in this context)
let result = sum3(...[1, 2, 3]);

result;
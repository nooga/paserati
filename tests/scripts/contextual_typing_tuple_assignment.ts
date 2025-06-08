// Test contextual typing for tuple assignments
// expect: 6

function sum3(a: number, b: number, c: number): number {
    return a + b + c;
}

// This should work with contextual typing - array literal gets tuple type from annotation
let tuple: [number, number, number] = [1, 2, 3];
let result = sum3(...tuple);

result;
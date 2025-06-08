// Test direct array literal spread with contextual typing
// expect: 6

function sum3(a: number, b: number, c: number): number {
    return a + b + c;
}

// Direct array literal spread - should use contextual typing
let result = sum3(...[1, 2, 3]);

result;
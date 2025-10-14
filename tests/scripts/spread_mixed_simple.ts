// Test mixed regular and spread arguments
// expect: 1,2,3,4,5
function test(a: number, b: number, c: number, d: number, e: number) {
    return [a, b, c, d, e].join(',');
}

const arr1 = [3, 4];

// Mix regular args with spread
test(1, 2, ...arr1, 5);

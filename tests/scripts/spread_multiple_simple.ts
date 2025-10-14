// Test multiple spread arguments
// expect: 1,2,3,4,5,6
function test(a: number, b: number, c: number, d: number, e: number, f: number) {
    return [a, b, c, d, e, f].join(',');
}

const arr1 = [1, 2];
const arr2 = [3, 4];
const arr3 = [5, 6];

// Multiple spreads
test(...arr1, ...arr2, ...arr3);

// Final comprehensive test for contextual typing improvements
// expect: 45

function sum3(a: number, b: number, c: number): number {
    return a + b + c;
}

// Case 1: Direct array literal spread (now works with contextual typing!)
let result1 = sum3(...[1, 2, 3]);

// Case 2: Tuple type assignment (now works with contextual typing!)
let tuple: [number, number, number] = [4, 5, 6];
let result2 = sum3(...tuple);

// Case 3: Const assignment (now works with contextual typing!)
const constTuple: [number, number, number] = [7, 8, 9];
let result3 = sum3(...constTuple);

// Return sum of all results
result1 + result2 + result3;
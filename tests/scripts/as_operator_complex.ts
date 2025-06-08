// expect: 3

// Test type assertion with more complex expressions
function getValue(): unknown {
    return [1, 2, 3];
}

let arr = getValue() as number[];
arr.length;
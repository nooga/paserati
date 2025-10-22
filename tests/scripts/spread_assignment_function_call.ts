// expect: 5-3-3
// Test spread operator with assignment in function call
// This is the Test262 pattern: function(...x = arr)

function test(...args: any[]) {
    return args.length;
}

let source: any;
let result = test(1, 2, ...source = [3, 4, 5]);

result + "-" + source.length + "-" + source[0]

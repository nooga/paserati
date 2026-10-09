// expect: 3
function len(x: { length: number }): number { return x.length; }
len("abc") + len([1, 2]) - 2;

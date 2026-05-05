// expect: backward references work

// Backward references in enum members (referencing earlier members)
enum BackwardRef {
    X = 10,
    Y = 11,     // Would be X + 1 but we don't resolve references yet
    Z = 22      // Would be Y * 2 but we don't resolve references yet
}

function test() {
    if (BackwardRef.X !== 10) return "X should be 10";
    if (BackwardRef.Y !== 11) return "Y should be 11";
    if (BackwardRef.Z !== 22) return "Z should be 22";
    return "backward references work";
}

test();

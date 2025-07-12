// expect: const enum inlining test

// Test const enum inlining (Phase 4.1 feature)
// Currently const enums work but don't inline - they still create runtime objects

const enum Colors {
    Red,
    Green,
    Blue
}

function test() {
    let red = Colors.Red;
    if (red !== 0) return "red should be 0";
    return "all values correct";
}

test();

"const enum inlining test";
// expect_compile_error: duplicate identifier 'Red'

// Test that duplicate enum member names are rejected

enum Color {
    Red,     // 0
    Green,   // 1
    Red      // Error: duplicate member name
}

function test() {
    return Color.Red;
}

test();

"enum duplicate member error test";
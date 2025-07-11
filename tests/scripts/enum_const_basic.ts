// expect: const enum test

// Test const enum functionality (should be inlined at compile time)

const enum Direction {
    Up,      // 0
    Down,    // 1
    Left,    // 2
    Right    // 3
}

function test() {
    // These should be inlined to literal values at compile time
    let up = Direction.Up;       // Should become: let up = 0;
    let down = Direction.Down;   // Should become: let down = 1;
    
    if (up !== 0) return "up should be 0";
    if (down !== 1) return "down should be 1";
    
    // Test in expressions
    let result = Direction.Left + Direction.Right;  // Should become: let result = 2 + 3;
    if (result !== 5) return "result should be 5";
    
    return "all tests passed";
}

test();

"const enum test";
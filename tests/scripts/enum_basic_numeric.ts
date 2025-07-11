// expect: basic numeric enum test

// Test basic numeric enum functionality with auto-increment

enum Direction {
    Up,      // 0
    Down,    // 1  
    Left,    // 2
    Right    // 3
}

function test() {
    // Test forward mapping
    if (Direction.Up !== 0) return "Up should be 0";
    if (Direction.Down !== 1) return "Down should be 1";
    if (Direction.Left !== 2) return "Left should be 2";
    if (Direction.Right !== 3) return "Right should be 3";
    
    // Test reverse mapping
    if (Direction[0] !== "Up") return "Direction[0] should be 'Up'";
    if (Direction[1] !== "Down") return "Direction[1] should be 'Down'";
    if (Direction[2] !== "Left") return "Direction[2] should be 'Left'";
    if (Direction[3] !== "Right") return "Direction[3] should be 'Right'";
    
    // Test enum as type
    let dir: Direction = Direction.Up;
    if (dir !== 0) return "dir should be 0";
    
    return "all tests passed";
}

test();

"basic numeric enum test";
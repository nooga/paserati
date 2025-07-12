// expect: enum module test success

// Test enum module export/import functionality
import { Direction, Color, Status, Mixed } from "./enums";

function test() {
    // Test numeric enum
    if (Direction.Up !== 0) return "Direction.Up should be 0";
    if (Direction.Down !== 1) return "Direction.Down should be 1";
    if (Direction[0] !== "Up") return "Direction[0] should be 'Up'";
    if (Direction[1] !== "Down") return "Direction[1] should be 'Down'";
    
    // Test const enum
    if (Color.Red !== 0) return "Color.Red should be 0";
    if (Color.Green !== 1) return "Color.Green should be 1";
    if (Color.Blue !== 2) return "Color.Blue should be 2";
    
    // Test string enum
    if (Status.Loading !== "loading") return "Status.Loading should be 'loading'";
    if (Status.Success !== "success") return "Status.Success should be 'success'";
    if (Status.Error !== "error") return "Status.Error should be 'error'";
    
    // Test mixed enum
    if (Mixed.A !== 0) return "Mixed.A should be 0";
    if (Mixed.B !== "hello") return "Mixed.B should be 'hello'";
    if (Mixed.C !== 5) return "Mixed.C should be 5";
    if (Mixed.D !== 6) return "Mixed.D should be 6";
    
    // Note: Function exports using enum types are not yet supported
    
    // Test type checking - these should work
    let dir: Direction = Direction.Left;
    let color: Color = Color.Blue;
    let status: Status = Status.Success;
    
    if (dir !== Direction.Left) return "dir should be Direction.Left";
    if (color !== Color.Blue) return "color should be Color.Blue";
    if (status !== Status.Success) return "status should be Status.Success";
    
    return "enum module test success";
}

test();
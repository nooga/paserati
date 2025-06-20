// 🎨 Enhanced Error Reporting Showcase
// This file demonstrates Paserati's TypeScript-style error reporting

console.log("Testing enhanced error reporting with multiple error types...");

// Type assignment error (PS2001)
let stringVar: string = 42;

// Generic constraint violation (PS2004)  
interface Lengthable {
    length: number;
}

interface Container<T extends Lengthable> {
    item: T;
}

let badContainer: Container<number> = {
    item: 123
};

// Function argument type error (PS2003)
function takesString(s: string): void {
    console.log(s);
}

takesString(999);

console.log("If you see this, the errors above were handled gracefully!");

// expect_compile_error: Multiple enhanced errors with codes and formatting
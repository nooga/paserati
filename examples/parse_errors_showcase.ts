// 🔧 Parse Error Showcase  
// This file demonstrates various parsing/syntax errors

console.log("Testing parse errors...");

// Missing semicolon (if required)
let x = 42
let y = 24  // Might cause issues depending on ASI rules

// Invalid assignment target
42 = "cannot assign to literal";  // Invalid left-hand side

// Mismatched brackets
let array = [1, 2, 3};  // Square bracket opened, curly brace closed

// Invalid function syntax
function = "not valid";  // Missing function name

// Missing parentheses in function call
console.log "missing parens";  // Should be console.log("...")

// Invalid object literal
let obj = {
    key1: "value1",
    key2  // Missing colon and value
    key3: "value3"
};

// Unexpected token in expression
let result = 5 + + 3;  // Double plus might be problematic

// Invalid interface syntax
interface BadInterface {
    name string;  // Missing colon
    age: number
    = invalid;    // Unexpected equals in interface
}

console.log("If you see this, parse errors were handled!");

// expect_compile_error: Various syntax and parsing errors
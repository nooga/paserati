// 🔤 Lexing Error Showcase
// This file demonstrates various lexical analysis errors

console.log("Testing lexing errors...");

// Invalid character in identifier (contains unicode that lexer might not handle)
let var@ = 42;  // Invalid character '@' in identifier

// Unterminated string literal
let message = "Hello world;  // Missing closing quote

// Invalid number format
let badNumber = 123.45.67;  // Multiple decimal points

// Unterminated template literal
let template = `Hello ${name;  // Missing closing backtick

console.log("If you see this, lexing errors were handled!");

// expect_compile_error: Various lexical analysis errors
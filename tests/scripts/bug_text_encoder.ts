// Test that TextEncoder and TextDecoder are recognized types
// Bug 5: TextDecoder/TextEncoder builtin type declarations
const encoder = new TextEncoder();
const decoder = new TextDecoder();
const encoded = encoder.encode("hello");
const decoded = decoder.decode(encoded);
decoded;

// expect: hello

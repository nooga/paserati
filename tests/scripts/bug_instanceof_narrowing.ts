// Test that instanceof narrows types for Error and other constructors
// Bug 6b: instanceof narrowing should work for all constructors, not just Date/Array/Object
function checkError(val: unknown): string {
  if (val instanceof Error) {
    return val.message;
  }
  return "not an error";
}

checkError(new Error("found it"));

// expect: found it

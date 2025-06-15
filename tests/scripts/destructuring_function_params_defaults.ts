// Test function parameter destructuring with defaults
function createMessage([prefix = "Hello", suffix = "World"]) {
  return prefix + " " + suffix;
}

let result1 = createMessage(["Hi", "there"]);
let result2 = createMessage(["Goodbye"]);

`${result1}, ${result2}`;
// expect: Hi there, Goodbye World
// KNOWN ISSUE: Defaults in destructuring parameters don't work correctly yet
// Currently returns: Hello World, Hello World

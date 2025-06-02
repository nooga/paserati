// Test file for default arguments limitations
// These features are not yet implemented and should cause type errors
// expect: true

// LIMITATION 1: Default parameters cannot reference other parameters
// This should cause a type error: "undefined variable: a"
// function paramRef(a: number, b: number = a + 1): number {
//   return a + b;
// }

// LIMITATION 2: Default parameters cannot reference earlier parameters
// This should cause a type error: "undefined variable: name"
// function nameRef(name: string, greeting: string = "Hello " + name): string {
//   return greeting;
// }

// LIMITATION 3: Complex expressions in defaults may have scoping issues
// This should cause a type error if the parameter is not in scope
// function complexDefault(data: string, processed: string = data.toUpperCase()): string {
//   return processed;
// }

// WHAT WORKS: Simple literal defaults
function simpleDefaults(
  a: number = 5,
  b: string = "hello",
  c: boolean = true
): string {
  return a + " " + b + " " + (c ? "yes" : "no");
}

// WHAT WORKS: Expression defaults that don't reference parameters
function expressionDefaults(
  x: number = 2 + 3,
  y: string = "pre" + "fix"
): string {
  return y + ": " + x;
}

// Test what currently works
simpleDefaults() === "5 hello yes" && expressionDefaults() === "prefix: 5";

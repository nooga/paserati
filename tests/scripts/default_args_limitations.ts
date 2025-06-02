// Test file for default arguments limitations and capabilities
// expect: true

// ✅ WORKS: Default parameters CAN reference earlier parameters (FIXED!)
function paramRef(a: number, b: number = a + 1): number {
  return a + b;
}

// ✅ WORKS: Default parameters CAN reference earlier parameters by name (FIXED!)
function nameRef(name: string, greeting: string = "Hello " + name): string {
  return greeting;
}

// ❌ LIMITATION: Default parameters CANNOT reference later parameters (forward references)
// This should cause a type error: "undefined variable: b"
// function forwardRef(a: number = b + 1, b: number): number {
//   return a + b;
// }

// ❌ LIMITATION: Complex expressions may have issues with method calls on parameters
// This should work in theory but may have issues depending on available built-ins
// function complexDefault(data: string, processed: string = data.toUpperCase()): string {
//   return processed;
// }

// ✅ WORKS: Simple literal defaults
function simpleDefaults(
  a: number = 5,
  b: string = "hello",
  c: boolean = true
): string {
  return a + " " + b + " " + (c ? "yes" : "no");
}

// ✅ WORKS: Expression defaults that don't reference parameters
function expressionDefaults(
  x: number = 2 + 3,
  y: string = "pre" + "fix"
): string {
  return y + ": " + x;
}

// ✅ WORKS: Parameter references in complex expressions
function complexParamRefs(
  x: number,
  y: number = x * 2,
  z: number = x + y,
  result: string = "Result: " + (x + y + z)
): string {
  return result;
}

// Test what currently works
let test1 = simpleDefaults() === "5 hello yes";
let test2 = expressionDefaults() === "prefix: 5";
let test3 = paramRef(3) === 7; // 3 + (3+1) = 7
let test4 = nameRef("World") === "Hello World";
let test5 = complexParamRefs(2) === "Result: 12"; // 2 + 4 + 6 = 12

test1 && test2 && test3 && test4 && test5;

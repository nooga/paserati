// Simple test for type narrowing with function type aliases
type MyFunc = (x: number) => string;

function test(param: MyFunc | string | undefined) {
  if (typeof param === "function") {
    // This should work after type narrowing
    return param(42);
  }
  return "not a function";
}

let fn: MyFunc = (n: number) => "result: " + n;
test(fn);

// expect: result: 42
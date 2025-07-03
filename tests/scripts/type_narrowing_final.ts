// Final test for type narrowing improvements
// Tests that type aliases can be narrowed with typeof function guards

// Basic function type alias
type MyFunc = (x: number) => string;

function test(param: MyFunc | string | undefined) {
  if (typeof param === "function") {
    // Should work - param is narrowed from union to just MyFunc
    return param(42);
  }
  return "not a function";
}

// Test it works
let func: MyFunc = (n) => "result: " + n;
test(func);

// expect: result: 42
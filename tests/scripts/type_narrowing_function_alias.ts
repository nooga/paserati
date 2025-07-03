// Test type narrowing with function type aliases
type MyFunction<T> = (value: string) => T;
type MyData = string | number;

function test(param: MyFunction<number> | MyData | undefined) {
  if (typeof param === "function") {
    // After narrowing, this should work
    return param("hello");
  }
  return "not a function";
}

let fn: MyFunction<number> = (s: string) => s.length;
test(fn);

// expect: 5
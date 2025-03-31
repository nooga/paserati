// expect: 0
type StringOrNumber = string | number;

let value: StringOrNumber = "hello";
value = 123;
// value = true; // Should be a type error

function proc(input: string | null): number {
  if (input === null) {
    return 0;
  }
  return 1; // Simplified
}

proc("test");
proc(null);
//proc(undefined); // Should be a type error

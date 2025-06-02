// Test function overloads
// expect: test

console.log("start");

// Basic string/number overloads
function test(x: string): string;
function test(x: number): number;
function test(x: string | number): string | number {
  return x;
}

console.log("foo");

let result1: string = test("hello");
let result2: number = test(42);

// Complex overloads with different return types
function convert(input: string): number;
function convert(input: number): string;
function convert(input: boolean): boolean;
function convert(input: string | number | boolean): string | number | boolean {
  if (typeof input === "string") {
    return 123;
  }
  if (typeof input === "number") {
    return "converted";
  }
  return input;
}

console.log("bar");

let converted1: number = convert("test");
let converted2: string = convert(456);
let converted3: boolean = convert(true);

// Test with any overload
function flexible(x: any): any;
function flexible(x: string): string;
function flexible(x: any): any {
  return x;
}

console.log("baz");

let flexible1 = flexible("test");
let flexible2 = flexible(123);
console.log("done");
flexible1;

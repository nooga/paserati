// expect: hello!84

// Test callable types with overloads - basic functionality
type ProcessFn = {
  (value: string): string;
  (value: number): number;
};

const process: ProcessFn = (value: any): any => {
  if (typeof value === "string") {
    return value + "!";
  } else {
    return value * 2;
  }
};

// Test both overloads and concatenate results
let stringResult = process("hello"); // Should be "hello!"
let numberResult = process(42); // Should be 84

// Final test result: concatenate the results
stringResult + numberResult;

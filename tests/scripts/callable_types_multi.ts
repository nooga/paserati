// expect: 9

// Test callable types with multiple overloads
type MultiFn = {
  (x: number): number;
  (x: string): string;
  (x: boolean): boolean;
};

const multiProcess: MultiFn = (x: any): any => {
  if (typeof x === "number") {
    return x * 3;
  } else if (typeof x === "string") {
    return x + x;
  } else if (typeof x === "boolean") {
    return !x;
  }
  return x;
};

// Test number overload
let numResult = multiProcess(3); // Should be 9

// Return the number result as the test value
numResult;

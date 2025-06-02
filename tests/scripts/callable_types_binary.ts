// expect: 8

// Test callable types with binary operations
type BinaryOp = {
  (a: number, b: number): number;
  (a: string, b: string): string;
};

const combine: BinaryOp = (a: any, b: any): any => {
  return a + b;
};

// Test number addition
let numResult = combine(3, 5); // Should be 8

// Return the number result
numResult;

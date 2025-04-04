// expect: Value: 123
type NumToString = (input: number) => string;

let converter: NumToString;

// Assign a compatible function
converter = (x: number) => {
  return "Value: " + x;
};

// Type error: Parameter type mismatch
// converter = (s: string) => s;

// Type error: Return type mismatch
// converter = (y: number) => y > 0;
converter(123); // Check call site

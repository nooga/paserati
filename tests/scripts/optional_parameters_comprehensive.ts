// Test optional parameters in interface methods only
interface TestInterface {
  // Method signature
  method(a: string, b?: number, c?: boolean): void;
  
  // Call signature
  (a: string, b?: number, c?: boolean): void;
  
  // Constructor signature
  new (a: string, b?: number, c?: boolean): any;
}

// Test function type aliases
type FuncType1 = (a: string, b?: number, c?: boolean) => void;

console.log("All optional parameter tests passed!");
// expect: undefined
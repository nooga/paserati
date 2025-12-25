// Simple test for optional parameters in interface methods
interface Test {
  method(a: string, b?: number): void;
}
// expect: undefined
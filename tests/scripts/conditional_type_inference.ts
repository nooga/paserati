// Conditional types with infer keyword
// expect: extracted

// Extract return type from function type
type ReturnType<T> = T extends (...args: any[]) => infer R ? R : never;

// Test function
function greet(): string {
  return "extracted";
}

// Use conditional type to extract return type
type GreetReturn = ReturnType<typeof greet>;

// Should be string
const result: GreetReturn = "extracted";
result;

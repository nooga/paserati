// expect: number
// Properly typed decorator + conditional type with infer (via built-in ReturnType)
// Tests that decorators work alongside advanced type-level programming

// A properly generic decorator that preserves the method signature
function track<T extends (...args: any[]) => any>(
  target: T,
  context: { kind: string; name: string },
): T {
  const replacement = function(...args: any[]): any {
    return (target as any).apply(null, args);
  };
  return replacement as unknown as T;
}

class DataService {
  @track
  fetchCount(): number {
    return 42;
  }
}

// Use built-in conditional type to extract the return type
type FetchReturn = ReturnType<() => number>;

// Verify the conditional type resolves correctly
function checkType(x: FetchReturn): string {
  return typeof x;
}

checkType(42);

// expect: raw
// Test: typeof "function" control flow narrowing eliminates callable members
// from generic union types

type Getter<T> = () => T;
type Spec<T> = T | Getter<T>;

function process<T>(spec: Spec<T>): string {
  if (typeof spec === "function") {
    return "fn";
  }
  // After typeof !== "function", Getter<T> is eliminated, spec is T
  let x: T = spec;
  return "raw";
}

process<number>(42);

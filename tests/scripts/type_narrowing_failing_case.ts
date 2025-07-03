// Test case that should fail type narrowing
type ContextFn<T> = (value: string) => T;

function test(date: ContextFn<number> | string | undefined, value: string): number {
  if (typeof date === "function") {
    return date(value); // This line should fail if narrowing doesn't work
  }
  return 0;
}

// This should work
test((s: string) => s.length, "hello");

// expect: 5
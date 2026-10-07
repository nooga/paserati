// expect_compile_error: No overload matches this call.

// Test callable types error case - wrong argument type
type Fn = {
  (x: string): string;
  (x: number): number;
};

const fn: Fn = (x: any): any => x;

// This should cause a compile error - boolean doesn't match any overload
fn(true);

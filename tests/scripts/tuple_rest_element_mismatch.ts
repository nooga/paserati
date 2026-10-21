// A tuple's rest element is related to the target's.
// expect_compile_error: Type '[[unknown], ...number[]]' is not assignable
type E = [[unknown] | [], ...[string, number][]];
function g(bad: number[]): E {
  return [[1], ...bad];
}

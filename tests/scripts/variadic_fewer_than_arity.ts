// expect: true
// paserati#170: a variadic function's non-rest parameters are not a hard
// minimum argument count - real JS pads any missing one with undefined,
// the same as any non-variadic function's optional parameters, and the
// rest parameter itself ends up []. The VM used to throw "Expected at
// least N arguments but got M" instead, for every variadic function
// call site, unconditionally.
//
// Both calls below go through a value the checker can't statically
// resolve (`getFoo(): any`), so they exercise the VM's own runtime arity
// check directly - not the separate, intentionally out-of-scope
// compile-time diagnostic (pkg/checker/call.go:704) that still fires for
// a literal, statically-resolved call like `bar()` in a plain .ts file.
class Foo {
  browse(path, ...args) {
    return [path, args];
  }
  browseTwo(a, b, ...rest) {
    return [a, b, rest];
  }
}
function getFoo(): any {
  return new Foo();
}

const oneNamed = getFoo().browse();
const twoNamed = getFoo().browseTwo(1);

oneNamed[0] === undefined &&
  oneNamed[1].length === 0 &&
  twoNamed[0] === 1 &&
  twoNamed[1] === undefined &&
  twoNamed[2].length === 0;

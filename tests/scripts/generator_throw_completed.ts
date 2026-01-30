// Test generator throw() method on completed generator
// Per ECMAScript spec, throw() on completed generator re-throws the original exception
// expect_runtime_error: test error

function* gen() {
  yield 1;
  return "done";
}

let g = gen();
g.next();  // Get first yield
g.next();  // Complete the generator
g.throw("test error");  // Throw into completed generator
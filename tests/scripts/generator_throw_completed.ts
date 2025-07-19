// Test generator throw() method on completed generator
// expect_runtime_error: exception thrown: test error

function* gen() {
  yield 1;
  return "done";
}

let g = gen();
g.next();  // Get first yield
g.next();  // Complete the generator
g.throw("test error");  // Throw into completed generator
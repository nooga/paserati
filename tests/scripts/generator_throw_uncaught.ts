// Test generator throw() method with uncaught exception
// expect_runtime_error: exception thrown: test error

function* gen() {
  yield 1;
  yield 2;  // This should not be reached
}

let g = gen();
g.next();  // Get to the first yield
g.throw("test error");  // Throw into the generator (uncaught)
// Test generator throw() method
// expect: caught error

function* gen() {
  try {
    yield 1;
    yield 2;  // This should not be reached
  } catch (e) {
    return "caught " + e;
  }
}

let g = gen();
g.next();  // Get to the first yield
let result = g.throw("error");  // Throw into the generator
result.value;
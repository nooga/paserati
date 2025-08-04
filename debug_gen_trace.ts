// Simple test to trace generator argument passing
function* test(x: number) {
  yield x;
}

let gen = test(42);
gen.next();
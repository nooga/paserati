// verify hoisting inside function body and that inner functions see outer-scope functions
// expect: ok

function top() {
  return inner();

  function inner(): string {
    return id("ok");
  }
}

function id<T>(v: T): T {
  return v;
}

top();

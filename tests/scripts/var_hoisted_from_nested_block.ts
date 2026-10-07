// expect: 5
// A `var` is function scoped: it is visible (and its use must type check)
// outside the block that declares it and before the declaration statement.

function f() {
  const read = () => later;
  if (true) {
    var inner = 5;
  }
  var later = inner;
  return read();
}

f();

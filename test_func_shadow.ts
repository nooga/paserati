// expect: outer
function test() {
  let x = 'outer';
  {
    let x = 'inner';
  }
  return x;
}
test();

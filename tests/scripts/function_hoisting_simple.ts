// ensure function declarations are hoisted within their scope
// expect: 3

function outer(): number {
  return add(2);

  function add(x: number): number {
    return x + 1;
  }
}

outer();

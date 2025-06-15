// expect: 10

// Test bind with partial application only
function add(a: number, b: number) {
  return a + b;
}

let add5 = add.bind(null, 5);
add5(5);
